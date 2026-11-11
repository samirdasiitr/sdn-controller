package vpcattach

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/agent/pipeline"
)

// ovsOut runs an OVS command and returns trimmed stdout.
func ovsOut(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

// macTail returns the last five bytes of a MAC. Hypervisor tap devices flip the
// first byte (libvirt: 52:54:00:.. guest -> fe:54:00:.. tap), so match on the rest.
func macTail(mac string) string {
	m := strings.ToLower(strings.TrimSpace(strings.Trim(mac, `"`)))
	if len(m) != 17 {
		return ""
	}
	return m[3:]
}

// portsByMAC maps macTail -> OVS interface name for all local interfaces.
func portsByMAC(ctx context.Context) map[string]string {
	out, err := ovsOut(ctx, "ovs-vsctl", "--bare", "--columns=name,mac_in_use", "list", "interface")
	res := map[string]string{}
	if err != nil {
		return res
	}
	// --bare output prints one column per line, records separated by blank lines.
	for _, rec := range strings.Split(out, "\n\n") {
		f := strings.Split(strings.TrimSpace(rec), "\n")
		if len(f) >= 2 {
			if t := macTail(f[1]); t != "" {
				res[t] = strings.TrimSpace(f[0])
			}
		}
	}
	return res
}

// bindVNics binds VNics of this VPC that have a local port, returns the port
// security flows to install on the VPC bridge, and reports status.
func (r *Reconciler) bindVNics(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, subnetNames map[string]bool) []pipeline.Port {
	var vnics vpcv1alpha1.VNicList
	if err := r.c.List(ctx, &vnics, client.InNamespace(v.Namespace)); err != nil {
		r.log.Error("list VNics", "error", err)
		return nil
	}
	ports := portsByMAC(ctx)
	var bound []pipeline.Port
	for i := range vnics.Items {
		vn := &vnics.Items[i]
		if !subnetNames[vn.Spec.SubnetRef] || vn.Status.MACAddress == "" || vn.Status.IPAddress == "" || !vn.DeletionTimestamp.IsZero() {
			continue
		}
		if vn.Spec.NodeName != "" && vn.Spec.NodeName != r.node {
			continue
		}
		port, found := ports[macTail(vn.Status.MACAddress)]
		if !found {
			if vn.Status.BoundNode == r.node {
				r.reportUnbound(ctx, vn)
			}
			continue
		}
		cur := bridge // the bridge's own local port has no Port row (port-to-br fails for it)
		if port != bridge {
			var err error
			if cur, err = ovsOut(ctx, "ovs-vsctl", "port-to-br", port); err != nil {
				continue
			}
		}
		if cur != bridge {
			r.log.Info("moving VNic port to VPC bridge", "vnic", vn.Name, "port", port, "from", cur, "to", bridge)
			if err := sh(ctx, "ovs-vsctl", "--if-exists", "del-port", cur, port); err != nil {
				r.log.Error("detach port", "error", err)
				continue
			}
			if err := sh(ctx, "ovs-vsctl", "--may-exist", "add-port", bridge, port); err != nil {
				r.log.Error("attach port", "error", err)
				continue
			}
		}
		if port != bridge {
			_ = sh(ctx, "ovs-vsctl", "set", "interface", port, "external_ids:sdn-vnic="+vn.Namespace+"/"+vn.Name)
		}
		ofs, err := ovsOut(ctx, "ovs-vsctl", "get", "interface", port, "ofport")
		ofport, perr := strconv.Atoi(ofs)
		if err != nil || perr != nil || ofport <= 0 {
			continue
		}
		bound = append(bound, pipeline.Port{
			Key: vn.Namespace + "/" + vn.Name, IP: vn.Status.IPAddress, MAC: vn.Status.MACAddress, Subnet: vn.Spec.SubnetRef,
			FirewallRef: vn.Spec.FirewallRef, OFPort: ofport, Infra: vn.Spec.Attachment != nil && vn.Spec.Attachment.Kind == vpcv1alpha1.VNicAttachOther && vn.Spec.Attachment.Name == "host", PortSecurity: vn.Spec.PortSecurity == nil || *vn.Spec.PortSecurity,
		})
		r.reportBound(ctx, vn, bridge, port, int32(ofport))
	}
	return bound
}

func (r *Reconciler) reportBound(ctx context.Context, vn *vpcv1alpha1.VNic, bridge, port string, ofport int32) {
	s := &vn.Status
	if s.Phase == vpcv1alpha1.VNicPhaseBound && s.BoundNode == r.node && s.Bridge == bridge && s.OVSPort == port && s.OFPort == ofport {
		return
	}
	orig := vn.DeepCopy()
	s.Phase, s.BoundNode, s.Bridge, s.OVSPort, s.OFPort = vpcv1alpha1.VNicPhaseBound, r.node, bridge, port, ofport
	if err := r.c.Status().Patch(ctx, vn, client.MergeFrom(orig)); err != nil {
		r.log.Error("report VNic bound", "vnic", vn.Name, "error", err)
	}
}

func (r *Reconciler) reportUnbound(ctx context.Context, vn *vpcv1alpha1.VNic) {
	orig := vn.DeepCopy()
	s := &vn.Status
	s.Phase, s.BoundNode, s.Bridge, s.OVSPort, s.OFPort = vpcv1alpha1.VNicPhaseAllocated, "", "", "", 0
	if err := r.c.Status().Patch(ctx, vn, client.MergeFrom(orig)); err != nil {
		r.log.Error("report VNic unbound", "vnic", vn.Name, "error", err)
	}
}
