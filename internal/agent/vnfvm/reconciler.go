package vnfvm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	nfvv1alpha1 "github.com/sdncontroller/sdncontroller/api/nfv/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
)

const (
	defaultBase = "/var/lib/libvirt/images/guests/base.img"
	workDir     = "/var/lib/libvirt/images/vnf"
)

// Reconciler makes the libvirt guests on this node match the VM-type VNF instances scheduled here.
type Reconciler struct {
	c    client.Client
	node string
	log  *slog.Logger
	key  string
	mu   sync.Mutex
}

// New creates the reconciler. authorizedKey (optional) is installed for the "ubuntu" user in guests.
func New(c client.Client, node, authorizedKey string, log *slog.Logger) *Reconciler {
	return &Reconciler{c: c, node: node, key: authorizedKey, log: log.With("component", "vnfvm")}
}

func virsh(ctx context.Context, args ...string) (string, error) {
	b, err := exec.CommandContext(ctx, "virsh", append([]string{"-c", "qemu:///system"}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

func domainName(ns, inst string) string { return "vnf-" + ns + "-" + inst }

// Reconcile creates missing guests and removes guests of VNFs/instances that no longer exist.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	if _, err := exec.LookPath("virsh"); err != nil {
		return nil // image without libvirt tooling: VNF guests are not supported on this node
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var vnfs nfvv1alpha1.VirtualNetworkFunctionList
	if err := r.c.List(ctx, &vnfs); err != nil {
		return fmt.Errorf("list VNFs: %w", err)
	}
	desired := map[string]bool{}
	var errs []string
	for i := range vnfs.Items {
		v := &vnfs.Items[i]
		if v.Spec.Type != nfvv1alpha1.VNFTypeVM || !v.DeletionTimestamp.IsZero() {
			continue
		}
		for _, inst := range v.Status.Instances {
			if inst.Node != r.node {
				continue
			}
			dom := domainName(v.Namespace, inst.Name)
			desired[dom] = true
			if err := r.ensure(ctx, v, inst, dom); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", dom, err))
			}
		}
	}
	r.prune(ctx, desired)
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (r *Reconciler) ensure(ctx context.Context, v *nfvv1alpha1.VirtualNetworkFunction, inst nfvv1alpha1.VNFInstanceStatus, dom string) error {
	if state, err := virsh(ctx, "domstate", dom); err == nil {
		if strings.Contains(state, "shut off") {
			_, err = virsh(ctx, "start", dom)
			return err
		}
		return nil
	}

	// Build NICs from this instance's VNics (all must have an identity before we boot).
	var vnics vpcv1alpha1.VNicList
	if err := r.c.List(ctx, &vnics, client.InNamespace(v.Namespace)); err != nil {
		return err
	}
	byName := map[string]vpcv1alpha1.VNic{}
	for _, vn := range vnics.Items {
		if vn.Spec.Attachment != nil && vn.Spec.Attachment.Kind == vpcv1alpha1.VNicAttachVNF && vn.Spec.Attachment.Name == inst.Name {
			byName[vn.Name] = vn
		}
	}
	var nics []NIC
	for _, itf := range v.Spec.Interfaces {
		if itf.SubnetRef == "" {
			continue
		}
		vn, ok := byName[inst.Name+"-"+itf.Name]
		if !ok || vn.Status.IPAddress == "" || vn.Status.MACAddress == "" {
			return fmt.Errorf("VNic for interface %q has no address yet", itf.Name)
		}
		bridge, err := r.bridgeFor(ctx, v.Namespace, vn.Status.VPCRef)
		if err != nil {
			return err
		}
		nics = append(nics, NIC{MAC: vn.Status.MACAddress, IP: vn.Status.IPAddress, Gateway: vn.Status.Gateway,
			PrefixLen: int(vn.Status.PrefixLength), MTU: int(vn.Status.MTU), Bridge: bridge})
	}
	if len(nics) == 0 {
		return fmt.Errorf("no interfaces with subnetRef")
	}

	cfg := v.Spec.VMConfig
	vm := VM{Domain: dom, Hostname: inst.Name, MemoryMB: 512, VCPUs: 1, NICs: nics, AuthorizedKey: r.key}
	if cfg != nil {
		if cfg.MemoryMB > 0 {
			vm.MemoryMB = int(cfg.MemoryMB)
		}
		if cfg.VCPUs > 0 {
			vm.VCPUs = int(cfg.VCPUs)
		}
		vm.Packages, vm.Commands = cfg.Packages, cfg.Commands
	}
	base := defaultBase
	if cfg != nil && cfg.Image != "" {
		if _, err := os.Stat(cfg.Image); err == nil {
			base = cfg.Image
		} else {
			r.log.Warn("VNF image not found on node, using default guest image", "image", cfg.Image, "default", defaultBase)
		}
	}
	if _, err := os.Stat(base); err != nil {
		return fmt.Errorf("base image %s not found on node", base)
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	vm.DiskPath = filepath.Join(workDir, dom+".qcow2")
	vm.Seed = filepath.Join(workDir, dom+"-seed.iso")
	tmp, err := os.MkdirTemp("", "vnf-seed")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for name, content := range map[string]string{"meta-data": vm.metaData(), "user-data": vm.userData(), "network-config": vm.networkConfig()} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	steps := [][]string{
		{"genisoimage", "-quiet", "-output", vm.Seed, "-volid", "cidata", "-joliet", "-rock",
			filepath.Join(tmp, "meta-data"), filepath.Join(tmp, "user-data"), filepath.Join(tmp, "network-config")},
		{"qemu-img", "create", "-q", "-f", "qcow2", "-b", base, "-F", "qcow2", vm.DiskPath, "10G"},
	}
	for _, s := range steps {
		if out, err := exec.CommandContext(ctx, s[0], s[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w (%s)", s[0], err, strings.TrimSpace(string(out)))
		}
	}
	xml := filepath.Join(tmp, "domain.xml")
	if err := os.WriteFile(xml, []byte(vm.domainXML()), 0o644); err != nil {
		return err
	}
	if out, err := virsh(ctx, "define", xml); err != nil {
		return fmt.Errorf("virsh define: %w (%s)", err, out)
	}
	if out, err := virsh(ctx, "start", dom); err != nil {
		return fmt.Errorf("virsh start: %w (%s)", err, out)
	}
	_, _ = virsh(ctx, "autostart", dom)
	r.log.Info("started VNF guest", "domain", dom, "nics", len(nics), "base", base)
	return nil
}

func (r *Reconciler) bridgeFor(ctx context.Context, ns, vpcName string) (string, error) {
	var vpc vpcv1alpha1.VPC
	if err := r.c.Get(ctx, client.ObjectKey{Namespace: ns, Name: vpcName}, &vpc); err != nil {
		return "", fmt.Errorf("get VPC %q: %w", vpcName, err)
	}
	for _, a := range vpc.Status.AttachedNodes {
		if a.NodeName == r.node && a.Bridge != "" {
			return a.Bridge, nil
		}
	}
	return "", fmt.Errorf("node %s is not attached to VPC %s", r.node, vpcName)
}

// prune removes agent-created guests (identified by the marker description) that are no longer desired.
func (r *Reconciler) prune(ctx context.Context, desired map[string]bool) {
	out, err := virsh(ctx, "list", "--all", "--name")
	if err != nil {
		return
	}
	for _, dom := range strings.Fields(out) {
		if !strings.HasPrefix(dom, "vnf-") || desired[dom] {
			continue
		}
		if desc, err := virsh(ctx, "desc", dom); err != nil || strings.TrimSpace(desc) != marker {
			continue // not ours
		}
		r.log.Info("removing VNF guest", "domain", dom)
		_, _ = virsh(ctx, "destroy", dom)
		_, _ = virsh(ctx, "undefine", dom, "--nvram")
		_ = os.Remove(filepath.Join(workDir, dom+".qcow2"))
		_ = os.Remove(filepath.Join(workDir, dom+"-seed.iso"))
	}
}
