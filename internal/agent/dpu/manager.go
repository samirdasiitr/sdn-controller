/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package dpu

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/sdncontroller/sdncontroller/pkg/hw"
)

// FlowRule represents a flow rule to be offloaded to the DPU.
type FlowRule struct {
	// SrcIP is the source IP match.
	SrcIP string

	// DstIP is the destination IP match.
	DstIP string

	// Protocol is the L4 protocol (e.g., "tcp", "udp").
	Protocol string

	// Action is the flow action (e.g., "forward", "drop").
	Action string
}

// DPUStatus describes the current status of a DPU on the node.
type DPUStatus struct {
	// Model is the DPU model name (e.g., "BlueField-2", "BlueField-3").
	Model string

	// Connected indicates whether the DPU is reachable.
	Connected bool

	// OffloadedFlows is the number of flows currently offloaded to the DPU.
	OffloadedFlows int64
}

// DPUManager manages DPU detection and flow offloading.
type DPUManager struct {
	logger         *slog.Logger
	present        bool
	model          string
	offloadedFlows atomic.Int64
}

// NewDPUManager creates a new DPUManager.
func NewDPUManager(logger *slog.Logger) *DPUManager {
	return &DPUManager{
		logger: logger.With("component", "dpu-manager"),
	}
}

// Detect checks if a DPU is present on the node using pkg/hw.DetectDPU.
func (m *DPUManager) Detect() (present bool, model string) {
	present, model = hw.DetectDPU()
	m.present = present
	m.model = model

	if present {
		m.logger.Info("DPU detected", "model", model)
	} else {
		m.logger.Info("no DPU detected")
	}

	return present, model
}

// DelegateOffload sends flow rules to the DPU for hardware offloading.
// In production, this would communicate with the DPU via gRPC or a vendor SDK.
func (m *DPUManager) DelegateOffload(ctx context.Context, flows []FlowRule) error {
	if !m.present {
		m.logger.Warn("attempted to offload flows but no DPU is present")
		return nil
	}

	m.logger.InfoContext(ctx, "delegating flow offload to DPU",
		"model", m.model, "flows", len(flows))

	// In a real implementation, this would communicate with the DPU.
	// For now, we track the offloaded flow count.
	m.offloadedFlows.Add(int64(len(flows)))

	m.logger.InfoContext(ctx, "flows offloaded to DPU",
		"count", len(flows), "total", m.offloadedFlows.Load())
	return nil
}

// GetDPUStatus returns the current DPU status.
func (m *DPUManager) GetDPUStatus() *DPUStatus {
	return &DPUStatus{
		Model:          m.model,
		Connected:      m.present,
		OffloadedFlows: m.offloadedFlows.Load(),
	}
}
