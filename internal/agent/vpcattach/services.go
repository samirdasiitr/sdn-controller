package vpcattach

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/agent/ovsinfo"
	"github.com/sdncontroller/sdncontroller/internal/agent/pipeline"
)

const natRouteProto = "77" // marks routes owned by the agent so stale ones can be removed

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// programPipeline compiles the services of the VPC for this node, installs them atomically
// and reports what was programmed back to each object's status.
func (r *Reconciler) programPipeline(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, bound []pipeline.Port, tunnelPortNames map[string]bool, cidrs []string, isRouter bool) error {
	var fws servicev1alpha1.FirewallList
	var lbs servicev1alpha1.LoadBalancerList
	var nats gwv1alpha1.NATGatewayList
	var subnets vpcv1alpha1.SubnetList
	var vnics vpcv1alpha1.VNicList
	for _, l := range []client.ObjectList{&fws, &lbs, &nats, &subnets, &vnics} {
		if err := r.c.List(ctx, l, client.InNamespace(v.Namespace)); err != nil {
			return err
		}
	}

	in := pipeline.Input{VNI: int(v.Status.AllocatedVNI), VPCCIDRs: append([]string{v.Spec.CIDR}, v.Spec.SecondaryCIDRs...),
		Subnets: map[string]string{}, IPToMAC: map[string]string{}, Ports: bound}
	for _, sn := range subnets.Items {
		if sn.Spec.VPCRef == v.Name {
			in.Subnets[sn.Name] = sn.Spec.CIDR
		}
	}
	seen := map[string]bool{}
	for _, rt := range v.Status.Routes {
		if !seen[rt.Destination] {
			seen[rt.Destination] = true
			in.Routes = append(in.Routes, rt.Destination)
		}
	}
	sort.Strings(in.Routes)
	for _, vn := range vnics.Items {
		if vn.Status.IPAddress != "" && vn.Status.MACAddress != "" {
			if _, ok := in.Subnets[vn.Spec.SubnetRef]; ok {
				in.IPToMAC[vn.Status.IPAddress] = vn.Status.MACAddress
			}
		}
	}
	// Remote VNics: MAC -> tunnel port toward the node hosting them.
	in.RemoteMACs = map[string]int{}
	nodeTunnel := map[string]int{}
	for _, peer := range v.Status.AttachedNodes {
		if peer.NodeName == r.node || peer.TunnelIP == "" {
			continue
		}
		if of, err := r.ovs.OFPort(ctx, portName(v.Name, peer.TunnelIP)); err == nil {
			nodeTunnel[peer.NodeName] = of
		}
	}
	for _, vn := range vnics.Items {
		node := vn.Status.BoundNode
		if node == "" {
			node = vn.Spec.NodeName
		}
		if tp, ok := nodeTunnel[node]; ok && node != r.node && vn.Status.MACAddress != "" && vn.DeletionTimestamp.IsZero() {
			in.RemoteMACs[vn.Status.MACAddress] = tp
		}
	}
	for name := range tunnelPortNames {
		if of, err := r.ovs.OFPort(ctx, name); err == nil {
			in.TunnelPorts = append(in.TunnelPorts, of)
		}
	}
	sort.Ints(in.TunnelPorts)

	var myFWs []servicev1alpha1.Firewall
	for _, fw := range fws.Items {
		if fw.Spec.VPCRef == v.Name && fw.DeletionTimestamp.IsZero() && contains(fw.Status.Nodes, r.node) {
			myFWs = append(myFWs, fw)
		}
	}
	var myLBs []servicev1alpha1.LoadBalancer
	for _, lb := range lbs.Items {
		if lb.Spec.VPCRef == v.Name && lb.DeletionTimestamp.IsZero() && contains(lb.Status.Nodes, r.node) {
			myLBs = append(myLBs, lb)
		}
	}
	var myNATs []gwv1alpha1.NATGateway
	for _, n := range nats.Items {
		if n.Spec.VPCRef == v.Name && n.DeletionTimestamp.IsZero() && n.Status.ActiveNode == r.node {
			myNATs = append(myNATs, n)
		}
	}
	in.Firewalls, in.LBs, in.NATs = myFWs, myLBs, myNATs

	// Runtime state for load balancers: published backend health/MACs, live connection counts.
	vnicIPs := map[string]bool{}
	for ip := range in.IPToMAC {
		vnicIPs[ip] = true
	}
	var vpcLBs []servicev1alpha1.LoadBalancer
	for _, lb := range lbs.Items {
		if lb.Spec.VPCRef == v.Name && lb.DeletionTimestamp.IsZero() {
			vpcLBs = append(vpcLBs, lb)
		}
	}
	if isRouter {
		r.probeBackends(ctx, bridge, vpcLBs, vnicIPs)
	}
	in.SubnetGateways = map[string]string{}
	for _, sn := range subnets.Items {
		if sn.Spec.VPCRef == v.Name && sn.Status.Gateway != "" {
			in.SubnetGateways[sn.Name] = sn.Status.Gateway
		}
	}
	in.LBRuntime = map[string]pipeline.LBRuntime{}
	for _, lb := range myLBs {
		rt := pipeline.LBRuntime{Health: map[string]bool{}, MACs: map[string]string{}}
		for _, bs := range lb.Status.Backends {
			rt.Health[bs.IP] = bs.Healthy
			if bs.MAC != "" {
				rt.MACs[bs.IP] = bs.MAC
			}
		}
		in.LBRuntime[pipeline.ReportKey("LoadBalancer", lb.Namespace, lb.Name)] = rt
	}
	out := pipeline.Build(in)
	// LeastConn needs the live per-backend connection counts, which need the zone numbers.
	needConns := false
	for _, lb := range myLBs {
		needConns = needConns || lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmLeastConn
	}
	if needConns {
		for _, lb := range myLBs {
			key := pipeline.ReportKey("LoadBalancer", lb.Namespace, lb.Name)
			if z, ok := out.LBZones[key]; ok && lb.Spec.Algorithm == servicev1alpha1.LBAlgorithmLeastConn {
				rt := in.LBRuntime[key]
				rt.Conns = r.ovs.ConntrackByReplySrc(ctx, z)
				in.LBRuntime[key] = rt
			}
		}
		out = pipeline.Build(in)
	}

	// Learned session-affinity flows pin clients to backend indexes; drop them when the backend set changes.
	for _, lb := range myLBs {
		if lb.Spec.SessionAffinity == nil || !lb.Spec.SessionAffinity.Enabled {
			continue
		}
		key := pipeline.ReportKey("LoadBalancer", lb.Namespace, lb.Name)
		var sig []string
		for _, be := range pipeline.LBBackendOrder(lb) {
			h, ok := in.LBRuntime[key].Health[be.IP]
			sig = append(sig, fmt.Sprintf("%s/%d/%v", be.IP, be.Weight, !ok || h))
		}
		cur := strings.Join(sig, ",")
		if old, seen := r.lbSig[key]; seen && old != cur {
			r.log.Info("backend set changed, flushing session affinity", "lb", lb.Name)
			_ = r.ovs.DeleteFlowsMatching(ctx, bridge, "table=18,ip,nw_dst="+lb.Spec.Frontend.IP)
		}
		r.lbSig[key] = cur
	}

	groups := map[int]string{}
	for _, g := range out.Groups {
		groups[g.ID] = g.Def
	}
	applyErr := func() error {
		prune, err := r.ovs.SyncGroups(ctx, bridge, groups, 1000)
		if err != nil {
			return fmt.Errorf("programming groups: %w", err)
		}
		// Recreating flows resets their counters, so only reprogram when the compiled pipeline
		// changed, plus a periodic resync that heals manual drift.
		sum := sha256.Sum256([]byte(strings.Join(out.Flows, "\n")))
		sig := hex.EncodeToString(sum[:])
		if last, ok := r.applied[bridge]; !ok || last.sig != sig || time.Since(last.at) > 5*time.Minute {
			if err := r.ovs.ReplaceTables(ctx, bridge, pipelineTables, out.Flows); err != nil {
				return err
			}
			r.applied[bridge] = appliedState{sig: sig, at: time.Now()}
		}
		return prune()
	}()
	if applyErr != nil {
		r.log.Error("programming pipeline", "vpc", v.Name, "error", applyErr)
	}

	r.hostNAT(ctx, v, bridge, myNATs, applyErr == nil)
	r.reportEnforcement(ctx, bridge, out, myFWs, myLBs, myNATs, applyErr)
	return applyErr
}

// pipelineTables are replaced on every reconcile; table 18 (learned affinity flows) is not.
var pipelineTables = []int{0, 5, 10, 15, 16, 17, 19, 20, 21, 22, 23, 25, 30, 40}

type appliedState struct {
	sig string
	at  time.Time
}

type monoCounter struct{ base, last int64 }

// mono turns a resettable flow counter into a cumulative one: when the raw value goes down (flows
// were recreated) the previous value is folded into the base, and on first sight the base is
// seeded from the value already reported in status so agent restarts do not reset or double count.
func (r *Reconciler) mono(key string, raw, seed int64) int64 {
	m := r.counters[key]
	if m == nil {
		base := seed - raw
		if base < 0 {
			base = 0
		}
		m = &monoCounter{base: base}
		r.counters[key] = m
	}
	if raw < m.last {
		m.base += m.last
	}
	m.last = raw
	return m.base + raw
}

type agg struct{ packets, bytes int64 }

func (r *Reconciler) reportEnforcement(ctx context.Context, bridge string, out pipeline.Output,
	fws []servicev1alpha1.Firewall, lbs []servicev1alpha1.LoadBalancer, nats []gwv1alpha1.NATGateway, applyErr error) {
	// Sum the flow counters of each object by cookie.
	rule := map[string]map[string]agg{} // fw key -> "rule|action" -> counters
	traffic := map[string]agg{}         // lb/nat key -> main traffic counters
	newConns := map[string]int64{}      // lb/nat key -> new connections/translations
	if applyErr == nil {
		flows, _ := ovsinfo.GetFlows(ctx, bridge)
		for _, f := range flows {
			var ck uint64
			fmt.Sscanf(f.Cookie, "0x%x", &ck)
			ref, ok := out.Cookies[ck]
			if !ok {
				continue
			}
			key := pipeline.ReportKey(ref.Kind, ref.Namespace, ref.Name)
			switch {
			case ref.Kind == "Firewall":
				if rule[key] == nil {
					rule[key] = map[string]agg{}
				}
				a := rule[key][ref.Rule+"|"+ref.Action]
				a.packets += f.Packets
				a.bytes += f.Bytes
				rule[key][ref.Rule+"|"+ref.Action] = a
			case ref.Rule == "connections" || ref.Rule == "translations":
				newConns[key] += f.Packets
			default:
				a := traffic[key]
				a.packets += f.Packets
				a.bytes += f.Bytes
				traffic[key] = a
			}
		}
	}
	for i := range fws {
		fw := &fws[i]
		key := pipeline.ReportKey("Firewall", fw.Namespace, fw.Name)
		orig := fw.DeepCopy()
		prev := orig.Status.Enforcement[r.node]
		prevRules := map[string]servicev1alpha1.RuleCounter{}
		for _, rc := range prev.Rules {
			prevRules[rc.Name] = rc
		}
		e := enforcement(fw.Generation, out.Reports[key], applyErr)
		var total int64
		var names []string
		for k := range rule[key] {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			name, action, _ := strings.Cut(k, "|")
			a := rule[key][k]
			pk := prevRules[name]
			rc := servicev1alpha1.RuleCounter{Name: name, Action: action,
				Packets: r.mono("fw/"+key+"/"+name+"/p", a.packets, pk.Packets), Bytes: r.mono("fw/"+key+"/"+name+"/b", a.bytes, pk.Bytes)}
			total += rc.Packets
			e.Rules = append(e.Rules, rc)
		}
		e.MatchedPackets = total
		if applyErr != nil {
			e.Rules, e.MatchedPackets = prev.Rules, prev.MatchedPackets
		}
		if fw.Status.Enforcement == nil {
			fw.Status.Enforcement = map[string]servicev1alpha1.EnforcementStatus{}
		}
		fw.Status.Enforcement[r.node] = e
		r.patchStatus(ctx, fw, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], e))
	}
	for i := range lbs {
		lb := &lbs[i]
		key := pipeline.ReportKey("LoadBalancer", lb.Namespace, lb.Name)
		orig := lb.DeepCopy()
		prev := orig.Status.Enforcement[r.node]
		e := enforcement(lb.Generation, out.Reports[key], applyErr)
		if applyErr == nil {
			if z, ok := out.LBZones[key]; ok {
				e.Connections = r.ovs.ConntrackCount(ctx, z)
			}
			e.NewConnections = r.mono("lb/"+key+"/n", newConns[key], prev.NewConnections)
			e.Packets = r.mono("lb/"+key+"/p", traffic[key].packets, prev.Packets)
			e.Bytes = r.mono("lb/"+key+"/b", traffic[key].bytes, prev.Bytes)
		} else {
			e.NewConnections, e.Packets, e.Bytes = prev.NewConnections, prev.Packets, prev.Bytes
		}
		if lb.Status.Enforcement == nil {
			lb.Status.Enforcement = map[string]servicev1alpha1.EnforcementStatus{}
		}
		lb.Status.Enforcement[r.node] = e
		r.patchStatus(ctx, lb, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], e))
	}
	for i := range nats {
		n := &nats[i]
		key := pipeline.ReportKey("NATGateway", n.Namespace, n.Name)
		orig := n.DeepCopy()
		prev := orig.Status.Enforcement[r.node]
		se := enforcement(n.Generation, out.Reports[key], applyErr)
		e := gwv1alpha1.EnforcementStatus{ObservedGeneration: se.ObservedGeneration, Programmed: se.Programmed,
			Flows: se.Flows, Message: se.Message, Warnings: se.Warnings}
		if applyErr == nil {
			if z, ok := out.NATZones[key]; ok {
				e.Translations = r.ovs.ConntrackCount(ctx, z) + r.ovs.ConntrackCount(ctx, z+30)
			}
			e.NewTranslations = r.mono("nat/"+key+"/n", newConns[key], prev.NewTranslations)
			e.Packets = r.mono("nat/"+key+"/p", traffic[key].packets, prev.Packets)
			e.Bytes = r.mono("nat/"+key+"/b", traffic[key].bytes, prev.Bytes)
		} else {
			e.NewTranslations, e.Packets, e.Bytes = prev.NewTranslations, prev.Packets, prev.Bytes
		}
		if n.Status.Enforcement == nil {
			n.Status.Enforcement = map[string]gwv1alpha1.EnforcementStatus{}
		}
		n.Status.Enforcement[r.node] = e
		r.patchStatus(ctx, n, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], e))
	}
}

func enforcement(gen int64, rep *pipeline.Report, applyErr error) servicev1alpha1.EnforcementStatus {
	e := servicev1alpha1.EnforcementStatus{ObservedGeneration: gen}
	if rep != nil {
		e.Flows, e.Warnings = int32(rep.Flows), rep.Warnings
	}
	switch {
	case applyErr != nil:
		e.Message = applyErr.Error()
	case rep != nil && rep.Error != "":
		e.Message = rep.Error
	default:
		e.Programmed = true
	}
	return e
}

func (r *Reconciler) patchStatus(ctx context.Context, obj client.Object, orig client.Object, unchanged bool) {
	// Compare before patching: counters change, everything else is stable.
	if unchanged {
		return
	}
	if err := r.c.Status().Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
		r.log.Error("report enforcement", "object", obj.GetName(), "error", err)
	}
}

// hostNAT plumbs the host side of a NAT gateway: SNAT'ed replies carry the external IP as
// destination, so that address is routed back into the VPC bridge (where conntrack un-NATs
// them), and egress traffic from the external IP is masqueraded to the uplink.
func (r *Reconciler) hostNAT(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, nats []gwv1alpha1.NATGateway, ok bool) {
	want := map[string]bool{}
	if ok {
		for _, n := range nats {
			want[n.Spec.ExternalIP] = true
			_ = sh(ctx, "ip", "route", "replace", n.Spec.ExternalIP+"/32", "dev", bridge, "proto", natRouteProto)
			_ = sh(ctx, "ip", "neigh", "replace", n.Spec.ExternalIP, "lladdr", pipeline.VIPMAC(n.Spec.ExternalIP), "dev", bridge, "nud", "permanent")
			ensureRule(ctx, "nat", "POSTROUTING", false, "-s", n.Spec.ExternalIP+"/32", "-o", uplink(ctx, ""),
				"-m", "comment", "--comment", "sdn-natgw-"+v.Name, "-j", "MASQUERADE")
		}
	}
	// Remove routes and masquerade rules of NAT gateways that are gone or moved.
	if out, err := ovsOut(ctx, "ip", "route", "show", "dev", bridge, "proto", natRouteProto); err == nil {
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 {
				ip := strings.TrimSuffix(f[0], "/32")
				if !want[ip] {
					_ = sh(ctx, "ip", "route", "del", ip+"/32", "dev", bridge, "proto", natRouteProto)
					_ = sh(ctx, "ip", "neigh", "del", ip, "dev", bridge)
				}
			}
		}
	}
	for _, line := range listRules(ctx, "nat", "POSTROUTING", "sdn-natgw-"+v.Name) {
		keep := false
		for ip := range want {
			keep = keep || strings.Contains(line, "-s "+ip+"/32")
		}
		if !keep {
			_ = sh(ctx, "iptables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, strings.Fields(strings.TrimPrefix(line, "-A POSTROUTING "))...)...)
		}
	}
}
