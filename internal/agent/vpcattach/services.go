package vpcattach

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

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
func (r *Reconciler) programPipeline(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, bound []pipeline.Port, tunnelPortNames map[string]bool, cidrs []string) error {
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

	out := pipeline.Build(in)

	groups := map[int]string{}
	for _, g := range out.Groups {
		groups[g.ID] = g.Def
	}
	applyErr := func() error {
		prune, err := r.ovs.SyncGroups(ctx, bridge, groups, 1000)
		if err != nil {
			return fmt.Errorf("programming groups: %w", err)
		}
		if err := r.ovs.ReplaceFlows(ctx, bridge, out.Flows); err != nil {
			return err
		}
		return prune()
	}()
	if applyErr != nil {
		r.log.Error("programming pipeline", "vpc", v.Name, "error", applyErr)
	}

	r.hostNAT(ctx, v, bridge, myNATs, applyErr == nil)

	// Report enforcement and counters back to each object.
	var flowStats []ovsinfo.Flow
	if applyErr == nil {
		flowStats, _ = ovsinfo.GetFlows(ctx, bridge)
	}
	pkts := map[string]int64{}
	for _, f := range flowStats {
		var ck uint64
		fmt.Sscanf(f.Cookie, "0x%x", &ck)
		if ref, ok := out.Cookies[ck]; ok && ref.Kind == "Firewall" {
			pkts[pipeline.ReportKey(ref.Kind, ref.Namespace, ref.Name)] += f.Packets
		}
	}
	for i := range myFWs {
		fw := &myFWs[i]
		rep := out.Reports[pipeline.ReportKey("Firewall", fw.Namespace, fw.Name)]
		orig := fw.DeepCopy()
		if fw.Status.Enforcement == nil {
			fw.Status.Enforcement = map[string]servicev1alpha1.EnforcementStatus{}
		}
		fw.Status.Enforcement[r.node] = enforcement(fw.Generation, rep, applyErr, pkts[pipeline.ReportKey("Firewall", fw.Namespace, fw.Name)], 0)
		r.patchStatus(ctx, fw, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], fw.Status.Enforcement[r.node]))
	}
	for i := range myLBs {
		lb := &myLBs[i]
		key := pipeline.ReportKey("LoadBalancer", lb.Namespace, lb.Name)
		var conns int64
		if z, ok := out.LBZones[key]; ok && applyErr == nil {
			conns = r.ovs.ConntrackCount(ctx, z)
		}
		orig := lb.DeepCopy()
		if lb.Status.Enforcement == nil {
			lb.Status.Enforcement = map[string]servicev1alpha1.EnforcementStatus{}
		}
		lb.Status.Enforcement[r.node] = enforcement(lb.Generation, out.Reports[key], applyErr, 0, conns)
		r.patchStatus(ctx, lb, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], lb.Status.Enforcement[r.node]))
	}
	for i := range myNATs {
		n := &myNATs[i]
		key := pipeline.ReportKey("NATGateway", n.Namespace, n.Name)
		var tr int64
		if z, ok := out.NATZones[key]; ok && applyErr == nil {
			tr = r.ovs.ConntrackCount(ctx, z)
		}
		e := enforcement(n.Generation, out.Reports[key], applyErr, 0, 0)
		orig := n.DeepCopy()
		if n.Status.Enforcement == nil {
			n.Status.Enforcement = map[string]gwv1alpha1.EnforcementStatus{}
		}
		n.Status.Enforcement[r.node] = gwv1alpha1.EnforcementStatus{ObservedGeneration: e.ObservedGeneration, Programmed: e.Programmed,
			Flows: e.Flows, Translations: tr, Message: e.Message, Warnings: e.Warnings}
		r.patchStatus(ctx, n, orig, reflect.DeepEqual(orig.Status.Enforcement[r.node], n.Status.Enforcement[r.node]))
	}
	return applyErr
}

func enforcement(gen int64, rep *pipeline.Report, applyErr error, packets, conns int64) servicev1alpha1.EnforcementStatus {
	e := servicev1alpha1.EnforcementStatus{ObservedGeneration: gen, MatchedPackets: packets, Connections: conns}
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
			if t := n.Spec.Type; t != "" && t != gwv1alpha1.NATTypeSNAT {
				continue
			}
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
