package pipeline

import (
	"fmt"
	"sort"
)

// Uplink describes how the VPC reaches the outside world through the Internet Gateway.
// Everything below is OVS-native: gateway IPs are ARP/ICMP responders, routing is
// dec_ttl + MAC rewrite, and source NAT is conntrack in the VPC bridge; the only
// non-OVS element is the physical NIC attached to the external OVS bridge.
type Uplink struct {
	ActiveHere  bool   // this node owns the physical uplink
	PatchPort   int    // ofport on the VPC bridge of the patch to the external bridge
	ExtMAC      string // MAC of the external bridge (source MAC towards the upstream router)
	UpstreamMAC string // MAC of the upstream router
	ExtIP       string // SNAT source for the Internet Gateway (uplink primary address)
	TunnelToIGW int    // on other nodes: tunnel ofport towards the IGW node (0 = none)
}

// Zones for the IGW (zone-wide per VPC, like the NAT gateways).
func (in Input) igwZone() int { return in.natZoneBase() + 90 }

// IGWZone is exported so the agent can program the external bridge with the same zone.
func IGWZone(in Input) int { return in.igwZone() }

// GatewayMAC is the virtual router MAC of a subnet gateway address.
func GatewayMAC(ip string) string { return VIPMAC(ip) }

func (b *builder) l3() {
	if len(b.in.Gateways) == 0 {
		return
	}
	names := make([]string, 0, len(b.in.Gateways))
	for n := range b.in.Gateways {
		names = append(names, n)
	}
	sort.Strings(names)
	var gwMACs []string
	for _, n := range names {
		g := b.in.Gateways[n]
		mac := GatewayMAC(g)
		gwMACs = append(gwMACs, mac)
		b.add("table=%d,priority=300,arp,arp_tpa=%s,arp_op=1,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:%s->eth_src,load:2->NXM_OF_ARP_OP[],move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],set_field:%s->arp_sha,move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],set_field:%s->arp_spa,in_port", tClassify, g, mac, mac, g)
		// ping the gateway
		b.add("table=%d,priority=290,icmp,icmp_type=8,nw_dst=%s,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:%s->eth_src,move:NXM_OF_IP_SRC[]->NXM_OF_IP_DST[],set_field:%s->ip_src,load:0->NXM_OF_ICMP_TYPE[],set_field:64->nw_ttl,in_port", tClassify, g, mac, g)
	}
	// Routing between subnets: frames addressed to a gateway MAC are re-addressed to the
	// destination VNic (new source MAC = gateway of the destination subnet) with TTL decremented.
	ips := make([]string, 0, len(b.in.IPToMAC))
	for ip := range b.in.IPToMAC {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	for _, ip := range ips {
		dstGW := ""
		if sn := b.in.IPToSubnet[ip]; sn != "" {
			dstGW = b.in.Gateways[sn]
		}
		if dstGW == "" {
			continue
		}
		rewrite := fmt.Sprintf("dec_ttl,set_field:%s->eth_src,set_field:%s->eth_dst,resubmit(,%d)", GatewayMAC(dstGW), b.in.IPToMAC[ip], tForward)
		for _, m := range gwMACs {
			b.add("table=%d,priority=100,ip,dl_dst=%s,nw_dst=%s,actions=%s", tForward, m, ip, rewrite)
		}
		if u := b.in.Uplink; u != nil && u.ActiveHere {
			// in_port stays the patch port after the rewrite, so a register marks "already routed"
			b.add("table=%d,priority=105,in_port=%d,reg4=0,ip,nw_dst=%s,actions=load:1->NXM_NX_REG4[],%s", tForward, u.PatchPort, ip, rewrite)
		}
	}
	// Load-balancer VIPs live on the LB nodes: other nodes route VIP traffic there.
	vips := make([]string, 0, len(b.in.RemoteVIPs))
	for v := range b.in.RemoteVIPs {
		vips = append(vips, v)
	}
	sort.Strings(vips)
	for _, m := range gwMACs {
		for _, v := range vips {
			b.add("table=%d,priority=97,ip,dl_dst=%s,nw_dst=%s,actions=output:%d", tForward, m, v, b.in.RemoteVIPs[v])
		}
	}
	// Traffic that leaves the VPC: addressed to a gateway MAC but not to a VPC address.
	for _, m := range gwMACs {
		for _, c := range b.in.VPCCIDRs {
			b.add("table=%d,priority=95,ip,dl_dst=%s,nw_dst=%s,actions=drop", tForward, m, c)
		}
		switch u := b.in.Uplink; {
		case u == nil:
			b.add("table=%d,priority=90,ip,dl_dst=%s,actions=drop", tForward, m)
		case u.ActiveHere:
			b.add("table=%d,priority=90,ip,dl_dst=%s,actions=dec_ttl,set_field:%s->eth_src,set_field:%s->eth_dst,output:%d", tForward, m, u.ExtMAC, u.UpstreamMAC, u.PatchPort)
		case u.TunnelToIGW > 0:
			b.add("table=%d,priority=90,ip,dl_dst=%s,actions=output:%d", tForward, m, u.TunnelToIGW)
		default:
			b.add("table=%d,priority=90,ip,dl_dst=%s,actions=drop", tForward, m)
		}
	}
}

// igw compiles source NAT of the whole VPC to the uplink address (the Internet Gateway).
func (b *builder) igw() {
	u := b.in.Uplink
	if u == nil || !u.ActiveHere || u.ExtIP == "" {
		return
	}
	zone := b.in.igwZone()
	if !b.vpcBypass {
		for _, c := range b.in.VPCCIDRs {
			b.add("table=%d,priority=300,ip,nw_dst=%s,actions=resubmit(,%d)", tNAT, c, tIngressDispatch)
		}
		b.vpcBypass = true
	}
	ck := b.newCookie(CookieRef{"InternetGateway", b.in.VPCNamespace, b.in.VPCName, "translations", "SNAT"})
	b.add("table=%d,priority=200,ip,ct_state=-trk,nw_dst=%s,actions=ct(table=%d,zone=%d,nat)", tNAT, u.ExtIP, tNATApply, zone)
	for _, c := range b.in.VPCCIDRs {
		b.add("table=%d,priority=190,ip,ct_state=-trk,nw_src=%s,actions=ct(table=%d,zone=%d,nat)", tNAT, c, tNATApply, zone)
		b.add("cookie=0x%x,table=%d,priority=195,ip,ct_zone=%d,ct_state=+trk+new,nw_src=%s,actions=ct(commit,zone=%d,nat(src=%s)),ct_clear,resubmit(,%d)", ck, tNATApply, zone, c, zone, u.ExtIP, tIngressDispatch)
	}
	b.add("table=%d,priority=190,ip,ct_zone=%d,ct_state=+trk+est,actions=ct_clear,resubmit(,%d)", tNATApply, zone, tIngressDispatch)
}

// ExternalFlows programs the external bridge: it only moves frames between the physical
// uplink and the patch port to the VPC bridge, plus ARP answers for NAT gateway addresses.
// uplink/patch are the ofports on the external bridge.
func ExternalFlows(uplink, patch int, igwIP string, natIPs []string, igwZone int) []string {
	var f []string
	sort.Strings(natIPs)
	for _, ip := range natIPs {
		mac := VIPMAC(ip)
		f = append(f, fmt.Sprintf("table=0,priority=300,arp,arp_tpa=%s,arp_op=1,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:%s->eth_src,load:2->NXM_OF_ARP_OP[],move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],set_field:%s->arp_sha,move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],set_field:%s->arp_spa,in_port", ip, mac, mac, ip))
		f = append(f, fmt.Sprintf("table=0,priority=150,in_port=%d,ip,nw_dst=%s,actions=output:%d", uplink, ip, patch))
	}
	f = append(f, fmt.Sprintf("table=0,priority=200,in_port=%d,actions=output:%d", patch, uplink))
	if igwIP != "" {
		// replies for the gateway address go through conntrack; everything else is the host's own traffic
		f = append(f, fmt.Sprintf("table=0,priority=140,in_port=%d,ip,nw_dst=%s,actions=ct(table=1,zone=%d)", uplink, igwIP, igwZone))
		f = append(f, fmt.Sprintf("table=1,priority=100,ct_state=+trk+est+rpl,ip,actions=ct_clear,output:%d", patch))
		f = append(f, "table=1,priority=0,actions=ct_clear,NORMAL")
	}
	f = append(f, "table=0,priority=0,actions=NORMAL")
	return f
}
