package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Bridge-level metrics
	OVSBridgeUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_up",
		Help: "Whether the OVS bridge is up (1) or down (0)",
	}, []string{"node", "bridge"})

	OVSBridgePortCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_ports",
		Help: "Number of ports on each OVS bridge",
	}, []string{"node", "bridge"})

	OVSBridgeFlowCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_flows",
		Help: "Number of OpenFlow rules on each OVS bridge",
	}, []string{"node", "bridge"})

	// Port-level metrics
	OVSPortRxBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_rx_bytes",
		Help: "Total received bytes on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortTxBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_tx_bytes",
		Help: "Total transmitted bytes on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortRxPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_rx_packets",
		Help: "Total received packets on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortTxPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_tx_packets",
		Help: "Total transmitted packets on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortRxDrops = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_rx_drops",
		Help: "Total receive drops on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortTxDrops = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_tx_drops",
		Help: "Total transmit drops on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortRxErrors = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_rx_errors",
		Help: "Total receive errors on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	OVSPortTxErrors = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_port_tx_errors",
		Help: "Total transmit errors on OVS port",
	}, []string{"node", "bridge", "port", "ofport"})

	// Per-table flow counts
	OVSTableFlowCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_table_flows",
		Help: "Number of flows per OpenFlow table",
	}, []string{"node", "bridge", "table"})

	// OVS daemon status
	OVSDaemonStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_daemon_up",
		Help: "Whether OVS daemon is running (1) or down (0)",
	}, []string{"node", "daemon"})

	// OpenFlow rule traffic per bridge
	OVSBridgeFlowPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_flow_packets",
		Help: "Sum of packet counters across all OpenFlow rules on the bridge",
	}, []string{"node", "bridge"})

	OVSBridgeFlowBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_flow_bytes",
		Help: "Sum of byte counters across all OpenFlow rules on the bridge",
	}, []string{"node", "bridge"})

	// Datapath (kernel/TC) flows and offload
	OVSDatapathFlows = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_datapath_flows",
		Help: "Datapath flows currently installed (OVS 'flows:' from dpctl/show)",
	}, []string{"node", "datapath"})

	OVSDatapathFlowsOffloaded = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_datapath_flows_offloaded",
		Help: "Datapath flows offloaded to hardware (offloaded:yes)",
	}, []string{"node"})

	OVSDatapathFlowsSoftware = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_datapath_flows_software",
		Help: "Datapath flows handled in software (offloaded:no)",
	}, []string{"node"})

	OVSDatapathFlowsByType = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_datapath_flows_by_type",
		Help: "Datapath flows by datapath type (dp:ovs, dp:tc, ...)",
	}, []string{"node", "dp"})

	OVSDatapathLookups = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_datapath_lookups",
		Help: "Datapath lookup counters (hit, missed, lost)",
	}, []string{"node", "result"})

	OVSHWOffloadEnabled = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_hw_offload_enabled",
		Help: "1 if Open_vSwitch other_config:hw-offload is true",
	}, []string{"node"})

	OVSNICHWTCOffload = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_nic_hw_tc_offload",
		Help: "1 if the physical NIC has hw-tc-offload enabled (ethtool -k)",
	}, []string{"node", "device"})

	// Summary
	OVSBridgeTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sdn_ovs_bridge_total",
		Help: "Total number of OVS bridges on this node",
	}, []string{"node"})
)

func init() {
	metrics.Registry.MustRegister(
		OVSBridgeUp, OVSBridgePortCount, OVSBridgeFlowCount,
		OVSPortRxBytes, OVSPortTxBytes,
		OVSPortRxPackets, OVSPortTxPackets,
		OVSPortRxDrops, OVSPortTxDrops,
		OVSPortRxErrors, OVSPortTxErrors,
		OVSTableFlowCount,
		OVSDaemonStatus,
		OVSBridgeTotal,
		OVSBridgeFlowPackets, OVSBridgeFlowBytes,
		OVSDatapathFlows, OVSDatapathFlowsOffloaded, OVSDatapathFlowsSoftware,
		OVSDatapathFlowsByType, OVSDatapathLookups,
		OVSHWOffloadEnabled, OVSNICHWTCOffload,
	)
}

// OVSCollector scrapes OVS state from the local host.
type OVSCollector struct {
	interval time.Duration
	nodeName string
	useSudo  bool
}

// NewOVSCollector creates a collector that polls OVS every interval.
func NewOVSCollector(interval time.Duration, nodeName string, useSudo bool) *OVSCollector {
	return &OVSCollector{interval: interval, nodeName: nodeName, useSudo: useSudo}
}

// Start implements manager.Runnable.
func (c *OVSCollector) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("ovs-collector")
	logger.Info("Starting OVS metrics collector", "interval", c.interval, "node", c.nodeName, "sudo", c.useSudo)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping OVS metrics collector")
			return nil
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *OVSCollector) collect(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("ovs-collector")

	// Reset to avoid stale series
	OVSBridgeUp.Reset()
	OVSBridgePortCount.Reset()
	OVSBridgeFlowCount.Reset()
	OVSPortRxBytes.Reset()
	OVSPortTxBytes.Reset()
	OVSPortRxPackets.Reset()
	OVSPortTxPackets.Reset()
	OVSPortRxDrops.Reset()
	OVSPortTxDrops.Reset()
	OVSPortRxErrors.Reset()
	OVSPortTxErrors.Reset()
	OVSTableFlowCount.Reset()
	OVSDaemonStatus.Reset()
	OVSBridgeTotal.Reset()
	OVSBridgeFlowPackets.Reset()
	OVSBridgeFlowBytes.Reset()
	OVSDatapathFlows.Reset()
	OVSDatapathFlowsOffloaded.Reset()
	OVSDatapathFlowsSoftware.Reset()
	OVSDatapathFlowsByType.Reset()
	OVSDatapathLookups.Reset()
	OVSHWOffloadEnabled.Reset()
	OVSNICHWTCOffload.Reset()

	// Check daemon status
	c.collectDaemonStatus()
	c.collectDatapath()

	// List bridges
	bridges, err := c.runOVS("ovs-vsctl", "list-br")
	if err != nil {
		logger.V(2).Error(err, "failed to list OVS bridges")
		OVSBridgeTotal.WithLabelValues(c.nodeName).Set(0)
		return
	}

	brList := parseLines(bridges)
	OVSBridgeTotal.WithLabelValues(c.nodeName).Set(float64(len(brList)))

	for _, br := range brList {
		if br == "" {
			continue
		}
		c.collectBridge(ctx, br)
	}
}

func (c *OVSCollector) collectDaemonStatus() {
	// Check ovs-vswitchd
	if out, err := c.runCmd("ovs-appctl", "version"); err == nil && strings.Contains(out, "ovs-vswitchd") {
		OVSDaemonStatus.WithLabelValues(c.nodeName, "ovs-vswitchd").Set(1)
	} else {
		OVSDaemonStatus.WithLabelValues(c.nodeName, "ovs-vswitchd").Set(0)
	}

	// Check ovsdb-server
	if out, err := c.runCmd("ovs-appctl", "-t", "ovsdb-server", "version"); err == nil && strings.Contains(out, "ovsdb-server") {
		OVSDaemonStatus.WithLabelValues(c.nodeName, "ovsdb-server").Set(1)
	} else {
		OVSDaemonStatus.WithLabelValues(c.nodeName, "ovsdb-server").Set(0)
	}
}

func (c *OVSCollector) collectBridge(ctx context.Context, bridge string) {
	logger := log.FromContext(ctx).WithName("ovs-collector")
	node := c.nodeName

	// Bridge up/down: if we can query it, it's up
	OVSBridgeUp.WithLabelValues(node, bridge).Set(1)

	// Port list
	portsOut, err := c.runOVS("ovs-vsctl", "list-ports", bridge)
	if err != nil {
		logger.V(2).Error(err, "failed to list ports", "bridge", bridge)
		return
	}
	ports := parseLines(portsOut)
	OVSBridgePortCount.WithLabelValues(node, bridge).Set(float64(len(ports)))

	// Flow count
	flowsOut, err := c.runOVS("ovs-ofctl", "dump-flows", bridge)
	if err == nil {
		flowLines := parseLines(flowsOut)
		flowCount := 0
		var pkts, bytes float64
		tableCounts := map[string]int{}
		for _, line := range flowLines {
			if strings.Contains(line, "cookie=") || strings.Contains(line, "actions=") {
				flowCount++
				if m := flowCountersRe.FindStringSubmatch(line); m != nil {
					p, _ := strconv.ParseFloat(m[1], 64)
					b, _ := strconv.ParseFloat(m[2], 64)
					pkts += p
					bytes += b
				}
				if tbl := extractTableID(line); tbl != "" {
					tableCounts[tbl]++
				}
			}
		}
		OVSBridgeFlowCount.WithLabelValues(node, bridge).Set(float64(flowCount))
		OVSBridgeFlowPackets.WithLabelValues(node, bridge).Set(pkts)
		OVSBridgeFlowBytes.WithLabelValues(node, bridge).Set(bytes)
		for tbl, cnt := range tableCounts {
			OVSTableFlowCount.WithLabelValues(node, bridge, tbl).Set(float64(cnt))
		}
	}

	// Port statistics via dump-ports
	statsOut, err := c.runOVS("ovs-ofctl", "dump-ports", bridge)
	if err != nil {
		return
	}
	c.parsePortStats(node, bridge, ports, statsOut)
}

var (
	flowCountersRe = regexp.MustCompile(`n_packets=(\d+), n_bytes=(\d+)`)
	dpOffloadedRe  = regexp.MustCompile(`offloaded:(yes|no)`)
	dpTypeRe       = regexp.MustCompile(`dp:(\w+)`)
	dpLookupsRe    = regexp.MustCompile(`lookups: hit:(\d+) missed:(\d+) lost:(\d+)`)
	dpFlowsRe      = regexp.MustCompile(`(?m)^\s*flows:\s*(\d+)`)
	dpNameRe       = regexp.MustCompile(`(?m)^(\S+@\S+):`)
)

// collectDatapath exports datapath flow counts, offload state and lookup stats.
func (c *OVSCollector) collectDatapath() {
	node := c.nodeName

	enabled := 0.0
	if out, err := c.runCmd("ovs-vsctl", "--if-exists", "get", "Open_vSwitch", ".", "other_config:hw-offload"); err == nil &&
		strings.Contains(out, "true") {
		enabled = 1
	}
	OVSHWOffloadEnabled.WithLabelValues(node).Set(enabled)

	if out, err := c.runCmd("ovs-appctl", "dpctl/show"); err == nil {
		dp := "system"
		if m := dpNameRe.FindStringSubmatch(out); m != nil {
			dp = m[1]
		}
		if m := dpFlowsRe.FindStringSubmatch(out); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			OVSDatapathFlows.WithLabelValues(node, dp).Set(v)
		}
		if m := dpLookupsRe.FindStringSubmatch(out); m != nil {
			for i, r := range []string{"hit", "missed", "lost"} {
				v, _ := strconv.ParseFloat(m[i+1], 64)
				OVSDatapathLookups.WithLabelValues(node, r).Set(v)
			}
		}
	}

	var off, soft float64
	byType := map[string]float64{"ovs": 0, "tc": 0}
	if out, err := c.runCmd("ovs-appctl", "dpctl/dump-flows", "-m"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			t := dpTypeRe.FindStringSubmatch(line)
			if t == nil {
				continue
			}
			byType[t[1]]++
			// Only TC/hardware flows print offloaded:yes|no; software (dp:ovs) flows omit it.
			if m := dpOffloadedRe.FindStringSubmatch(line); m != nil && m[1] == "yes" {
				off++
			} else {
				soft++
			}
		}
	}
	OVSDatapathFlowsOffloaded.WithLabelValues(node).Set(off)
	OVSDatapathFlowsSoftware.WithLabelValues(node).Set(soft)
	for t, v := range byType {
		OVSDatapathFlowsByType.WithLabelValues(node, t).Set(v)
	}

	// Physical NICs: is TC flower hardware offload enabled?
	if ents, err := os.ReadDir("/sys/class/net"); err == nil {
		for _, e := range ents {
			name := e.Name()
			if _, err := os.Stat("/sys/class/net/" + name + "/device"); err != nil {
				continue
			}
			out, err := c.runCmd("ethtool", "-k", name)
			if err != nil {
				continue
			}
			v := 0.0
			for _, l := range strings.Split(out, "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), "hw-tc-offload:") && strings.Contains(l, ": on") {
					v = 1
				}
			}
			OVSNICHWTCOffload.WithLabelValues(node, name).Set(v)
		}
	}
}

// portStatRe matches "port  N:" or "port LOCAL:"
var portStatRe = regexp.MustCompile(`port\s+(\S+):`)
var rxStatsRe = regexp.MustCompile(`rx pkts=(\d+), bytes=(\d+), drop=(\d+), errs=(\d+)`)
var txStatsRe = regexp.MustCompile(`tx pkts=(\d+), bytes=(\d+), drop=(\d+), errs=(\d+)`)

func (c *OVSCollector) parsePortStats(node, bridge string, ports []string, output string) {
	// Build ofport->name mapping
	portNames := map[string]string{"LOCAL": bridge}
	for _, p := range ports {
		if p == "" {
			continue
		}
		ofport, err := c.runOVS("ovs-vsctl", "get", "Interface", p, "ofport")
		if err == nil {
			ofport = strings.TrimSpace(ofport)
			portNames[ofport] = p
		}
	}

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		m := portStatRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ofport := strings.TrimSpace(m[1])
		portName := portNames[ofport]
		if portName == "" {
			portName = "port-" + ofport
		}

		// rx line is on same line or next
		rxLine := line
		if i+1 < len(lines) {
			rxLine += " " + lines[i+1]
		}
		// tx line is 1-2 lines after
		txLine := ""
		if i+1 < len(lines) {
			txLine = lines[i+1]
		}
		if i+2 < len(lines) {
			txLine += " " + lines[i+2]
		}

		if rm := rxStatsRe.FindStringSubmatch(rxLine); rm != nil {
			setGaugeVal(OVSPortRxPackets, node, bridge, portName, ofport, rm[1])
			setGaugeVal(OVSPortRxBytes, node, bridge, portName, ofport, rm[2])
			setGaugeVal(OVSPortRxDrops, node, bridge, portName, ofport, rm[3])
			setGaugeVal(OVSPortRxErrors, node, bridge, portName, ofport, rm[4])
		}
		if tm := txStatsRe.FindStringSubmatch(txLine); tm != nil {
			setGaugeVal(OVSPortTxPackets, node, bridge, portName, ofport, tm[1])
			setGaugeVal(OVSPortTxBytes, node, bridge, portName, ofport, tm[2])
			setGaugeVal(OVSPortTxDrops, node, bridge, portName, ofport, tm[3])
			setGaugeVal(OVSPortTxErrors, node, bridge, portName, ofport, tm[4])
		}
	}
}

func setGaugeVal(g *prometheus.GaugeVec, node, bridge, port, ofport, val string) {
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return
	}
	g.WithLabelValues(node, bridge, port, ofport).Set(v)
}

func extractTableID(flowLine string) string {
	// Flows look like: " cookie=0x0, duration=..., table=10, ..."
	for _, part := range strings.Split(flowLine, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "table=") {
			return strings.TrimPrefix(part, "table=")
		}
	}
	return "0"
}

func parseLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (c *OVSCollector) runOVS(bin string, args ...string) (string, error) {
	return c.runCmd(bin, args...)
}

func (c *OVSCollector) runCmd(bin string, args ...string) (string, error) {
	var cmd *exec.Cmd
	if c.useSudo {
		allArgs := append([]string{bin}, args...)
		cmd = exec.Command("sudo", allArgs...)
	} else {
		cmd = exec.Command(bin, args...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w: %s", bin, args, err, string(out))
	}
	return string(out), nil
}
