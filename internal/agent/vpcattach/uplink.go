package vpcattach

import (
	"context"
	"fmt"
	"hash/crc32"
	"net"
	"strings"
	"time"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/agent/pipeline"
)

func extBridgeName(vpc string) string {
	b := "brx-" + vpc
	if len(b) > 15 {
		b = b[:15]
	}
	return b
}

func patchNames(vpc string) (onVPC, onExt string) {
	h := crc32.ChecksumIEEE([]byte(vpc))
	return fmt.Sprintf("pv%08x", h), fmt.Sprintf("px%08x", h)
}

// ensureUplink plugs the Internet Gateway's physical NIC into an OVS bridge owned by the agent,
// connects it to the VPC bridge with a patch port and programs the external bridge. The VPC's
// own pipeline does routing and NAT; the external bridge only moves frames. It returns the
// uplink description for the pipeline compiler.
func (r *Reconciler) ensureUplink(ctx context.Context, v *vpcv1alpha1.VPC, bridge string, igw *gwv1alpha1.InternetGateway, nats []gwv1alpha1.NATGateway, in pipeline.Input) (*pipeline.Uplink, error) {
	if len(igw.Spec.ExternalIPs) == 0 || igw.Spec.ExternalInterface == "" || igw.Spec.ExternalGateway == "" {
		return nil, fmt.Errorf("InternetGateway %s needs externalInterface, externalIPs and externalGateway for the OVS uplink", igw.Name)
	}
	ext := extBridgeName(v.Name)
	if err := r.ovs.CreateBridge(ctx, ext, ""); err != nil {
		return nil, err
	}
	_ = sh(ctx, "ovs-vsctl", "set", "bridge", ext, "external_ids:sdn-managed=true")
	iface := igw.Spec.ExternalInterface
	if err := sh(ctx, "ovs-vsctl", "--may-exist", "add-port", ext, iface); err != nil {
		return nil, err
	}
	_ = sh(ctx, "ip", "link", "set", iface, "up")
	_ = sh(ctx, "ip", "link", "set", ext, "up")
	prefix := igw.Spec.ExternalPrefixLength
	if prefix == 0 {
		prefix = 24
	}
	extIP := igw.Spec.ExternalIPs[0]
	if err := sh(ctx, "ip", "addr", "replace", fmt.Sprintf("%s/%d", extIP, prefix), "dev", ext); err != nil {
		return nil, err
	}
	pv, px := patchNames(v.Name)
	if err := sh(ctx, "ovs-vsctl", "--may-exist", "add-port", bridge, pv, "--", "set", "interface", pv, "type=patch", "options:peer="+px); err != nil {
		return nil, err
	}
	if err := sh(ctx, "ovs-vsctl", "--may-exist", "add-port", ext, px, "--", "set", "interface", px, "type=patch", "options:peer="+pv); err != nil {
		return nil, err
	}
	patchVPC, err := r.ovs.OFPort(ctx, pv)
	if err != nil {
		return nil, err
	}
	patchExt, err := r.ovs.OFPort(ctx, px)
	if err != nil {
		return nil, err
	}
	upl, err := r.ovs.OFPort(ctx, iface)
	if err != nil {
		return nil, err
	}
	// resolve the upstream router's MAC with the host stack (one-off ARP on the external bridge)
	mac := neighborMAC(ctx, ext, igw.Spec.ExternalGateway)
	if mac == "" {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort(igw.Spec.ExternalGateway, "53"), time.Second); err == nil {
			c.Close()
		}
		time.Sleep(300 * time.Millisecond)
		mac = neighborMAC(ctx, ext, igw.Spec.ExternalGateway)
	}
	if mac == "" {
		return nil, fmt.Errorf("cannot resolve upstream router %s on %s", igw.Spec.ExternalGateway, ext)
	}
	var natIPs []string
	for _, n := range nats {
		natIPs = append(natIPs, n.Spec.ExternalIP)
	}
	flows := pipeline.ExternalFlows(upl, patchExt, extIP, natIPs, pipeline.IGWZone(in))
	if err := r.ovs.ReplaceFlows(ctx, ext, flows); err != nil {
		return nil, err
	}
	extMAC := ""
	if ifc, err := net.InterfaceByName(ext); err == nil {
		extMAC = strings.ToLower(ifc.HardwareAddr.String())
	}
	return &pipeline.Uplink{ActiveHere: true, PatchPort: patchVPC, ExtMAC: extMAC, UpstreamMAC: mac, ExtIP: extIP}, nil
}

// removeUplink releases the physical NIC when this node is no longer the active gateway.
func (r *Reconciler) removeUplink(ctx context.Context, v *vpcv1alpha1.VPC, bridge string) {
	ext := extBridgeName(v.Name)
	if ok, _ := r.ovs.BridgeExists(ctx, ext); !ok {
		return
	}
	if out, err := ovsOut(ctx, "ovs-vsctl", "--if-exists", "get", "bridge", ext, "external_ids:sdn-managed"); err != nil || !strings.Contains(out, "true") {
		return
	}
	pv, _ := patchNames(v.Name)
	_ = r.ovs.DeletePort(ctx, bridge, pv)
	_ = r.ovs.DeleteBridge(ctx, ext)
}
