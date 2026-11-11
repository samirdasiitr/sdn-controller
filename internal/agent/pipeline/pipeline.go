// Package pipeline compiles the VPC's network services (firewalls, load
// balancers, NAT gateways) and VNic port security into one OpenFlow pipeline
// for the VPC bridge on a node.
//
// Stage layout (each stage ends with resubmit to the next):
//
//	0   classify + port security           (arp -> NORMAL, VNic identity check)
//	5   egress ACL dispatch  -> 10 egress ACL   (conntrack zone = VNic ofport)
//	15  load balancer: VIP -> select group (DNAT); replies -> un-DNAT
//	16  full-NAT second step (SNAT client to VIP for clients arriving by tunnel)
//	17  un-SNAT for full-NAT replies
//	19  clear conntrack state
//	20  NAT gateway: SNAT / un-SNAT  -> 21 NAT apply
//	25  ingress ACL dispatch -> 30 ingress ACL  (zone = destination VNic ofport)
//	40  forwarding (NORMAL)
package pipeline

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
)

const (
	tClassify, tEgressDispatch, tEgressACL         = 0, 5, 10
	tLB, tLBSnat, tLBUnSnat, tLBDone               = 15, 16, 17, 19
	tNAT, tNATApply, tIngressDispatch, tIngressACL = 20, 21, 25, 30
	tForward                                       = 40

	groupBase = 1000
)

// Port is a VNic bound to a local OVS port.
type Port struct {
	Key          string // namespace/name
	IP, MAC      string
	Subnet       string
	FirewallRef  string
	OFPort       int
	PortSecurity bool
}

// Input is everything needed to compile the pipeline of one VPC bridge on one node.
type Input struct {
	// VNI selects conntrack zone ranges. Zones are global to the kernel datapath, so
	// VPCs (which may use overlapping CIDRs) must not share them.
	VNI         int
	VPCCIDRs    []string
	Subnets     map[string]string // subnet name -> CIDR
	Routes      []string
	Ports       []Port
	TunnelPorts []int
	IPToMAC     map[string]string // every VNic in the VPC
	// RemoteMACs maps the MAC of a VNic hosted on another node to the local tunnel port
	// leading there. Needed for hairpins: traffic that arrives from a tunnel and must leave
	// through the same tunnel (e.g. client and backend on the same remote node) is dropped
	// by NORMAL, so these are forwarded explicitly.
	RemoteMACs map[string]int
	Firewalls  []servicev1alpha1.Firewall
	LBs        []servicev1alpha1.LoadBalancer
	NATs       []gwv1alpha1.NATGateway
}

// Group is an OpenFlow group definition (without the group_id= prefix).
type Group struct {
	ID  int
	Def string
}

// Report tells how an object was compiled.
type Report struct {
	Flows    int
	Warnings []string
	Error    string
}

// CookieRef maps a flow cookie back to the rule that produced it (for counters).
type CookieRef struct{ Kind, Namespace, Name, Rule, Action string }

// Output is the compiled pipeline.
type Output struct {
	Flows   []string
	Groups  []Group
	Reports map[string]*Report // key: Kind/namespace/name
	Cookies map[uint64]CookieRef
	// Zones lets the agent read conntrack counts per object.
	LBZones  map[string]int // key -> zone
	NATZones map[string]int
}

func ReportKey(kind, ns, name string) string { return kind + "/" + ns + "/" + name }

// Zone bases: ACL zones are base+ofport (<1000 ports), LB uses base+2i/2i+1, NAT base+i.
func (in Input) aclZone(ofport int) int { return (in.VNI%40)*1000 + ofport }
func (in Input) lbZoneBase() int        { return 40000 + (in.VNI%100)*100 }
func (in Input) natZoneBase() int       { return 50000 + (in.VNI%100)*100 }

type builder struct {
	in     Input
	out    Output
	cookie uint64
}

func Build(in Input) Output {
	b := &builder{in: in, out: Output{Reports: map[string]*Report{}, Cookies: map[uint64]CookieRef{}, LBZones: map[string]int{}, NATZones: map[string]int{}}}
	b.base()
	b.portSecurity()
	b.firewalls()
	b.loadBalancers()
	b.natGateways()
	return b.out
}

func (b *builder) add(f string, a ...interface{}) {
	b.out.Flows = append(b.out.Flows, fmt.Sprintf(f, a...))
}

func (b *builder) report(kind, ns, name string) *Report {
	k := ReportKey(kind, ns, name)
	if b.out.Reports[k] == nil {
		b.out.Reports[k] = &Report{}
	}
	return b.out.Reports[k]
}

func (b *builder) newCookie(ref CookieRef) uint64 {
	b.cookie++
	b.out.Cookies[b.cookie] = ref
	return b.cookie
}

func (b *builder) base() {
	b.add("table=%d,priority=50,arp,actions=NORMAL", tClassify)
	seen := map[string]bool{}
	for _, d := range b.in.Routes {
		if !seen[d] {
			seen[d] = true
			b.add("table=%d,priority=100,ip,nw_dst=%s,actions=resubmit(,%d)", tClassify, d, tEgressDispatch)
		}
	}
	b.add("table=%d,priority=0,actions=resubmit(,%d)", tClassify, tEgressDispatch)
	b.add("table=%d,priority=0,actions=resubmit(,%d)", tEgressDispatch, tLB)
	b.add("table=%d,priority=0,actions=resubmit(,%d)", tLB, tLBDone)
	b.add("table=%d,priority=0,actions=ct_clear,resubmit(,%d)", tLBSnat, tLBDone)
	b.add("table=%d,priority=0,actions=ct_clear,resubmit(,%d)", tLBDone, tNAT)
	b.add("table=%d,priority=0,actions=resubmit(,%d)", tNAT, tIngressDispatch)
	b.add("table=%d,priority=0,actions=ct_clear,resubmit(,%d)", tNATApply, tIngressDispatch)
	b.add("table=%d,priority=0,actions=resubmit(,%d)", tIngressDispatch, tForward)
	b.add("table=%d,priority=0,actions=NORMAL", tForward)
	macs := make([]string, 0, len(b.in.RemoteMACs))
	for m := range b.in.RemoteMACs {
		macs = append(macs, m)
	}
	sort.Strings(macs)
	for _, m := range macs {
		tp := b.in.RemoteMACs[m]
		b.add("table=%d,priority=120,in_port=%d,dl_dst=%s,actions=IN_PORT", tForward, tp, m)
		b.add("table=%d,priority=110,dl_dst=%s,actions=output:%d", tForward, m, tp)
	}
}

func (b *builder) portSecurity() {
	for _, p := range b.in.Ports {
		if !p.PortSecurity {
			continue
		}
		b.add("table=%d,priority=200,in_port=%d,dl_src=%s,arp,arp_spa=%s,actions=NORMAL", tClassify, p.OFPort, p.MAC, p.IP)
		b.add("table=%d,priority=200,in_port=%d,dl_src=%s,arp,arp_spa=0.0.0.0,actions=NORMAL", tClassify, p.OFPort, p.MAC)
		b.add("table=%d,priority=200,in_port=%d,dl_src=%s,ip,nw_src=%s,actions=resubmit(,%d)", tClassify, p.OFPort, p.MAC, p.IP, tEgressDispatch)
		b.add("table=%d,priority=190,in_port=%d,actions=drop", tClassify, p.OFPort)
	}
}

// ---------------------------------------------------------------- firewalls

type policy struct {
	id       int
	fws      []servicev1alpha1.Firewall
	stateful bool
	deny     bool // default action
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (b *builder) firewalls() {
	// Group local VNic ports by the set of firewalls that apply to them.
	byKey := map[string]*policy{}
	portPolicy := map[int]*policy{}
	for _, p := range b.in.Ports {
		var fws []servicev1alpha1.Firewall
		for _, fw := range b.in.Firewalls {
			if fw.Name == p.FirewallRef || len(fw.Spec.SubnetRefs) == 0 || contains(fw.Spec.SubnetRefs, p.Subnet) {
				fws = append(fws, fw)
			}
		}
		if len(fws) == 0 {
			continue
		}
		sort.Slice(fws, func(i, j int) bool { return fws[i].Namespace+"/"+fws[i].Name < fws[j].Namespace+"/"+fws[j].Name })
		names := make([]string, len(fws))
		for i, f := range fws {
			names[i] = f.Namespace + "/" + f.Name
		}
		key := strings.Join(names, ",")
		pol := byKey[key]
		if pol == nil {
			pol = &policy{fws: fws}
			for _, f := range fws {
				pol.stateful = pol.stateful || f.Spec.Stateful
				pol.deny = pol.deny || f.Spec.DefaultAction != servicev1alpha1.FirewallActionAllow
			}
			byKey[key] = pol
		}
		portPolicy[p.OFPort] = pol
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		byKey[k].id = i + 1
	}

	for _, fw := range b.in.Firewalls {
		b.report("Firewall", fw.Namespace, fw.Name)
	}

	for _, k := range keys {
		pol := byKey[k]
		b.policyFlows(pol)
	}

	for _, p := range b.in.Ports {
		pol := portPolicy[p.OFPort]
		if pol == nil {
			continue
		}
		reg := fmt.Sprintf("load:%d->NXM_NX_REG0[0..15],load:%d->NXM_NX_REG1[]", b.in.aclZone(p.OFPort), pol.id)
		if pol.stateful {
			b.add("table=%d,priority=100,ip,in_port=%d,actions=%s,ct(table=%d,zone=NXM_NX_REG0[0..15])", tEgressDispatch, p.OFPort, reg, tEgressACL)
			b.add("table=%d,priority=100,ip,nw_dst=%s,actions=%s,ct(table=%d,zone=NXM_NX_REG0[0..15])", tIngressDispatch, p.IP, reg, tIngressACL)
		} else {
			b.add("table=%d,priority=100,ip,in_port=%d,actions=%s,resubmit(,%d)", tEgressDispatch, p.OFPort, reg, tEgressACL)
			b.add("table=%d,priority=100,ip,nw_dst=%s,actions=%s,resubmit(,%d)", tIngressDispatch, p.IP, reg, tIngressACL)
		}
	}
}

func (b *builder) policyFlows(pol *policy) {
	for _, tbl := range []struct {
		table int
		dir   servicev1alpha1.FirewallDirection
		next  int
	}{{tEgressACL, servicev1alpha1.FirewallDirectionEgress, tLB}, {tIngressACL, servicev1alpha1.FirewallDirectionIngress, tForward}} {
		if pol.stateful {
			b.add("table=%d,priority=65535,ct_state=+trk+inv,reg1=%d,actions=drop", tbl.table, pol.id)
			b.add("table=%d,priority=100,ct_state=+trk+est,reg1=%d,actions=ct_clear,resubmit(,%d)", tbl.table, pol.id, tbl.next)
			b.add("table=%d,priority=100,ct_state=+trk+rel,reg1=%d,actions=ct_clear,resubmit(,%d)", tbl.table, pol.id, tbl.next)
		}
		for _, fw := range pol.fws {
			rep := b.report("Firewall", fw.Namespace, fw.Name)
			rules := append([]servicev1alpha1.FirewallRule(nil), fw.Spec.Rules...)
			sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
			for _, r := range rules {
				if r.Direction != tbl.dir {
					continue
				}
				matches, err := ruleMatches(r.Match)
				if err != nil {
					if tbl.dir == servicev1alpha1.FirewallDirectionEgress {
						rep.Warnings = append(rep.Warnings, fmt.Sprintf("rule %q skipped: %v", r.Name, err))
					}
					continue
				}
				if r.Action == servicev1alpha1.FirewallActionLog && tbl.dir == servicev1alpha1.FirewallDirectionEgress {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("rule %q: action Log is enforced as Allow (counters are exported as metrics)", r.Name))
				}
				allow := r.Action != servicev1alpha1.FirewallActionDeny
				prio := 65536 - int(r.Priority)
				ck := b.newCookie(CookieRef{"Firewall", fw.Namespace, fw.Name, r.Name, string(r.Action)})
				for _, m := range matches {
					ct := ""
					if pol.stateful {
						ct = "ct_state=+trk+new,"
					}
					b.add("cookie=0x%x,table=%d,priority=%d,%sreg1=%d,%s,actions=%s", ck, tbl.table, prio, ct, pol.id, m, b.verdict(pol, allow, tbl.next))
					rep.Flows++
				}
			}
		}
		b.add("table=%d,priority=0,reg1=%d,actions=%s", tbl.table, pol.id, b.verdict(pol, !pol.deny, tbl.next))
	}
}

func (b *builder) verdict(pol *policy, allow bool, next int) string {
	if !allow {
		return "drop"
	}
	if pol.stateful {
		return fmt.Sprintf("ct(commit,zone=NXM_NX_REG0[0..15]),ct_clear,resubmit(,%d)", next)
	}
	return fmt.Sprintf("resubmit(,%d)", next)
}

func isV4CIDR(c string) bool { return strings.Contains(c, ".") && !strings.Contains(c, ":") }

// ruleMatches converts a rule match into one or more OpenFlow match strings (port ranges expand to masks).
func ruleMatches(m servicev1alpha1.FirewallRuleMatch) ([]string, error) {
	proto := strings.ToLower(m.Protocol)
	var base []string
	switch proto {
	case "", "any":
		base = []string{"ip"}
		if m.SrcPort != "" || m.DstPort != "" {
			return nil, fmt.Errorf("ports require protocol TCP, UDP or SCTP")
		}
	case "tcp", "udp", "sctp", "icmp":
		base = []string{proto}
		if proto == "icmp" && (m.SrcPort != "" || m.DstPort != "") {
			return nil, fmt.Errorf("ICMP has no ports")
		}
	default:
		return nil, fmt.Errorf("unknown protocol %q", m.Protocol)
	}
	for _, c := range []struct{ k, v string }{{"nw_src", m.SrcCIDR}, {"nw_dst", m.DstCIDR}} {
		if c.v == "" || c.v == "0.0.0.0/0" {
			continue
		}
		if !isV4CIDR(c.v) {
			return nil, fmt.Errorf("only IPv4 CIDRs are supported (%s)", c.v)
		}
		base = append(base, c.k+"="+c.v)
	}
	srcM, err := portMatches("tp_src", m.SrcPort)
	if err != nil {
		return nil, err
	}
	dstM, err := portMatches("tp_dst", m.DstPort)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range srcM {
		for _, d := range dstM {
			parts := append([]string(nil), base...)
			if s != "" {
				parts = append(parts, s)
			}
			if d != "" {
				parts = append(parts, d)
			}
			out = append(out, strings.Join(parts, ","))
		}
	}
	return out, nil
}

// portMatches parses "80", "1024-65535" or comma lists into masked OpenFlow port matches.
func portMatches(field, spec string) ([]string, error) {
	if spec == "" {
		return []string{""}, nil
	}
	var out []string
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		lo, hi := 0, 0
		if a, b, ok := strings.Cut(part, "-"); ok {
			lo, _ = strconv.Atoi(strings.TrimSpace(a))
			hi, _ = strconv.Atoi(strings.TrimSpace(b))
		} else {
			lo, _ = strconv.Atoi(part)
			hi = lo
		}
		if lo < 1 || hi > 65535 || lo > hi {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		for _, mm := range PortMasks(lo, hi) {
			out = append(out, field+"="+mm)
		}
	}
	return out, nil
}

// PortMasks covers [lo,hi] with the minimum number of value/mask blocks.
func PortMasks(lo, hi int) []string {
	var out []string
	for lo <= hi {
		size := 1
		for lo%(size*2) == 0 && lo+size*2-1 <= hi && size*2 <= 65536 {
			size *= 2
		}
		if size == 1 {
			out = append(out, fmt.Sprintf("0x%04x", lo))
		} else {
			out = append(out, fmt.Sprintf("0x%04x/0x%04x", lo, 0xffff&^(size-1)))
		}
		lo += size
	}
	return out
}

// ------------------------------------------------------------ load balancers

// VIPMAC is the virtual MAC answered by the ARP responder for a VIP.
func VIPMAC(ip string) string {
	var o [4]int
	fmt.Sscanf(ip, "%d.%d.%d.%d", &o[0], &o[1], &o[2], &o[3])
	return fmt.Sprintf("0a:5d:%02x:%02x:%02x:%02x", o[0], o[1], o[2], o[3])
}

func (b *builder) loadBalancers() {
	lbs := append([]servicev1alpha1.LoadBalancer(nil), b.in.LBs...)
	sort.Slice(lbs, func(i, j int) bool { return lbs[i].Namespace+"/"+lbs[i].Name < lbs[j].Namespace+"/"+lbs[j].Name })
	gid := groupBase
	for idx, lb := range lbs {
		rep := b.report("LoadBalancer", lb.Namespace, lb.Name)
		if lb.Spec.Type == servicev1alpha1.LBTypeL7 {
			rep.Error = "L7 load balancing is not implemented (OVS is L4 only)"
			continue
		}
		zone, zone2 := b.in.lbZoneBase()+2*idx, b.in.lbZoneBase()+2*idx+1
		b.out.LBZones[ReportKey("LoadBalancer", lb.Namespace, lb.Name)] = zone
		vip := lb.Spec.Frontend.IP
		vmac := VIPMAC(vip)
		method := "dp_hash"
		switch {
		case lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmIPHash:
			method = "hash,fields(ip_src)"
		case lb.Spec.SessionAffinity != nil && lb.Spec.SessionAffinity.Enabled:
			method = "hash,fields(ip_src)"
			rep.Warnings = append(rep.Warnings, "session affinity is implemented as source-IP hashing; the timeout is not used")
		case lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmLeastConn:
			rep.Warnings = append(rep.Warnings, "LeastConn has no OVS equivalent; using weighted hash distribution")
		}
		if lb.Spec.HealthCheck != nil {
			rep.Warnings = append(rep.Warnings, "health checks are not implemented: all backends are treated as healthy")
		}
		var backends []servicev1alpha1.LBBackend
		for _, be := range lb.Spec.Backends {
			if be.Weight > 0 {
				backends = append(backends, be)
			}
		}
		if len(backends) == 0 {
			rep.Error = "no backend with weight > 0"
			continue
		}

		b.add("table=%d,priority=300,arp,arp_tpa=%s,arp_op=1,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:%s->eth_src,load:2->NXM_OF_ARP_OP[],move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],set_field:%s->arp_sha,move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],set_field:%s->arp_spa,in_port", tClassify, vip, vmac, vmac, vip)
		rep.Flows++

		for _, bk := range backends {
			if b.in.IPToMAC[bk.IP] == "" {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("backend %s has no VNic: its MAC is learned by flooding", bk.IP))
			}
		}

		ck := b.newCookie(CookieRef{"LoadBalancer", lb.Namespace, lb.Name, "new-connections", "DNAT"})
		for _, fp := range lb.Spec.Frontend.Ports {
			proto := strings.ToLower(fp.Protocol)
			for _, v := range []struct {
				name string
				next int
			}{{"local", tLBDone}, {"tunnel", tLBSnat}} {
				variant, next := v.name, v.next
				gid++
				var buckets []string
				for _, bk := range backends {
					act := ""
					if mac := b.in.IPToMAC[bk.IP]; mac != "" {
						act = fmt.Sprintf("set_field:%s->eth_dst,", mac)
					}
					buckets = append(buckets, fmt.Sprintf("bucket=weight:%d,actions=%sct(commit,table=%d,zone=%d,nat(dst=%s:%d))", bk.Weight, act, next, zone, bk.IP, fp.TargetPort))
				}
				b.out.Groups = append(b.out.Groups, Group{ID: gid, Def: fmt.Sprintf("type=select,selection_method=%s,%s", method, strings.Join(buckets, ","))})
				if variant == "local" {
					b.add("cookie=0x%x,table=%d,priority=100,ip,%s,nw_dst=%s,tp_dst=%d,ct_state=-trk,actions=group:%d", ck, tLB, proto, vip, fp.Port, gid)
					rep.Flows++
				} else {
					for _, tp := range b.in.TunnelPorts {
						b.add("cookie=0x%x,table=%d,priority=110,in_port=%d,ip,%s,nw_dst=%s,tp_dst=%d,ct_state=-trk,actions=group:%d", ck, tLB, tp, proto, vip, fp.Port, gid)
						rep.Flows++
					}
				}
			}
			for _, bk := range backends {
				b.add("table=%d,priority=100,ip,%s,nw_src=%s,tp_src=%d,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tLB, proto, bk.IP, fp.TargetPort, tLBDone, zone)
				b.add("table=%d,priority=110,ip,%s,nw_src=%s,tp_src=%d,nw_dst=%s,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tLB, proto, bk.IP, fp.TargetPort, vip, tLBUnSnat, zone2)
				rep.Flows += 2
			}
		}
		// Full-NAT for clients that arrived through a tunnel: SNAT to the VIP so replies come back here.
		b.add("table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+new,actions=ct_clear,ct(commit,table=%d,zone=%d,nat(src=%s))", tLBSnat, zone, tLBDone, zone2, vip)
		// Established packets must re-apply the SNAT chosen for the first packet, otherwise the
		// backend sees a different source than in the SYN and resets the connection.
		b.add("table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+est,actions=ct_clear,ct(table=%d,zone=%d,nat)", tLBSnat, zone, tLBDone, zone2)
		b.add("table=%d,priority=100,ip,ct_zone=%d,actions=ct_clear,ct(table=%d,zone=%d,nat)", tLBUnSnat, zone2, tLBDone, zone)
		rep.Flows += 2
		// Replies re-enter with the VIP MAC as destination; restore the client's MAC when known.
		ips := make([]string, 0, len(b.in.IPToMAC))
		for ip := range b.in.IPToMAC {
			ips = append(ips, ip)
		}
		sort.Strings(ips)
		for _, ip := range ips {
			b.add("table=%d,priority=100,ip,dl_dst=%s,nw_dst=%s,actions=set_field:%s->eth_dst,resubmit(,%d)", tForward, vmac, ip, b.in.IPToMAC[ip], tForward)
		}
	}
}

// -------------------------------------------------------------- NAT gateways

func (b *builder) natGateways() {
	nats := append([]gwv1alpha1.NATGateway(nil), b.in.NATs...)
	sort.Slice(nats, func(i, j int) bool { return nats[i].Namespace+"/"+nats[i].Name < nats[j].Namespace+"/"+nats[j].Name })
	vpcCIDRDone := false
	for idx, nat := range nats {
		rep := b.report("NATGateway", nat.Namespace, nat.Name)
		if t := nat.Spec.Type; t != "" && t != gwv1alpha1.NATTypeSNAT {
			rep.Error = fmt.Sprintf("NAT type %s is not implemented; only SNAT is", t)
			continue
		}
		zone := b.in.natZoneBase() + idx
		b.out.NATZones[ReportKey("NATGateway", nat.Namespace, nat.Name)] = zone
		ext := nat.Spec.ExternalIP
		natSpec := "nat(src=" + ext + ")"
		if pr := nat.Spec.PortRange; pr != nil && pr.Start > 0 && pr.End >= pr.Start {
			natSpec = fmt.Sprintf("nat(src=%s:%d-%d)", ext, pr.Start, pr.End)
		}
		if !vpcCIDRDone {
			for _, c := range b.in.VPCCIDRs {
				b.add("table=%d,priority=300,ip,nw_dst=%s,actions=resubmit(,%d)", tNAT, c, tIngressDispatch)
			}
			vpcCIDRDone = true
		}
		b.add("table=%d,priority=200,ip,ct_state=-trk,nw_dst=%s,actions=ct(table=%d,zone=%d,nat)", tNAT, ext, tNATApply, zone)
		b.add("table=%d,priority=150,ip,ct_zone=%d,ct_state=+trk+new,nw_dst=%s,actions=drop", tNATApply, zone, ext)
		b.add("table=%d,priority=200,ip,ct_zone=%d,ct_state=+trk+est,actions=ct_clear,resubmit(,%d)", tNATApply, zone, tIngressDispatch)
		rep.Flows += 3
		// The host reaches the external IP through a permanent neighbor entry with this virtual
		// MAC; after un-NAT the frame still carries it, so restore the VM's real MAC.
		nmac := VIPMAC(ext)
		ips := make([]string, 0, len(b.in.IPToMAC))
		for ip := range b.in.IPToMAC {
			ips = append(ips, ip)
		}
		sort.Strings(ips)
		for _, ip := range ips {
			b.add("table=%d,priority=100,ip,dl_dst=%s,nw_dst=%s,actions=set_field:%s->eth_dst,resubmit(,%d)", tForward, nmac, ip, b.in.IPToMAC[ip], tForward)
		}
		ck := b.newCookie(CookieRef{"NATGateway", nat.Namespace, nat.Name, "snat", "SNAT"})
		for _, sn := range nat.Spec.SubnetRefs {
			cidr := b.in.Subnets[sn]
			if cidr == "" {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("subnet %q not found in the VPC", sn))
				continue
			}
			b.add("table=%d,priority=200,ip,ct_state=-trk,nw_src=%s,actions=ct(table=%d,zone=%d,nat)", tNAT, cidr, tNATApply, zone)
			b.add("cookie=0x%x,table=%d,priority=210,ip,ct_zone=%d,ct_state=+trk+new,nw_src=%s,actions=ct(commit,zone=%d,%s),ct_clear,resubmit(,%d)", ck, tNATApply, zone, cidr, zone, natSpec, tIngressDispatch)
			rep.Flows += 2
		}
	}
}
