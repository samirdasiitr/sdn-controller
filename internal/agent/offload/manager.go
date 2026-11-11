/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package offload

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/sdncontroller/sdncontroller/pkg/hw"
)

// HWCapabilities describes the hardware offload capabilities of the node.
type HWCapabilities struct {
	// HasMellanox indicates whether a Mellanox NIC is present.
	HasMellanox bool

	// MellanoxModel is the Mellanox NIC model (e.g., "ConnectX-6 Dx").
	MellanoxModel string

	// SwitchdevSupported indicates whether switchdev mode is available.
	SwitchdevSupported bool

	// MaxVFs is the maximum number of VFs supported by the NIC.
	MaxVFs int
}

// TCFlowerRule defines a TC flower rule for hardware offload.
type TCFlowerRule struct {
	// Device is the network device to program the rule on.
	Device string

	// Protocol is the protocol match (e.g., "ip", "ipv6").
	Protocol string

	// SrcIP is the source IP match.
	SrcIP string

	// DstIP is the destination IP match.
	DstIP string

	// Action is the TC action (e.g., "drop", "pass", "redirect dev <dev>").
	Action string

	// Priority is the rule priority.
	Priority int

	// SkipSW requests hardware-only offload.
	SkipSW bool
}

// OffloadManager manages hardware offload (switchdev, SR-IOV, TC flower) on the node.
type OffloadManager struct {
	logger *slog.Logger
}

// NewOffloadManager creates a new OffloadManager.
func NewOffloadManager(logger *slog.Logger) *OffloadManager {
	return &OffloadManager{
		logger: logger.With("component", "offload-manager"),
	}
}

// DetectCapabilities scans the system for hardware offload capabilities.
func (m *OffloadManager) DetectCapabilities() HWCapabilities {
	caps := HWCapabilities{}

	nics, err := hw.DiscoverNICs()
	if err != nil {
		m.logger.Warn("failed to discover NICs for offload detection", "error", err)
		return caps
	}

	for _, nic := range nics {
		if hw.IsMellanoxOffloadCapable(nic) {
			caps.HasMellanox = true
			caps.MellanoxModel = nic.Model
			caps.SwitchdevSupported = true
			if nic.NumVFs > caps.MaxVFs {
				caps.MaxVFs = nic.NumVFs
			}
		}
	}

	m.logger.Info("detected hardware capabilities",
		"hasMellanox", caps.HasMellanox,
		"model", caps.MellanoxModel,
		"switchdevSupported", caps.SwitchdevSupported,
		"maxVFs", caps.MaxVFs)

	return caps
}

// EnableSwitchdev switches a device to switchdev mode using devlink.
// If offload fails, it logs a warning and returns nil for graceful fallback.
func (m *OffloadManager) EnableSwitchdev(ctx context.Context, device string) error {
	pciAddr, err := m.getPCIAddress(ctx, device)
	if err != nil {
		m.logger.Warn("failed to get PCI address for switchdev, falling back to software path",
			"device", device, "error", err)
		return nil
	}

	m.logger.InfoContext(ctx, "enabling switchdev mode",
		"device", device, "pciAddr", pciAddr)

	cmd := exec.CommandContext(ctx, "devlink", "dev", "eswitch", "set",
		fmt.Sprintf("pci/%s", pciAddr), "mode", "switchdev")
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("failed to enable switchdev mode, falling back to software path",
			"device", device, "pciAddr", pciAddr,
			"error", err, "output", string(output))
		return nil
	}

	m.logger.InfoContext(ctx, "switchdev mode enabled",
		"device", device, "pciAddr", pciAddr)
	return nil
}

// CreateVFs creates the specified number of SR-IOV virtual functions on a device.
// If VF creation fails, it logs a warning and returns nil for graceful fallback.
func (m *OffloadManager) CreateVFs(ctx context.Context, device string, count int) error {
	m.logger.InfoContext(ctx, "creating VFs",
		"device", device, "count", count)

	sysPath := fmt.Sprintf("/sys/class/net/%s/device/sriov_numvfs", device)
	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("echo %d > %s", count, sysPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("failed to create VFs, falling back to software path",
			"device", device, "count", count,
			"error", err, "output", string(output))
		return nil
	}

	m.logger.InfoContext(ctx, "VFs created",
		"device", device, "count", count)
	return nil
}

// ProgramTCFlower programs a TC flower rule for hardware offload.
// If offload fails, it logs a warning and returns nil for graceful fallback.
func (m *OffloadManager) ProgramTCFlower(ctx context.Context, rule TCFlowerRule) error {
	m.logger.InfoContext(ctx, "programming TC flower rule",
		"device", rule.Device, "srcIP", rule.SrcIP,
		"dstIP", rule.DstIP, "action", rule.Action)

	args := []string{"filter", "add", "dev", rule.Device, "protocol", rule.Protocol,
		"parent", "ffff:", "prio", fmt.Sprintf("%d", rule.Priority), "flower"}

	if rule.SrcIP != "" {
		args = append(args, "src_ip", rule.SrcIP)
	}
	if rule.DstIP != "" {
		args = append(args, "dst_ip", rule.DstIP)
	}
	if rule.SkipSW {
		args = append(args, "skip_sw")
	}
	args = append(args, "action", rule.Action)

	cmd := exec.CommandContext(ctx, "tc", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("failed to program TC flower rule, falling back to software path",
			"device", rule.Device, "error", err, "output", string(output))
		return nil
	}

	return nil
}

// GetOffloadedFlowCount returns the number of TC flower rules offloaded to hardware.
// Returns 0 on any error (graceful fallback).
func (m *OffloadManager) GetOffloadedFlowCount(ctx context.Context, device string) int {
	cmd := exec.CommandContext(ctx, "tc", "-s", "filter", "show", "dev", device, "parent", "ffff:")
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("failed to query offloaded flows",
			"device", device, "error", err)
		return 0
	}

	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "in_hw") {
			count++
		}
	}
	return count
}

// getPCIAddress resolves the PCI address for a network device.
func (m *OffloadManager) getPCIAddress(ctx context.Context, device string) (string, error) {
	cmd := exec.CommandContext(ctx, "readlink", "-f",
		fmt.Sprintf("/sys/class/net/%s/device", device))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolving PCI address for %s: %w", device, err)
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("unexpected sysfs path for %s", device)
	}
	return parts[len(parts)-1], nil
}
