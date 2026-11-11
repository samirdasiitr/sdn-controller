package vpcattach

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
)

// ensureHostVNic gives the node itself a VNic in the VPC (infrastructure: used for health probes
// and operator access). The host is just another member of a subnet: its address is allocated by
// IPAM, the bridge's local port takes the VNic's MAC, and every other subnet is reached through the
// OVS gateway. It returns the address/prefix configured on the bridge ("" until the VNic is ready).
func (r *Reconciler) ensureHostVNic(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, subnets []vpcv1alpha1.Subnet) string {
	var ready []vpcv1alpha1.Subnet
	for _, sn := range subnets {
		if sn.Spec.VPCRef == v.Name && sn.Status.Phase == vpcv1alpha1.SubnetPhaseReady && sn.DeletionTimestamp.IsZero() {
			ready = append(ready, sn)
		}
	}
	if len(ready) == 0 {
		return ""
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Name < ready[j].Name })
	name := v.Name + "-host-" + r.node
	var vn vpcv1alpha1.VNic
	err := r.c.Get(ctx, client.ObjectKey{Namespace: v.Namespace, Name: name}, &vn)
	if apierrors.IsNotFound(err) {
		off := false
		vn = vpcv1alpha1.VNic{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: v.Namespace},
			Spec: vpcv1alpha1.VNicSpec{
				SubnetRef: ready[0].Name, NodeName: r.node, PortSecurity: &off,
				Attachment: &vpcv1alpha1.VNicAttachment{Kind: vpcv1alpha1.VNicAttachOther, Name: "host"},
			},
		}
		if err := r.c.Create(ctx, &vn); err != nil && !apierrors.IsAlreadyExists(err) {
			r.log.Error("create host VNic", "error", err)
		}
		return ""
	}
	if err != nil || vn.Status.IPAddress == "" || vn.Status.MACAddress == "" {
		return ""
	}
	if ifc, err := net.InterfaceByName(bridge); err == nil && !strings.EqualFold(ifc.HardwareAddr.String(), vn.Status.MACAddress) {
		_ = sh(ctx, "ovs-vsctl", "set", "bridge", bridge, "other-config:hwaddr="+vn.Status.MACAddress)
	}
	addr := fmt.Sprintf("%s/%d", vn.Status.IPAddress, vn.Status.PrefixLength)
	if err := sh(ctx, "ip", "addr", "replace", addr, "dev", bridge); err != nil {
		r.log.Error("set host address", "addr", addr, "error", err)
		return ""
	}
	for _, c := range append([]string{v.Spec.CIDR}, v.Spec.SecondaryCIDRs...) {
		if vn.Status.Gateway != "" {
			_ = sh(ctx, "ip", "route", "replace", c, "via", vn.Status.Gateway, "dev", bridge, "proto", "78")
		}
	}
	return addr
}

// cleanupLegacyHostNAT removes iptables/route state left by earlier agent versions that did
// gateway forwarding and NAT in the host kernel instead of OVS.
func (r *Reconciler) cleanupLegacyHostNAT(ctx context.Context, v *vpcv1alpha1.VPC, bridge string) {
	for _, comment := range []string{"sdn-igw-" + v.Name, "sdn-natgw-" + v.Name} {
		for _, line := range listRules(ctx, "nat", "POSTROUTING", comment) {
			_ = sh(ctx, "iptables", append([]string{"-t", "nat", "-D", "POSTROUTING"}, strings.Fields(strings.TrimPrefix(line, "-A POSTROUTING "))...)...)
		}
	}
	for _, rule := range [][]string{{"-i", bridge, "-j", "ACCEPT"}, {"-o", bridge, "-j", "ACCEPT"}} {
		for sh(ctx, "iptables", append([]string{"-D", "FORWARD"}, rule...)...) == nil {
		}
	}
	if out, err := ovsOut(ctx, "ip", "route", "show", "dev", bridge, "proto", natRouteProto); err == nil {
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 {
				ip := strings.TrimSuffix(f[0], "/32")
				_ = sh(ctx, "ip", "route", "del", ip+"/32", "dev", bridge, "proto", natRouteProto)
				_ = sh(ctx, "ip", "neigh", "del", ip, "dev", bridge)
			}
		}
	}
}
