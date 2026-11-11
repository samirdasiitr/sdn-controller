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
//	18  session-affinity table (flows learned by the LB groups, kept across reprogramming)
//	20  NAT gateway: SNAT / DNAT entry -> 21 apply -> 22 FullNAT source step, 23 FullNAT reply
//	25  ingress ACL dispatch -> 30 ingress ACL  (zone = destination VNic ofport)
//	40  forwarding (NORMAL)
package pipeline

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
)

const (
	tClassify, tEgressDispatch, tEgressACL         = 0, 5, 10
	tLB, tLBSnat, tLBUnSnat, tLBDone               = 15, 16, 17, 19
	tLBAff                                         = 18
	tNAT, tNATApply, tIngressDispatch, tIngressACL = 20, 21, 25, 30
	tNATFullSrc, tNATFullReply                     = 22, 23
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
	// LBRuntime carries observed state per load balancer (key ReportKey): backend health,
	// MACs of backends that are not VNics, and live connection counts for LeastConn.
	LBRuntime map[string]LBRuntime
	// SubnetGateways maps subnet name -> gateway IP (source address used by FullNAT).
	SubnetGateways map[string]string
	LBs            []servicev1alpha1.LoadBalancer
	NATs           []gwv1alpha1.NATGateway
}

// LBRuntime is the observed state of one load balancer.
type LBRuntime struct {
	Health map[string]bool   // backend ip -> healthy (absent = healthy)
	MACs   map[string]string // backend ip -> mac (backends that are not VNics)
	Conns  map[string]int64  // backend ip -> tracked connections
}

// AffinityTable is the table holding learned session-affinity flows. The agent must not
// delete it when it reprograms the other tables.
const AffinityTable = tLBAff

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

// VIPMAC is the virtual MAC answered by the ARP responder for an IP (VIPs, NAT external IPs).
func VIPMAC(ip string) string {
	var o [4]int
	fmt.Sscanf(ip, "%d.%d.%d.%d", &o[0], &o[1], &o[2], &o[3])
	return fmt.Sprintf("0a:5d:%02x:%02x:%02x:%02x", o[0], o[1], o[2], o[3])
}

// LBBackendOrder returns the stable per-LB ordering of backends used for affinity indexes.
func LBBackendOrder(lb servicev1alpha1.LoadBalancer) []servicev1alpha1.LBBackend {
	be := append([]servicev1alpha1.LBBackend(nil), lb.Spec.Backends...)
	sort.Slice(be, func(i, j int) bool { return be[i].IP < be[j].IP })
	return be
}

func lbWeights(lb servicev1alpha1.LoadBalancer, backends []servicev1alpha1.LBBackend, conns map[string]int64) map[string]int {
	w := map[string]int{}
	var maxC int64
	if lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmLeastConn {
		for _, be := range backends {
			if conns[be.IP] > maxC {
				maxC = conns[be.IP]
			}
		}
	}
	for _, be := range backends {
		weight := int(be.Weight)
		if lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmLeastConn {
			// weight shrinks as the backend holds more of the busiest backend's connections
			weight = int(int64(be.Weight) * 100 * (maxC - conns[be.IP] + 1) / (maxC + 1))
		}
		if weight < 1 {
			weight = 1
		}
		if weight > 65535 {
			weight = 65535
		}
		w[be.IP] = weight
	}
	return w
}

func (b *builder) loadBalancers() {
	lbs := append([]servicev1alpha1.LoadBalancer(nil), b.in.LBs...)
	sort.Slice(lbs, func(i, j int) bool { return lbs[i].Namespace+"/"+lbs[i].Name < lbs[j].Namespace+"/"+lbs[j].Name })
	gid := groupBase
	for idx, lb := range lbs {
		rep := b.report("LoadBalancer", lb.Namespace, lb.Name)
		if lb.Spec.Type == servicev1alpha1.LBTypeL7 {
			rep.Error = "L7 (HTTP) load balancing is not possible in OVS, which is L4 only; use an L4 LB in front of an L7 proxy VNF"
			continue
		}
		key := ReportKey("LoadBalancer", lb.Namespace, lb.Name)
		rt := b.in.LBRuntime[key]
		zone, zone2 := b.in.lbZoneBase()+2*idx, b.in.lbZoneBase()+2*idx+1
		b.out.LBZones[key] = zone
		vip := lb.Spec.Frontend.IP
		vmac := VIPMAC(vip)
		affinity := lb.Spec.SessionAffinity != nil && lb.Spec.SessionAffinity.Enabled
		method := "dp_hash"
		if lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmIPHash {
			method = "hash,fields(ip_src)"
		}
		if lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmMaglev {
			rep.Warnings = append(rep.Warnings, "Maglev is approximated by OVS dp_hash consistent selection")
		}
		if lb.Spec.HealthCheck == nil {
			rep.Warnings = append(rep.Warnings, "no healthCheck configured: backends are assumed healthy")
		}

		all := LBBackendOrder(lb) // index+1 is the stable affinity id
		type be struct {
			servicev1alpha1.LBBackend
			k   int
			mac string
		}
		var usable []be
		for i, x := range all {
			if x.Weight <= 0 {
				continue
			}
			mac := b.in.IPToMAC[x.IP]
			if mac == "" {
				mac = rt.MACs[x.IP]
			}
			if mac == "" {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("backend %s: MAC unknown, reached by flooding until it is learned", x.IP))
			}
			usable = append(usable, be{x, i + 1, mac})
		}
		var healthy []be
		for _, x := range usable {
			if h, ok := rt.Health[x.IP]; !ok || h {
				healthy = append(healthy, x)
			}
		}
		if len(usable) == 0 {
			rep.Error = "no backend with weight > 0"
			continue
		}
		if len(healthy) == 0 {
			healthy = usable
			rep.Warnings = append(rep.Warnings, "all backends are unhealthy: traffic is still sent to all of them (fail-open)")
		}
		hb := make([]servicev1alpha1.LBBackend, len(healthy))
		for i, x := range healthy {
			hb[i] = x.LBBackend
		}
		weights := lbWeights(lb, hb, rt.Conns)

		b.add("table=%d,priority=300,arp,arp_tpa=%s,arp_op=1,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:%s->eth_src,load:2->NXM_OF_ARP_OP[],move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[],set_field:%s->arp_sha,move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[],set_field:%s->arp_spa,in_port", tClassify, vip, vmac, vmac, vip)
		rep.Flows++

		ckVIP := b.newCookie(CookieRef{"LoadBalancer", lb.Namespace, lb.Name, "vip", "DNAT"})
		ckNew := b.newCookie(CookieRef{"LoadBalancer", lb.Namespace, lb.Name, "connections", "DNAT"})
		for _, fp := range lb.Spec.Frontend.Ports {
			proto := strings.ToLower(fp.Protocol)
			learnTimeout := 3600
			if affinity && lb.Spec.SessionAffinity.Timeout.Duration > 0 {
				learnTimeout = int(lb.Spec.SessionAffinity.Timeout.Duration.Seconds())
			}
			if learnTimeout < 1 {
				learnTimeout = 1
			}
			if learnTimeout > 65535 {
				learnTimeout = 65535
			}
			ipProto := 6
			switch proto {
			case "udp":
				ipProto = 17
			case "sctp":
				ipProto = 132
			}
			match := fmt.Sprintf("ip,%s,nw_dst=%s,tp_dst=%d,ct_state=-trk", proto, vip, fp.Port)
			direct := func(x be, next int) string {
				act := ""
				if x.mac != "" {
					act = fmt.Sprintf("set_field:%s->eth_dst,", x.mac)
				}
				return fmt.Sprintf("%sct(commit,table=%d,zone=%d,nat(dst=%s:%d))", act, next, zone, x.IP, fp.TargetPort)
			}
			for _, v := range []struct {
				name   string
				next   int
				prio   int
				inPort bool
			}{{"local", tLBDone, 100, false}, {"tunnel", tLBSnat, 105, true}} {
				var ports []int
				if v.inPort {
					ports = b.in.TunnelPorts
				} else {
					ports = []int{0}
				}
				if len(ports) == 0 {
					continue
				}
				gid++
				var buckets []string
				for _, x := range healthy {
					buckets = append(buckets, fmt.Sprintf("bucket=weight:%d,actions=%s", weights[x.IP], direct(x, v.next)))
				}
				b.out.Groups = append(b.out.Groups, Group{ID: gid, Def: fmt.Sprintf("type=select,selection_method=%s,%s", method, strings.Join(buckets, ","))})
				for _, tp := range ports {
					in := ""
					if v.inPort {
						in = fmt.Sprintf("in_port=%d,", tp)
					}
					if !affinity {
						b.add("cookie=0x%x,table=%d,priority=%d,%s%s,actions=group:%d", ckVIP, tLB, v.prio+10, in, match, gid)
						rep.Flows++
						continue
					}
					// affinity: look up the learned table first, then pin to the learned backend or select one
					b.add("cookie=0x%x,table=%d,priority=%d,%s%s,reg3=0,actions=load:1->NXM_NX_REG3[],resubmit(,%d),resubmit(,%d)", ckVIP, tLB, v.prio+40, in, match, tLBAff, tLB)
					b.add("cookie=0x%x,table=%d,priority=%d,%s%s,reg3=1,actions=group:%d", ckVIP, tLB, v.prio+10, in, match, gid)
					for _, x := range healthy {
						b.add("cookie=0x%x,table=%d,priority=%d,%s%s,reg3=1,reg2=%d,actions=%s", ckVIP, tLB, v.prio+20, in, match, x.k, direct(x, v.next))
						rep.Flows++
					}
					rep.Flows += 2
				}
			}
			if affinity {
				// Learn the client->backend mapping when the connection's first packet comes out of
				// DNAT as +new (learn inside a group bucket is not executed by OVS). The learned flow
				// matches the original VIP tuple and selects the backend by its stable index.
				for _, x := range usable {
					learn := fmt.Sprintf("learn(table=%d,idle_timeout=%d,priority=100,eth_type=0x800,ip_proto=%d,NXM_OF_IP_SRC[],nw_dst=%s,%s_dst=%d,load:%d->NXM_NX_REG2[])", tLBAff, learnTimeout, ipProto, vip, proto, fp.Port, x.k)
					m := fmt.Sprintf("ip,%s,ct_zone=%d,ct_state=+trk+new,nw_dst=%s,tp_dst=%d", proto, zone, x.IP, fp.TargetPort)
					b.add("cookie=0x%x,table=%d,priority=110,%s,actions=%s,ct_clear,resubmit(,%d)", ckNew, tLBDone, m, learn, tNAT)
					b.add("cookie=0x%x,table=%d,priority=110,%s,actions=%s,ct_clear,ct(commit,table=%d,zone=%d,nat(src=%s))", ckNew, tLBSnat, m, learn, tLBDone, zone2, vip)
					rep.Flows += 2
				}
			}
			for _, x := range usable {
				b.add("table=%d,priority=100,ip,%s,nw_src=%s,tp_src=%d,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tLB, proto, x.IP, fp.TargetPort, tLBDone, zone)
				b.add("table=%d,priority=110,ip,%s,nw_src=%s,tp_src=%d,nw_dst=%s,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tLB, proto, x.IP, fp.TargetPort, vip, tLBUnSnat, zone2)
				rep.Flows += 2
			}
		}
		// New-connection counters: the first packet of a connection recirculates as +new.
		b.add("cookie=0x%x,table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+new,actions=ct_clear,resubmit(,%d)", ckNew, tLBDone, zone, tNAT)
		// Full-NAT for clients that arrived through a tunnel: SNAT to the VIP so replies come back here.
		b.add("cookie=0x%x,table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+new,actions=ct_clear,ct(commit,table=%d,zone=%d,nat(src=%s))", ckNew, tLBSnat, zone, tLBDone, zone2, vip)
		// Established packets must re-apply the SNAT chosen for the first packet, otherwise the
		// backend sees a different source than in the SYN and resets the connection.
		b.add("table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+est,actions=ct_clear,ct(table=%d,zone=%d,nat)", tLBSnat, zone, tLBDone, zone2)
		b.add("table=%d,priority=100,ip,ct_zone=%d,actions=ct_clear,ct(table=%d,zone=%d,nat)", tLBUnSnat, zone2, tLBDone, zone)
		rep.Flows += 4
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

func containsIP(cidr, ip string) bool {
	_, n, err := net.ParseCIDR(cidr)
	return err == nil && net.ParseIP(ip) != nil && n.Contains(net.ParseIP(ip))
}

func (b *builder) natGateways() {
	nats := append([]gwv1alpha1.NATGateway(nil), b.in.NATs...)
	sort.Slice(nats, func(i, j int) bool { return nats[i].Namespace+"/"+nats[i].Name < nats[j].Namespace+"/"+nats[j].Name })
	vpcCIDRDone := false
	for idx, nat := range nats {
		rep := b.report("NATGateway", nat.Namespace, nat.Name)
		typ := nat.Spec.Type
		if typ == "" {
			typ = gwv1alpha1.NATTypeSNAT
		}
		doSNAT := typ == gwv1alpha1.NATTypeSNAT || typ == gwv1alpha1.NATTypeFullNAT
		doDNAT := typ == gwv1alpha1.NATTypeDNAT || typ == gwv1alpha1.NATTypeFullNAT
		if doDNAT && len(nat.Spec.PortForwards) == 0 {
			rep.Error = fmt.Sprintf("type %s needs spec.portForwards", typ)
			continue
		}
		key := ReportKey("NATGateway", nat.Namespace, nat.Name)
		zone := b.in.natZoneBase() + idx
		zoneD, zoneF := zone+30, zone+60
		b.out.NATZones[key] = zone
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
		ckSNAT := b.newCookie(CookieRef{"NATGateway", nat.Namespace, nat.Name, "translations", "SNAT"})
		ckTraffic := b.newCookie(CookieRef{"NATGateway", nat.Namespace, nat.Name, "traffic", "NAT"})

		// Replies / established packets arriving for the external IP.
		b.add("table=%d,priority=200,ip,ct_state=-trk,nw_dst=%s,actions=ct(table=%d,zone=%d,nat)", tNAT, ext, tNATApply, zone)
		b.add("table=%d,priority=150,ip,ct_zone=%d,ct_state=+trk+new,nw_dst=%s,actions=drop", tNATApply, zone, ext)
		b.add("table=%d,priority=200,ip,ct_zone=%d,ct_state=+trk+est,actions=ct_clear,resubmit(,%d)", tNATApply, zone, tIngressDispatch)
		rep.Flows += 3
		if doSNAT {
			for _, sn := range nat.Spec.SubnetRefs {
				cidr := b.in.Subnets[sn]
				if cidr == "" {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("subnet %q not found in the VPC", sn))
					continue
				}
				b.add("cookie=0x%x,table=%d,priority=200,ip,ct_state=-trk,nw_src=%s,actions=ct(table=%d,zone=%d,nat)", ckTraffic, tNAT, cidr, tNATApply, zone)
				b.add("cookie=0x%x,table=%d,priority=210,ip,ct_zone=%d,ct_state=+trk+new,nw_src=%s,actions=ct(commit,zone=%d,%s),ct_clear,resubmit(,%d)", ckSNAT, tNATApply, zone, cidr, zone, natSpec, tIngressDispatch)
				rep.Flows += 2
			}
		}
		if !doDNAT {
			continue
		}
		// DNAT port forwards: external IP:port -> internal IP:port. FullNAT additionally rewrites the
		// client's source to the target subnet's gateway so replies always come back through here.
		for _, pf := range nat.Spec.PortForwards {
			proto := strings.ToLower(pf.Protocol)
			if proto != "tcp" && proto != "udp" {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("port forward %q skipped: protocol %q", pf.Name, pf.Protocol))
				continue
			}
			gw := ""
			for sn, cidr := range b.in.Subnets {
				if containsIP(cidr, pf.InternalIP) {
					gw = b.in.SubnetGateways[sn]
				}
			}
			if gw == "" && typ == gwv1alpha1.NATTypeFullNAT {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("port forward %q skipped: %s is not in a VPC subnet", pf.Name, pf.InternalIP))
				continue
			}
			macAct := ""
			if mac := b.in.IPToMAC[pf.InternalIP]; mac != "" {
				macAct = fmt.Sprintf("set_field:%s->eth_dst,", mac)
			} else {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("port forward %q: %s has no VNic, its MAC is learned by flooding", pf.Name, pf.InternalIP))
			}
			in := fmt.Sprintf("%s,nw_dst=%s,tp_dst=%d", proto, ext, pf.ExternalPort)
			// entry (above the intra-VPC bypass, replies from the target may be addressed inside the VPC)
			b.add("cookie=0x%x,table=%d,priority=400,ip,%s,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", ckTraffic, tNAT, in, tNATApply, zoneD)
			// FullNAT: recirculate between the DNAT and SNAT commits (table 22), like the LB does.
			ctNext := tIngressDispatch
			if typ == gwv1alpha1.NATTypeFullNAT {
				ctNext = tNATFullSrc
				b.add("table=%d,priority=100,ip,ct_zone=%d,ct_state=+trk+new,nw_dst=%s,actions=ct_clear,ct(commit,table=%d,zone=%d,nat(src=%s))", tNATFullSrc, zoneD, pf.InternalIP, tIngressDispatch, zoneF, gw)
			}
			b.add("cookie=0x%x,table=%d,priority=220,ip,ct_zone=%d,ct_state=+trk+new,%s,actions=%sct(commit,table=%d,zone=%d,nat(dst=%s:%d))", ckSNAT, tNATApply, zoneD, in, macAct, ctNext, zoneD, pf.InternalIP, pf.InternalPort)
			// replies from the target
			reply := fmt.Sprintf("%s,nw_src=%s,tp_src=%d", proto, pf.InternalIP, pf.InternalPort)
			if typ == gwv1alpha1.NATTypeFullNAT {
				b.add("table=%d,priority=400,ip,%s,nw_dst=%s,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tNAT, reply, gw, tNATFullReply, zoneF)
			} else {
				b.add("table=%d,priority=400,ip,%s,ct_state=-trk,actions=ct(table=%d,zone=%d,nat)", tNAT, reply, tNATApply, zoneD)
			}
			rep.Flows += 4
		}
		b.add("table=%d,priority=200,ip,ct_zone=%d,ct_state=+trk+est+rpl,actions=ct_clear,resubmit(,%d)", tNATApply, zoneD, tIngressDispatch)
		if typ == gwv1alpha1.NATTypeFullNAT {
			// established forward packets re-apply the stored SNAT
			b.add("table=%d,priority=200,ip,ct_zone=%d,ct_state=+trk+est-rpl,actions=ct_clear,ct(table=%d,zone=%d,nat)", tNATApply, zoneD, tIngressDispatch, zoneF)
			b.add("table=%d,priority=100,ip,ct_zone=%d,actions=ct_clear,ct(table=%d,zone=%d,nat)", tNATFullReply, zoneF, tNATApply, zoneD)
		} else {
			b.add("table=%d,priority=200,ip,ct_zone=%d,ct_state=+trk+est-rpl,actions=ct_clear,resubmit(,%d)", tNATApply, zoneD, tIngressDispatch)
		}
	}
}
