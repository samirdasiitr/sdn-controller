/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package libvirt

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"text/template"
	"bytes"

	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

// vmNetworkXMLTemplate is the libvirt XML template for an OVS-backed network interface.
const vmNetworkXMLTemplate = `<interface type='bridge'>
  <mac address='{{.MACAddr}}'/>
  <source bridge='{{.Bridge}}'/>
  <virtualport type='openvswitch'/>
  <model type='virtio'/>
</interface>`

// VMNetwork represents a VM network interface configuration.
type VMNetwork struct {
	// VMName is the name of the virtual machine.
	VMName string

	// Bridge is the OVS bridge this VM interface is attached to.
	Bridge string

	// MACAddr is the MAC address of the VM interface.
	MACAddr string

	// PortName is the OVS port name for this VM interface.
	PortName string

	// XMLDefinition is the generated libvirt XML for this interface.
	XMLDefinition string
}

// LibvirtManager manages OVS port creation and libvirt XML generation for VMs.
type LibvirtManager struct {
	client   *pkgovs.Client
	logger   *slog.Logger
	xmlTmpl  *template.Template

	mu       sync.RWMutex
	networks map[string]VMNetwork // keyed by "vmName/bridge"
}

// NewLibvirtManager creates a new LibvirtManager.
func NewLibvirtManager(client *pkgovs.Client, logger *slog.Logger) *LibvirtManager {
	tmpl := template.Must(template.New("vmnet").Parse(vmNetworkXMLTemplate))
	return &LibvirtManager{
		client:   client,
		logger:   logger.With("component", "libvirt-manager"),
		xmlTmpl:  tmpl,
		networks: make(map[string]VMNetwork),
	}
}

// CreateVMNetwork creates an OVS port for a VM and generates the libvirt XML definition.
func (m *LibvirtManager) CreateVMNetwork(ctx context.Context, vmName, bridge, macAddr string) (*VMNetwork, error) {
	portName := m.portName(vmName)

	m.logger.InfoContext(ctx, "creating VM network",
		"vm", vmName, "bridge", bridge, "mac", macAddr, "port", portName)

	// Create the OVS port for this VM.
	options := map[string]string{
		"external_ids": fmt.Sprintf("vm-name=%s", vmName),
	}
	if err := m.client.AddPort(ctx, bridge, portName, options); err != nil {
		return nil, fmt.Errorf("creating OVS port %s for VM %s: %w", portName, vmName, err)
	}

	// Generate libvirt XML definition.
	xmlDef, err := m.generateXML(bridge, macAddr)
	if err != nil {
		return nil, fmt.Errorf("generating libvirt XML for VM %s: %w", vmName, err)
	}

	net := VMNetwork{
		VMName:        vmName,
		Bridge:        bridge,
		MACAddr:       macAddr,
		PortName:      portName,
		XMLDefinition: xmlDef,
	}

	key := m.networkKey(vmName, bridge)
	m.mu.Lock()
	m.networks[key] = net
	m.mu.Unlock()

	m.logger.InfoContext(ctx, "VM network created",
		"vm", vmName, "bridge", bridge, "port", portName)
	return &net, nil
}

// DeleteVMNetwork removes the OVS port for a VM.
func (m *LibvirtManager) DeleteVMNetwork(ctx context.Context, vmName, bridge string) error {
	key := m.networkKey(vmName, bridge)

	m.mu.RLock()
	net, exists := m.networks[key]
	m.mu.RUnlock()

	if !exists {
		m.logger.Warn("VM network not found for deletion",
			"vm", vmName, "bridge", bridge)
		return nil
	}

	m.logger.InfoContext(ctx, "deleting VM network",
		"vm", vmName, "bridge", bridge, "port", net.PortName)

	if err := m.client.DeletePort(ctx, bridge, net.PortName); err != nil {
		return fmt.Errorf("deleting OVS port %s for VM %s: %w", net.PortName, vmName, err)
	}

	m.mu.Lock()
	delete(m.networks, key)
	m.mu.Unlock()

	return nil
}

// ListVMNetworks returns all currently tracked VM network interfaces.
func (m *LibvirtManager) ListVMNetworks() []VMNetwork {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]VMNetwork, 0, len(m.networks))
	for _, net := range m.networks {
		result = append(result, net)
	}
	return result
}

// generateXML renders the libvirt XML template for a VM network interface.
func (m *LibvirtManager) generateXML(bridge, macAddr string) (string, error) {
	data := struct {
		Bridge  string
		MACAddr string
	}{
		Bridge:  bridge,
		MACAddr: macAddr,
	}

	var buf bytes.Buffer
	if err := m.xmlTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing XML template: %w", err)
	}
	return buf.String(), nil
}

// portName generates a deterministic OVS port name for a VM.
func (m *LibvirtManager) portName(vmName string) string {
	name := fmt.Sprintf("vm-%s", vmName)
	// OVS port names are limited to 15 characters for kernel datapath.
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

// networkKey returns a unique key for a VM network.
func (m *LibvirtManager) networkKey(vmName, bridge string) string {
	return fmt.Sprintf("%s/%s", vmName, bridge)
}
