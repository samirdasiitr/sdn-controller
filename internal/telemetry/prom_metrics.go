package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Resource inventory gauges
	VPCCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_vpc_total",
		Help: "Total number of VPC resources",
	})
	VPCReadyCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_vpc_ready",
		Help: "Number of VPCs in Ready phase",
	})
	SubnetCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_subnet_total",
		Help: "Total number of Subnet resources",
	})
	SubnetReadyCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_subnet_ready",
		Help: "Number of Subnets in Ready phase",
	})
	InternetGatewayCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_internetgateway_total",
		Help: "Total number of InternetGateway resources",
	})
	NATGatewayCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_natgateway_total",
		Help: "Total number of NATGateway resources",
	})
	LoadBalancerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_loadbalancer_total",
		Help: "Total number of LoadBalancer resources",
	})
	FirewallCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_firewall_total",
		Help: "Total number of Firewall resources",
	})
	TunnelCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_tunnel_total",
		Help: "Total number of Tunnel resources",
	})
	VNFCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_vnf_total",
		Help: "Total number of VirtualNetworkFunction resources",
	})
	ServiceChainCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_servicechain_total",
		Help: "Total number of ServiceChain resources",
	})
	NodeCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sdn_node_total",
		Help: "Total number of Kubernetes nodes",
	})

	// Detail gauges with labels
	VPCInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_vpc_info",
		Help: "VPC information (value is VNI)",
	}, []string{"name", "namespace", "cidr", "phase", "mtu"})

	SubnetInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_subnet_info",
		Help: "Subnet information (value is available IPs)",
	}, []string{"name", "namespace", "vpc", "cidr", "gateway", "phase", "type"})

	FirewallRuleCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_firewall_rules",
		Help: "Number of rules per firewall",
	}, []string{"name", "namespace", "vpc", "default_action"})

	LBBackendCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_loadbalancer_backends",
		Help: "Number of backends per load balancer",
	}, []string{"name", "namespace", "vpc", "vip", "algorithm"})

	TunnelInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_tunnel_info",
		Help: "Tunnel information (value is 1 if established)",
	}, []string{"name", "namespace", "type", "local_node", "remote_node", "phase"})

	ServiceChainHops = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_servicechain_hops",
		Help: "Number of VNF hops per service chain",
	}, []string{"name", "namespace", "vpc", "direction", "phase"})

	VNFReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_vnf_replicas",
		Help: "VNF replica count (value is ready replicas)",
	}, []string{"name", "namespace", "type", "phase"})

	GatewayInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_gateway_info",
		Help: "Gateway information (value is 1 if ready)",
	}, []string{"name", "namespace", "vpc", "kind", "active_node", "phase"})

	// Topology edges for node graph
	TopologyEdge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_topology_edge",
		Help: "Network topology edges (value 1 = connected)",
	}, []string{"source_id", "source_name", "source_type", "target_id", "target_name", "target_type", "edge_type"})
)

func init() {
	metrics.Registry.MustRegister(
		VPCCount, VPCReadyCount,
		SubnetCount, SubnetReadyCount,
		InternetGatewayCount, NATGatewayCount,
		LoadBalancerCount, FirewallCount,
		TunnelCount, VNFCount, ServiceChainCount, NodeCount,
		VPCInfo, SubnetInfo,
		FirewallRuleCount, LBBackendCount,
		TunnelInfo, ServiceChainHops, VNFReplicas, GatewayInfo,
		TopologyEdge,
	)
}
