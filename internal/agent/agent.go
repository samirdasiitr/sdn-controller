/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha1 "github.com/sdncontroller/sdncontroller/api/infra/v1alpha1"
	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	nfvv1alpha1 "github.com/sdncontroller/sdncontroller/api/nfv/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	agentbond "github.com/sdncontroller/sdncontroller/internal/agent/bond"
	agentdpu "github.com/sdncontroller/sdncontroller/internal/agent/dpu"
	agentlibvirt "github.com/sdncontroller/sdncontroller/internal/agent/libvirt"
	agentnfv "github.com/sdncontroller/sdncontroller/internal/agent/nfv"
	agentoffload "github.com/sdncontroller/sdncontroller/internal/agent/offload"
	agentovs "github.com/sdncontroller/sdncontroller/internal/agent/ovs"
	agenttunnel "github.com/sdncontroller/sdncontroller/internal/agent/tunnel"
	agentvnfvm "github.com/sdncontroller/sdncontroller/internal/agent/vnfvm"
	agentvpc "github.com/sdncontroller/sdncontroller/internal/agent/vpcattach"
	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(infrav1alpha1.AddToScheme(scheme))
	utilruntime.Must(nfvv1alpha1.AddToScheme(scheme))
	utilruntime.Must(vpcv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gwv1alpha1.AddToScheme(scheme))
}

// Agent orchestrates all node agent subsystems: OVS, tunnels, offload, bonds, NFV, libvirt, and DPU.
type Agent struct {
	cfg Config
	log *slog.Logger

	k8sClient client.Client

	OVSManager     *agentovs.OVSManager
	TunnelManager  *agenttunnel.TunnelManager
	OffloadManager *agentoffload.OffloadManager
	BondManager    *agentbond.BondManager
	NFVAgent       *agentnfv.NFVAgent
	LibvirtManager *agentlibvirt.LibvirtManager
	DPUManager     *agentdpu.DPUManager
	VPCAttach      *agentvpc.Reconciler
	VNFVM          *agentvnfvm.Reconciler

	cancel context.CancelFunc
}

// New creates a new Agent with the given configuration and logger.
func New(cfg Config, logger *slog.Logger) *Agent {
	return &Agent{
		cfg: cfg,
		log: logger.With("component", "agent", "node", cfg.NodeName),
	}
}

// Start initializes all subsystems and begins watching Kubernetes CRDs.
func (a *Agent) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.log.InfoContext(ctx, "starting node agent",
		"node", a.cfg.NodeName, "ovsSocket", a.cfg.OVSSocket)

	// Build Kubernetes client.
	k8sClient, err := a.buildK8sClient()
	if err != nil {
		cancel()
		return fmt.Errorf("building Kubernetes client: %w", err)
	}
	a.k8sClient = k8sClient

	// Initialize OVS client and manager.
	ovsClient := pkgovs.NewClient()
	a.OVSManager = agentovs.NewOVSManager(ovsClient, a.log)
	a.TunnelManager = agenttunnel.NewTunnelManager(ovsClient, a.log)
	a.OffloadManager = agentoffload.NewOffloadManager(a.log)
	a.BondManager = agentbond.NewBondManager(a.log)
	a.NFVAgent = agentnfv.NewNFVAgent(ovsClient, "br-int", a.log)
	a.LibvirtManager = agentlibvirt.NewLibvirtManager(ovsClient, a.log)
	a.DPUManager = agentdpu.NewDPUManager(a.log)
	a.VPCAttach = agentvpc.New(a.k8sClient, a.cfg.NodeName, ovsClient, a.log)
	a.VNFVM = agentvnfvm.New(a.k8sClient, a.cfg.NodeName, os.Getenv("VNF_SSH_AUTHORIZED_KEY"), a.log)

	// Phase 1: Discover hardware.
	a.log.InfoContext(ctx, "discovering hardware capabilities")
	caps := a.OffloadManager.DetectCapabilities()
	a.log.InfoContext(ctx, "hardware discovery complete",
		"hasMellanox", caps.HasMellanox,
		"switchdevSupported", caps.SwitchdevSupported)

	dpuPresent, dpuModel := a.DPUManager.Detect()
	if dpuPresent {
		a.log.InfoContext(ctx, "DPU present", "model", dpuModel)
	}

	// Phase 2: Configure OVS bridges from NodeNetworkConfig.
	if err := a.configureBridges(ctx); err != nil {
		a.log.ErrorContext(ctx, "failed to configure bridges", "error", err)
		// Continue; some bridges may have been configured.
	}

	// Phase 3: Establish tunnels from Tunnel CRDs.
	if err := a.establishTunnels(ctx); err != nil {
		a.log.ErrorContext(ctx, "failed to establish tunnels", "error", err)
	}

	// Phase 4: Configure bonds from Bond CRDs.
	if err := a.configureBonds(ctx); err != nil {
		a.log.ErrorContext(ctx, "failed to configure bonds", "error", err)
	}

	// Phase 5: Install service chains from ServiceChain CRDs.
	if err := a.installServiceChains(ctx); err != nil {
		a.log.ErrorContext(ctx, "failed to install service chains", "error", err)
	}

	// Phase 5b: Attach this node to its VPCs (bridges, tunnels, flows, IGW NAT).
	if err := a.VPCAttach.Reconcile(ctx); err != nil {
		a.log.ErrorContext(ctx, "failed to attach VPCs", "error", err)
	}

	// Phase 5c: Boot guests for VM-type VNF instances scheduled on this node.
	if err := a.VNFVM.Reconcile(ctx); err != nil {
		a.log.WarnContext(ctx, "VNF guest reconciliation incomplete", "error", err)
	}

	// Phase 6: Report status.
	a.log.InfoContext(ctx, "node agent started, beginning watch loop")

	// Start the CRD watch loop in the background.
	go a.watchLoop(ctx)

	return nil
}

// Stop gracefully shuts down the agent.
func (a *Agent) Stop() {
	a.log.Info("stopping node agent")
	if a.cancel != nil {
		a.cancel()
	}
}

// buildK8sClient creates a controller-runtime client for watching CRDs.
func (a *Agent) buildK8sClient() (client.Client, error) {
	var cfg *rest.Config
	var err error

	if a.cfg.KubeConfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", a.cfg.KubeConfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("building REST config: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}

	return c, nil
}

// configureBridges reads NodeNetworkConfig CRDs for this node and configures OVS bridges.
func (a *Agent) configureBridges(ctx context.Context) error {
	var nncList infrav1alpha1.NodeNetworkConfigList
	if err := a.k8sClient.List(ctx, &nncList); err != nil {
		return fmt.Errorf("listing NodeNetworkConfigs: %w", err)
	}

	for _, nnc := range nncList.Items {
		if nnc.Spec.NodeName != a.cfg.NodeName {
			continue
		}

		a.log.InfoContext(ctx, "applying NodeNetworkConfig",
			"name", nnc.Name, "bridges", len(nnc.Spec.OVSBridges))

		var bridges []agentovs.BridgeConfig
		for _, br := range nnc.Spec.OVSBridges {
			bridges = append(bridges, agentovs.BridgeConfig{
				Name:         br.Name,
				Type:         string(br.Type),
				DatapathType: string(br.DatapathType),
				Uplinks:      br.Uplinks,
			})
		}

		if err := a.OVSManager.EnsureBridges(ctx, bridges); err != nil {
			return fmt.Errorf("ensuring bridges for %s: %w", nnc.Name, err)
		}
	}

	return nil
}

// establishTunnels reads Tunnel CRDs for this node and creates tunnels.
func (a *Agent) establishTunnels(ctx context.Context) error {
	var tunnelList infrav1alpha1.TunnelList
	if err := a.k8sClient.List(ctx, &tunnelList); err != nil {
		return fmt.Errorf("listing Tunnels: %w", err)
	}

	for _, tun := range tunnelList.Items {
		if tun.Spec.LocalEndpoint.NodeName != a.cfg.NodeName {
			continue
		}

		var vni int32
		if tun.Spec.VNI != nil {
			vni = *tun.Spec.VNI
		}

		bridge := "br-tun" // default tunnel bridge
		_, err := a.TunnelManager.EnsureTunnel(ctx,
			string(tun.Spec.Type),
			tun.Spec.LocalEndpoint.IP,
			tun.Spec.RemoteEndpoint.IP,
			vni,
			bridge,
		)
		if err != nil {
			a.log.ErrorContext(ctx, "failed to establish tunnel",
				"name", tun.Name, "error", err)
			continue
		}

		a.log.InfoContext(ctx, "tunnel established",
			"name", tun.Name,
			"type", tun.Spec.Type,
			"remote", tun.Spec.RemoteEndpoint.NodeName)
	}

	return nil
}

// configureBonds reads Bond CRDs for this node and configures bonding.
func (a *Agent) configureBonds(ctx context.Context) error {
	var bondList infrav1alpha1.BondList
	if err := a.k8sClient.List(ctx, &bondList); err != nil {
		return fmt.Errorf("listing Bonds: %w", err)
	}

	for _, bond := range bondList.Items {
		if bond.Spec.NodeName != a.cfg.NodeName {
			continue
		}

		err := a.BondManager.EnsureBond(ctx,
			bond.Name,
			string(bond.Spec.Mode),
			bond.Spec.Interfaces,
			int(bond.Spec.MTU),
		)
		if err != nil {
			a.log.ErrorContext(ctx, "failed to configure bond",
				"name", bond.Name, "error", err)
			continue
		}
	}

	return nil
}

// installServiceChains reads ServiceChain CRDs and installs chains for this node.
func (a *Agent) installServiceChains(ctx context.Context) error {
	var chainList nfvv1alpha1.ServiceChainList
	if err := a.k8sClient.List(ctx, &chainList); err != nil {
		return fmt.Errorf("listing ServiceChains: %w", err)
	}

	for _, sc := range chainList.Items {
		var hops []agentnfv.ChainHop
		for _, elem := range sc.Spec.Chain {
			hops = append(hops, agentnfv.ChainHop{
				VNFName:     elem.Name,
				IngressPort: elem.PortPair.Ingress,
				EgressPort:  elem.PortPair.Egress,
			})
		}

		if err := a.NFVAgent.InstallChain(ctx, sc.Name, hops); err != nil {
			a.log.ErrorContext(ctx, "failed to install service chain",
				"name", sc.Name, "error", err)
			continue
		}
	}

	return nil
}

// watchLoop periodically reconciles CRD state. In a production agent, this would
// use controller-runtime informers with watches; here we use periodic polling.
func (a *Agent) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("watch loop stopped")
			return
		case <-ticker.C:
			a.log.Debug("reconciling CRD state")
			if err := a.configureBridges(ctx); err != nil {
				a.log.Error("bridge reconciliation failed", "error", err)
			}
			if err := a.establishTunnels(ctx); err != nil {
				a.log.Error("tunnel reconciliation failed", "error", err)
			}
			if err := a.configureBonds(ctx); err != nil {
				a.log.Error("bond reconciliation failed", "error", err)
			}
			if err := a.installServiceChains(ctx); err != nil {
				a.log.Error("service chain reconciliation failed", "error", err)
			}
			if err := a.VPCAttach.Reconcile(ctx); err != nil {
				a.log.Error("VPC attachment reconciliation failed", "error", err)
			}
			if err := a.VNFVM.Reconcile(ctx); err != nil {
				a.log.Warn("VNF guest reconciliation incomplete", "error", err)
			}
		}
	}
}
