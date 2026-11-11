// Package vpcattach programs the local dataplane for every VPC this node is
// attached to: per-VPC OVS bridge, tunnels with the VPC's VNI to the other
// attached nodes, subnet gateway addresses, route flows distributed through the
// Kubernetes API, and Internet Gateway NAT.
package vpcattach

import (
	"context"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

const tunnelPrefix = "sdn"

// Reconciler converges the node dataplane to VPC status.
type Reconciler struct {
	c    client.Client
	node string
	ovs  *pkgovs.Client
	log  *slog.Logger

	counters map[string]*monoCounter  // cumulative counters that survive flow recreation
	health   map[string]*backendState // LB backend probe state (router node)
	lbSig    map[string]string        // LB backend-set signature, to flush learned affinity flows
	applied  map[string]appliedState  // last programmed pipeline per bridge
}

func New(c client.Client, node string, ovs *pkgovs.Client, log *slog.Logger) *Reconciler {
	return &Reconciler{c: c, node: node, ovs: ovs, log: log.With("component", "vpcattach"),
		counters: map[string]*monoCounter{}, health: map[string]*backendState{}, lbSig: map[string]string{}, applied: map[string]appliedState{}}
}

// Reconcile applies all VPCs. Errors for one VPC do not stop the others.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	var vpcs vpcv1alpha1.VPCList
	if err := r.c.List(ctx, &vpcs); err != nil {
		return fmt.Errorf("listing VPCs: %w", err)
	}
	var errs []string
	for i := range vpcs.Items {
		if err := r.reconcileVPC(ctx, &vpcs.Items[i]); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", vpcs.Items[i].Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (r *Reconciler) attachment(v *vpcv1alpha1.VPC) *vpcv1alpha1.VPCNodeAttachment {
	for i := range v.Status.AttachedNodes {
		if v.Status.AttachedNodes[i].NodeName == r.node {
			return &v.Status.AttachedNodes[i]
		}
	}
	return nil
}

func (r *Reconciler) reconcileVPC(ctx context.Context, v *vpcv1alpha1.VPC) error {
	att := r.attachment(v)
	if att == nil || !v.DeletionTimestamp.IsZero() || v.Status.Phase == vpcv1alpha1.VPCPhaseFailed {
		return r.teardown(ctx, v, att)
	}
	bridge := att.Bridge
	if bridge == "" {
		return fmt.Errorf("attachment has no bridge name")
	}

	if err := r.ovs.CreateBridge(ctx, bridge, ""); err != nil {
		return err
	}
	_ = sh(ctx, "ovs-vsctl", "set", "bridge", bridge, "external_ids:sdn-managed=true")
	if v.Spec.MTU > 0 {
		_ = r.ovs.SetBridgeMTU(ctx, bridge, int(v.Spec.MTU))
	}
	if err := sh(ctx, "ip", "link", "set", bridge, "up"); err != nil {
		return err
	}

	var igws gwv1alpha1.InternetGatewayList
	if err := r.c.List(ctx, &igws, client.InNamespace(v.Namespace)); err != nil {
		return err
	}
	// The prober runs on one node per VPC (the Internet Gateway node, else the first attached node).
	isRouter := routerNode(v, igws.Items) == r.node

	var subnets vpcv1alpha1.SubnetList
	if err := r.c.List(ctx, &subnets, client.InNamespace(v.Namespace)); err != nil {
		return err
	}
	var cidrs []string
	vpcSubnets := map[string]bool{}
	for _, sn := range subnets.Items {
		if sn.Spec.VPCRef == v.Name {
			vpcSubnets[sn.Name] = true
		}
		if sn.Spec.VPCRef == v.Name && sn.Status.Phase == vpcv1alpha1.SubnetPhaseReady {
			cidrs = append(cidrs, sn.Spec.CIDR)
		}
	}
	// Routing is done by OVS flows (virtual gateways). The host only gets a VNic like any other member.
	wantAddrs := map[string]bool{}
	if addr := r.ensureHostVNic(ctx, v, bridge, subnets.Items); addr != "" {
		wantAddrs[addr] = true
	}
	r.pruneAddrs(ctx, bridge, wantAddrs)
	r.cleanupLegacyHostNAT(ctx, v, bridge)

	// Tunnels: one port per other attached node, all keyed with the VPC's VNI.
	wantPorts := map[string]bool{}
	tType := v.Spec.TunnelType
	if tType == "" {
		tType = vpcv1alpha1.TunnelTypeVXLAN
	}
	vni := v.Status.AllocatedVNI
	for _, peer := range v.Status.AttachedNodes {
		if peer.NodeName == r.node || peer.TunnelIP == "" {
			continue
		}
		port := portName(v.Name, peer.TunnelIP)
		wantPorts[port] = true
		var err error
		switch tType {
		case vpcv1alpha1.TunnelTypeVXLAN:
			err = r.ovs.AddVXLANPort(ctx, bridge, port, peer.TunnelIP, vni)
		case vpcv1alpha1.TunnelTypeGRE:
			err = r.ovs.AddGREPort(ctx, bridge, port, peer.TunnelIP)
			if err == nil {
				err = sh(ctx, "ovs-vsctl", "set", "interface", port, fmt.Sprintf("options:key=%d", vni))
			}
		case vpcv1alpha1.TunnelTypeIPIP:
			err = r.ovs.AddIPIPPort(ctx, bridge, port, att.TunnelIP, peer.TunnelIP)
		}
		if err != nil {
			r.log.Error("ensure tunnel", "vpc", v.Name, "peer", peer.NodeName, "error", err)
		}
	}
	if ports, err := r.ovs.ListPorts(ctx, bridge); err == nil {
		for _, p := range ports {
			if strings.HasPrefix(p, tunnelPrefix) && !wantPorts[p] {
				_ = r.ovs.DeletePort(ctx, bridge, p)
			}
		}
	}

	// Compile and install the pipeline: port security, firewalls, load balancers, NAT gateways.
	bound := r.bindVNics(ctx, v, bridge, vpcSubnets)
	if err := r.programPipeline(ctx, v, bridge, bound, wantPorts, cidrs, isRouter); err != nil {
		return err
	}

	return nil
}

// routerNode picks the node that routes for the VPC: the active Internet
// Gateway node when there is one, otherwise the first attached node.
func routerNode(v *vpcv1alpha1.VPC, igws []gwv1alpha1.InternetGateway) string {
	attached := map[string]bool{}
	first := ""
	for _, a := range v.Status.AttachedNodes {
		attached[a.NodeName] = true
		if first == "" || a.NodeName < first {
			first = a.NodeName
		}
	}
	for _, g := range igws {
		if g.Spec.VPCRef == v.Name && g.DeletionTimestamp.IsZero() &&
			g.Status.Phase == gwv1alpha1.GatewayPhaseReady && attached[g.Status.ActiveNode] {
			return g.Status.ActiveNode
		}
	}
	return first
}

func (r *Reconciler) teardown(ctx context.Context, v *vpcv1alpha1.VPC, att *vpcv1alpha1.VPCNodeAttachment) error {
	bridge := "br-" + v.Name
	if len(bridge) > 15 {
		bridge = bridge[:15]
	}
	if att != nil && att.Bridge != "" {
		bridge = att.Bridge
	}
	for _, line := range listRules(ctx, "nat", "POSTROUTING", "sdn-igw-"+v.Name) {
		_ = sh(ctx, "iptables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, strings.Fields(strings.TrimPrefix(line, "-A POSTROUTING "))...)...)
	}
	if ok, _ := r.ovs.BridgeExists(ctx, bridge); !ok || os.Getenv("SDN_KEEP_BRIDGES") != "" {
		return nil
	}
	if out, err := exec.CommandContext(ctx, "ovs-vsctl", "--if-exists", "get", "bridge", bridge, "external_ids:sdn-managed").Output(); err != nil || !strings.Contains(string(out), "true") {
		return nil
	}
	r.log.Info("VPC no longer attached, removing bridge", "vpc", v.Name, "bridge", bridge)
	for _, rule := range [][]string{{"-i", bridge, "-j", "ACCEPT"}, {"-o", bridge, "-j", "ACCEPT"}} {
		for sh(ctx, "iptables", append([]string{"-D", "FORWARD"}, rule...)...) == nil {
		}
	}
	return r.ovs.DeleteBridge(ctx, bridge)
}

func (r *Reconciler) pruneAddrs(ctx context.Context, bridge string, want map[string]bool) {
	ifc, err := net.InterfaceByName(bridge)
	if err != nil {
		return
	}
	addrs, _ := ifc.Addrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() {
			continue
		}
		if !want[ipn.String()] {
			_ = sh(ctx, "ip", "addr", "del", ipn.String(), "dev", bridge)
		}
	}
}

func portName(vpc, remote string) string {
	return fmt.Sprintf("%s%08x", tunnelPrefix, crc32.ChecksumIEEE([]byte(vpc+"/"+remote)))
}

func uplink(ctx context.Context, want string) string {
	if want != "" {
		if _, err := net.InterfaceByName(want); err == nil {
			return want
		}
	}
	out, err := exec.CommandContext(ctx, "ip", "route", "show", "default").Output()
	if err == nil {
		f := strings.Fields(string(out))
		for i, w := range f {
			if w == "dev" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return want
}

func sh(ctx context.Context, name string, args ...string) error {
	if out, err := exec.CommandContext(ctx, name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureRule(ctx context.Context, table, chain string, insert bool, spec ...string) {
	if sh(ctx, "iptables", append([]string{"-t", table, "-C", chain}, spec...)...) == nil {
		return
	}
	op := "-A"
	args := []string{"-t", table, op, chain}
	if insert {
		args = []string{"-t", table, "-I", chain, "1"}
	}
	_ = sh(ctx, "iptables", append(args, spec...)...)
}

func listRules(ctx context.Context, table, chain, comment string) []string {
	out, err := exec.CommandContext(ctx, "iptables", "-t", table, "-S", chain).Output()
	if err != nil {
		return nil
	}
	var rules []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, comment) {
			rules = append(rules, l)
		}
	}
	return rules
}
