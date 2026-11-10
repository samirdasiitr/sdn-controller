package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "sdncontroller"

// Metrics holds all SDN controller metric instruments.
type Metrics struct {
	// Controller metrics
	ReconcileDuration  metric.Float64Histogram
	ReconcileErrors    metric.Int64Counter
	ReconcileTotal     metric.Int64Counter
	ResourceCount      metric.Int64UpDownCounter
	VPCActiveCount     metric.Int64UpDownCounter
	SubnetActiveCount  metric.Int64UpDownCounter

	// Node agent metrics
	OVSFlowCount       metric.Int64Gauge
	TunnelTxBytes      metric.Int64Counter
	TunnelRxBytes      metric.Int64Counter
	TunnelTxPackets    metric.Int64Counter
	TunnelRxPackets    metric.Int64Counter
	OffloadFlowCount   metric.Int64Gauge
	BondActiveIfaces   metric.Int64Gauge
	NATTranslations    metric.Int64Gauge
	LBConnectionsActive metric.Int64Gauge
	LBConnectionsTotal metric.Int64Counter
	FWMatchedPackets   metric.Int64Counter
	FWDroppedPackets   metric.Int64Counter

	// Service chain metrics
	ChainProcessedPkts metric.Int64Counter
	ChainLatency       metric.Float64Histogram

	// IPAM metrics
	IPsAllocated  metric.Int64Gauge
	IPsAvailable  metric.Int64Gauge

	// Health metrics
	AgentUptime   metric.Float64Counter
	OVSConnStatus metric.Int64Gauge
}

// NewMetrics creates and registers all metric instruments.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(meterName)
	m := &Metrics{}
	var err error

	// Controller metrics
	if m.ReconcileDuration, err = meter.Float64Histogram("sdn.reconcile.duration",
		metric.WithDescription("Duration of reconciliation loops in seconds"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.ReconcileErrors, err = meter.Int64Counter("sdn.reconcile.errors",
		metric.WithDescription("Total reconciliation errors")); err != nil {
		return nil, err
	}
	if m.ReconcileTotal, err = meter.Int64Counter("sdn.reconcile.total",
		metric.WithDescription("Total reconciliation attempts")); err != nil {
		return nil, err
	}
	if m.ResourceCount, err = meter.Int64UpDownCounter("sdn.resource.count",
		metric.WithDescription("Number of active resources by type")); err != nil {
		return nil, err
	}
	if m.VPCActiveCount, err = meter.Int64UpDownCounter("sdn.vpc.active",
		metric.WithDescription("Number of active VPCs")); err != nil {
		return nil, err
	}
	if m.SubnetActiveCount, err = meter.Int64UpDownCounter("sdn.subnet.active",
		metric.WithDescription("Number of active subnets")); err != nil {
		return nil, err
	}

	// Node agent metrics
	if m.OVSFlowCount, err = meter.Int64Gauge("sdn.ovs.flow.count",
		metric.WithDescription("Number of OVS flows per bridge")); err != nil {
		return nil, err
	}
	if m.TunnelTxBytes, err = meter.Int64Counter("sdn.tunnel.tx.bytes",
		metric.WithDescription("Tunnel transmitted bytes"),
		metric.WithUnit("By")); err != nil {
		return nil, err
	}
	if m.TunnelRxBytes, err = meter.Int64Counter("sdn.tunnel.rx.bytes",
		metric.WithDescription("Tunnel received bytes"),
		metric.WithUnit("By")); err != nil {
		return nil, err
	}
	if m.TunnelTxPackets, err = meter.Int64Counter("sdn.tunnel.tx.packets",
		metric.WithDescription("Tunnel transmitted packets")); err != nil {
		return nil, err
	}
	if m.TunnelRxPackets, err = meter.Int64Counter("sdn.tunnel.rx.packets",
		metric.WithDescription("Tunnel received packets")); err != nil {
		return nil, err
	}
	if m.OffloadFlowCount, err = meter.Int64Gauge("sdn.offload.flow.count",
		metric.WithDescription("Number of hardware-offloaded flows")); err != nil {
		return nil, err
	}
	if m.BondActiveIfaces, err = meter.Int64Gauge("sdn.bond.active.interfaces",
		metric.WithDescription("Number of active bond member interfaces")); err != nil {
		return nil, err
	}
	if m.NATTranslations, err = meter.Int64Gauge("sdn.nat.translations.active",
		metric.WithDescription("Number of active NAT translations")); err != nil {
		return nil, err
	}
	if m.LBConnectionsActive, err = meter.Int64Gauge("sdn.lb.connections.active",
		metric.WithDescription("Number of active load balancer connections")); err != nil {
		return nil, err
	}
	if m.LBConnectionsTotal, err = meter.Int64Counter("sdn.lb.connections.total",
		metric.WithDescription("Total load balancer connections")); err != nil {
		return nil, err
	}
	if m.FWMatchedPackets, err = meter.Int64Counter("sdn.firewall.matched.packets",
		metric.WithDescription("Total firewall matched packets")); err != nil {
		return nil, err
	}
	if m.FWDroppedPackets, err = meter.Int64Counter("sdn.firewall.dropped.packets",
		metric.WithDescription("Total firewall dropped packets")); err != nil {
		return nil, err
	}

	// Service chain metrics
	if m.ChainProcessedPkts, err = meter.Int64Counter("sdn.chain.processed.packets",
		metric.WithDescription("Total packets processed by service chains")); err != nil {
		return nil, err
	}
	if m.ChainLatency, err = meter.Float64Histogram("sdn.chain.latency",
		metric.WithDescription("Service chain processing latency"),
		metric.WithUnit("ms")); err != nil {
		return nil, err
	}

	// IPAM metrics
	if m.IPsAllocated, err = meter.Int64Gauge("sdn.ipam.allocated",
		metric.WithDescription("Number of allocated IPs per subnet")); err != nil {
		return nil, err
	}
	if m.IPsAvailable, err = meter.Int64Gauge("sdn.ipam.available",
		metric.WithDescription("Number of available IPs per subnet")); err != nil {
		return nil, err
	}

	// Health metrics
	if m.AgentUptime, err = meter.Float64Counter("sdn.agent.uptime",
		metric.WithDescription("Agent uptime in seconds"),
		metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if m.OVSConnStatus, err = meter.Int64Gauge("sdn.ovs.connection.status",
		metric.WithDescription("OVS connection status (1=connected, 0=disconnected)")); err != nil {
		return nil, err
	}

	return m, nil
}
