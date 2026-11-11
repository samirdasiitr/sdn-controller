/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package ovs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

// BridgeConfig defines the desired state of an OVS bridge.
type BridgeConfig struct {
	// Name is the OVS bridge name (e.g., "br-int", "br-ex").
	Name string

	// Type is the bridge role (e.g., "integration", "external", "tunnel").
	Type string

	// DatapathType is the OVS datapath type (e.g., "system", "netdev").
	DatapathType string

	// Uplinks are physical interfaces to attach to the bridge.
	Uplinks []string
}

// OVSManager manages OVS bridges, ports, and flows on the node.
type OVSManager struct {
	client *pkgovs.Client
	logger *slog.Logger
}

// NewOVSManager creates a new OVSManager wrapping the given OVS client.
func NewOVSManager(client *pkgovs.Client, logger *slog.Logger) *OVSManager {
	return &OVSManager{
		client: client,
		logger: logger.With("component", "ovs-manager"),
	}
}

// EnsureBridges creates or verifies the specified OVS bridges and attaches uplinks.
func (m *OVSManager) EnsureBridges(ctx context.Context, bridges []BridgeConfig) error {
	for _, br := range bridges {
		m.logger.InfoContext(ctx, "ensuring OVS bridge",
			"bridge", br.Name, "type", br.Type, "datapath", br.DatapathType)

		if err := m.client.CreateBridge(ctx, br.Name, br.DatapathType); err != nil {
			return fmt.Errorf("creating bridge %s: %w", br.Name, err)
		}

		for _, uplink := range br.Uplinks {
			m.logger.InfoContext(ctx, "attaching uplink to bridge",
				"bridge", br.Name, "uplink", uplink)
			if err := m.client.AddPort(ctx, br.Name, uplink, nil); err != nil {
				return fmt.Errorf("adding uplink %s to bridge %s: %w", uplink, br.Name, err)
			}
		}
	}
	return nil
}

// AddPortToBridge adds a port to the specified OVS bridge with optional configuration.
func (m *OVSManager) AddPortToBridge(ctx context.Context, bridge, port string, options map[string]string) error {
	m.logger.InfoContext(ctx, "adding port to bridge",
		"bridge", bridge, "port", port)
	return m.client.AddPort(ctx, bridge, port, options)
}

// RemovePort removes a port from the specified OVS bridge.
func (m *OVSManager) RemovePort(ctx context.Context, bridge, port string) error {
	m.logger.InfoContext(ctx, "removing port from bridge",
		"bridge", bridge, "port", port)
	return m.client.DeletePort(ctx, bridge, port)
}

// ProgramFlows adds one or more OpenFlow rules to a bridge.
func (m *OVSManager) ProgramFlows(ctx context.Context, bridge string, flows []string) error {
	for _, flow := range flows {
		m.logger.DebugContext(ctx, "programming flow",
			"bridge", bridge, "flow", flow)
		if err := m.client.AddFlow(ctx, bridge, flow); err != nil {
			return fmt.Errorf("adding flow to %s: %w", bridge, err)
		}
	}
	return nil
}

// GetFlowCount returns the number of OpenFlow rules installed on a bridge.
func (m *OVSManager) GetFlowCount(ctx context.Context, bridge string) (int, error) {
	output, err := m.client.DumpFlows(ctx, bridge)
	if err != nil {
		return 0, fmt.Errorf("dumping flows on %s: %w", bridge, err)
	}
	if output == "" {
		return 0, nil
	}
	lines := strings.Split(output, "\n")
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip header lines and empty lines; flow lines start with " cookie=" or contain "actions=".
		if trimmed != "" && !strings.HasPrefix(trimmed, "NXST_FLOW") && !strings.HasPrefix(trimmed, "OFPST_FLOW") {
			count++
		}
	}
	return count, nil
}
