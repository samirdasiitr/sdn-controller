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

package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

// Supported tunnel types.
const (
	TypeVXLAN = "VXLAN"
	TypeIPIP  = "IPIP"
	TypeGRE   = "GRE"
)

// TunnelInfo describes an active tunnel on the node.
type TunnelInfo struct {
	// PortName is the OVS port name for this tunnel.
	PortName string

	// TunnelType is the encapsulation type (VXLAN, IPIP, GRE).
	TunnelType string

	// LocalIP is the local underlay IP address.
	LocalIP string

	// RemoteIP is the remote underlay IP address.
	RemoteIP string

	// VNI is the VXLAN Network Identifier (only for VXLAN tunnels).
	VNI int32

	// Bridge is the OVS bridge the tunnel port is attached to.
	Bridge string
}

// TunnelManager manages overlay tunnels on the node via OVS.
type TunnelManager struct {
	client  *pkgovs.Client
	logger  *slog.Logger
	mu      sync.RWMutex
	tunnels map[string]TunnelInfo // keyed by port name
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(client *pkgovs.Client, logger *slog.Logger) *TunnelManager {
	return &TunnelManager{
		client:  client,
		logger:  logger.With("component", "tunnel-manager"),
		tunnels: make(map[string]TunnelInfo),
	}
}

// EnsureTunnel creates or verifies a tunnel port on the specified bridge.
func (m *TunnelManager) EnsureTunnel(ctx context.Context, tunnelType, localIP, remoteIP string, vni int32, bridge string) (string, error) {
	portName := m.portName(tunnelType, remoteIP, vni)

	m.logger.InfoContext(ctx, "ensuring tunnel",
		"type", tunnelType, "localIP", localIP, "remoteIP", remoteIP,
		"vni", vni, "bridge", bridge, "port", portName)

	var err error
	switch strings.ToUpper(tunnelType) {
	case TypeVXLAN:
		err = m.client.AddVXLANPort(ctx, bridge, portName, remoteIP, vni)
	case TypeIPIP:
		err = m.client.AddIPIPPort(ctx, bridge, portName, localIP, remoteIP)
	case TypeGRE:
		err = m.client.AddGREPort(ctx, bridge, portName, remoteIP)
	default:
		return "", fmt.Errorf("unsupported tunnel type: %s", tunnelType)
	}
	if err != nil {
		return "", fmt.Errorf("creating %s tunnel to %s: %w", tunnelType, remoteIP, err)
	}

	info := TunnelInfo{
		PortName:   portName,
		TunnelType: tunnelType,
		LocalIP:    localIP,
		RemoteIP:   remoteIP,
		VNI:        vni,
		Bridge:     bridge,
	}
	m.mu.Lock()
	m.tunnels[portName] = info
	m.mu.Unlock()

	return portName, nil
}

// RemoveTunnel removes a tunnel port from its bridge.
func (m *TunnelManager) RemoveTunnel(ctx context.Context, portName, bridge string) error {
	m.logger.InfoContext(ctx, "removing tunnel",
		"port", portName, "bridge", bridge)

	if err := m.client.DeletePort(ctx, bridge, portName); err != nil {
		return fmt.Errorf("deleting tunnel port %s: %w", portName, err)
	}

	m.mu.Lock()
	delete(m.tunnels, portName)
	m.mu.Unlock()

	return nil
}

// ListTunnels returns all currently tracked tunnels.
func (m *TunnelManager) ListTunnels() []TunnelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TunnelInfo, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		result = append(result, t)
	}
	return result
}

// HealthCheck verifies that all tracked tunnel ports still exist on their bridges.
func (m *TunnelManager) HealthCheck(ctx context.Context) error {
	m.mu.RLock()
	tunnelsCopy := make(map[string]TunnelInfo, len(m.tunnels))
	for k, v := range m.tunnels {
		tunnelsCopy[k] = v
	}
	m.mu.RUnlock()

	var errs []string
	for portName, info := range tunnelsCopy {
		exists, err := m.client.BridgeExists(ctx, info.Bridge)
		if err != nil {
			errs = append(errs, fmt.Sprintf("checking bridge %s for tunnel %s: %v", info.Bridge, portName, err))
			continue
		}
		if !exists {
			errs = append(errs, fmt.Sprintf("bridge %s not found for tunnel %s", info.Bridge, portName))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("tunnel health check failures: %s", strings.Join(errs, "; "))
	}
	return nil
}

// portName generates a deterministic OVS port name for a tunnel.
func (m *TunnelManager) portName(tunnelType, remoteIP string, vni int32) string {
	// Sanitize the IP for use in a port name.
	sanitized := strings.ReplaceAll(remoteIP, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	prefix := strings.ToLower(tunnelType)
	name := fmt.Sprintf("%s-%s", prefix, sanitized)
	if vni > 0 {
		name = fmt.Sprintf("%s-%d", name, vni)
	}
	// OVS port names are limited to 15 characters for kernel datapath.
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}
