package telemetry

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	infrav1alpha1 "github.com/sdncontroller/sdncontroller/api/infra/v1alpha1"
	nfvv1alpha1 "github.com/sdncontroller/sdncontroller/api/nfv/v1alpha1"
	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
)

// MetricsCollector periodically scrapes CRD state and updates Prometheus gauges.
type MetricsCollector struct {
	client   client.Client
	interval time.Duration
}

// NewMetricsCollector creates a new collector.
func NewMetricsCollector(c client.Client, interval time.Duration) *MetricsCollector {
	return &MetricsCollector{client: c, interval: interval}
}

// Start implements manager.Runnable so it can be added via mgr.Add().
func (mc *MetricsCollector) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("metrics-collector")
	logger.Info("Starting metrics collector", "interval", mc.interval)

	ticker := time.NewTicker(mc.interval)
	defer ticker.Stop()

	// Collect immediately on start
	mc.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping metrics collector")
			return nil
		case <-ticker.C:
			mc.collect(ctx)
		}
	}
}

func (mc *MetricsCollector) collect(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("metrics-collector")

	// Reset label-based gauges to avoid stale series
	VPCInfo.Reset()
	SubnetInfo.Reset()
	FirewallRuleCount.Reset()
	LBBackendCount.Reset()
	TunnelInfo.Reset()
	ServiceChainHops.Reset()
	VNFReplicas.Reset()
	GatewayInfo.Reset()
	TopologyEdge.Reset()

	// Nodes
	var nodes corev1.NodeList
	if err := mc.client.List(ctx, &nodes); err == nil {
		NodeCount.Set(float64(len(nodes.Items)))
	}

	// VPCs
	var vpcs vpcv1alpha1.VPCList
	if err := mc.client.List(ctx, &vpcs); err == nil {
		VPCCount.Set(float64(len(vpcs.Items)))
		ready := 0
		for _, v := range vpcs.Items {
			phase := string(v.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			if v.Status.Phase == vpcv1alpha1.VPCPhaseReady {
				ready++
			}
			vni := float64(0)
			if v.Status.AllocatedVNI > 0 {
				vni = float64(v.Status.AllocatedVNI)
			}
			VPCInfo.WithLabelValues(v.Name, v.Namespace, v.Spec.CIDR, phase, strconv.Itoa(int(v.Spec.MTU))).Set(vni)
		}
		VPCReadyCount.Set(float64(ready))
	} else {
		logger.V(2).Error(err, "failed to list VPCs")
	}

	// Subnets
	var subnets vpcv1alpha1.SubnetList
	if err := mc.client.List(ctx, &subnets); err == nil {
		SubnetCount.Set(float64(len(subnets.Items)))
		ready := 0
		for _, s := range subnets.Items {
			phase := string(s.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			if s.Status.Phase == vpcv1alpha1.SubnetPhaseReady {
				ready++
			}
			avail := float64(s.Status.AvailableIPs)
			SubnetInfo.WithLabelValues(s.Name, s.Namespace, s.Spec.VPCRef, s.Spec.CIDR, s.Status.Gateway, phase, string(s.Spec.Type)).Set(avail)

			// Topology: subnet -> vpc
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("subnet/%s/%s", s.Namespace, s.Name), s.Name, "subnet",
				fmt.Sprintf("vpc/%s/%s", s.Namespace, s.Spec.VPCRef), s.Spec.VPCRef, "vpc",
				"belongs_to",
			).Set(1)
		}
		SubnetReadyCount.Set(float64(ready))
	}

	// InternetGateways
	var igws gatewayv1alpha1.InternetGatewayList
	if err := mc.client.List(ctx, &igws); err == nil {
		InternetGatewayCount.Set(float64(len(igws.Items)))
		for _, g := range igws.Items {
			phase := string(g.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			val := float64(0)
			if phase == "Ready" {
				val = 1
			}
			GatewayInfo.WithLabelValues(g.Name, g.Namespace, g.Spec.VPCRef, "InternetGateway", g.Status.ActiveNode, phase).Set(val)
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("igw/%s/%s", g.Namespace, g.Name), g.Name, "igw",
				fmt.Sprintf("vpc/%s/%s", g.Namespace, g.Spec.VPCRef), g.Spec.VPCRef, "vpc",
				"attached_to",
			).Set(1)
			if g.Status.ActiveNode != "" {
				TopologyEdge.WithLabelValues(
					fmt.Sprintf("igw/%s/%s", g.Namespace, g.Name), g.Name, "igw",
					fmt.Sprintf("node/%s", g.Status.ActiveNode), g.Status.ActiveNode, "node",
					"runs_on",
				).Set(1)
			}
		}
	}

	// NATGateways
	var natgws gatewayv1alpha1.NATGatewayList
	if err := mc.client.List(ctx, &natgws); err == nil {
		NATGatewayCount.Set(float64(len(natgws.Items)))
		for _, g := range natgws.Items {
			phase := string(g.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			val := float64(0)
			if phase == "Ready" {
				val = 1
			}
			GatewayInfo.WithLabelValues(g.Name, g.Namespace, g.Spec.VPCRef, "NATGateway", g.Status.ActiveNode, phase).Set(val)
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("natgw/%s/%s", g.Namespace, g.Name), g.Name, "natgw",
				fmt.Sprintf("vpc/%s/%s", g.Namespace, g.Spec.VPCRef), g.Spec.VPCRef, "vpc",
				"attached_to",
			).Set(1)
		}
	}

	// Firewalls
	var fws servicev1alpha1.FirewallList
	if err := mc.client.List(ctx, &fws); err == nil {
		FirewallCount.Set(float64(len(fws.Items)))
		for _, f := range fws.Items {
			FirewallRuleCount.WithLabelValues(f.Name, f.Namespace, f.Spec.VPCRef, string(f.Spec.DefaultAction)).Set(float64(len(f.Spec.Rules)))
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("fw/%s/%s", f.Namespace, f.Name), f.Name, "firewall",
				fmt.Sprintf("vpc/%s/%s", f.Namespace, f.Spec.VPCRef), f.Spec.VPCRef, "vpc",
				"protects",
			).Set(1)
		}
	}

	// LoadBalancers
	var lbs servicev1alpha1.LoadBalancerList
	if err := mc.client.List(ctx, &lbs); err == nil {
		LoadBalancerCount.Set(float64(len(lbs.Items)))
		for _, lb := range lbs.Items {
			LBBackendCount.WithLabelValues(lb.Name, lb.Namespace, lb.Spec.VPCRef, lb.Spec.Frontend.IP, string(lb.Spec.Algorithm)).Set(float64(len(lb.Spec.Backends)))
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("lb/%s/%s", lb.Namespace, lb.Name), lb.Name, "loadbalancer",
				fmt.Sprintf("vpc/%s/%s", lb.Namespace, lb.Spec.VPCRef), lb.Spec.VPCRef, "vpc",
				"serves",
			).Set(1)
		}
	}

	// Tunnels
	var tunnels infrav1alpha1.TunnelList
	if err := mc.client.List(ctx, &tunnels); err == nil {
		TunnelCount.Set(float64(len(tunnels.Items)))
		for _, t := range tunnels.Items {
			phase := string(t.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			val := float64(0)
			if phase == "Established" {
				val = 1
			}
			TunnelInfo.WithLabelValues(t.Name, t.Namespace, string(t.Spec.Type), t.Spec.LocalEndpoint.NodeName, t.Spec.RemoteEndpoint.NodeName, phase).Set(val)
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("node/%s", t.Spec.LocalEndpoint.NodeName), t.Spec.LocalEndpoint.NodeName, "node",
				fmt.Sprintf("node/%s", t.Spec.RemoteEndpoint.NodeName), t.Spec.RemoteEndpoint.NodeName, "node",
				fmt.Sprintf("tunnel_%s", string(t.Spec.Type)),
			).Set(1)
		}
	}

	// VNFs
	var vnfs nfvv1alpha1.VirtualNetworkFunctionList
	if err := mc.client.List(ctx, &vnfs); err == nil {
		VNFCount.Set(float64(len(vnfs.Items)))
		for _, v := range vnfs.Items {
			phase := string(v.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			VNFReplicas.WithLabelValues(v.Name, v.Namespace, string(v.Spec.Type), phase).Set(float64(v.Status.ReadyReplicas))
		}
	}

	// ServiceChains
	var chains nfvv1alpha1.ServiceChainList
	if err := mc.client.List(ctx, &chains); err == nil {
		ServiceChainCount.Set(float64(len(chains.Items)))
		for _, sc := range chains.Items {
			phase := string(sc.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			ServiceChainHops.WithLabelValues(sc.Name, sc.Namespace, sc.Spec.VPCRef, string(sc.Spec.Direction), phase).Set(float64(len(sc.Spec.Chain)))
			TopologyEdge.WithLabelValues(
				fmt.Sprintf("sc/%s/%s", sc.Namespace, sc.Name), sc.Name, "servicechain",
				fmt.Sprintf("vpc/%s/%s", sc.Namespace, sc.Spec.VPCRef), sc.Spec.VPCRef, "vpc",
				"chains_in",
			).Set(1)
			for _, hop := range sc.Spec.Chain {
				TopologyEdge.WithLabelValues(
					fmt.Sprintf("sc/%s/%s", sc.Namespace, sc.Name), sc.Name, "servicechain",
					fmt.Sprintf("vnf/%s/%s", sc.Namespace, hop.VNFRef), hop.VNFRef, "vnf",
					"uses_vnf",
				).Set(1)
			}
		}
	}
}
