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

package nfv

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

// ChainHop represents a single hop in a service function chain.
type ChainHop struct {
	// VNFName is the name of the virtual network function at this hop.
	VNFName string

	// IngressPort is the OVS port name where traffic enters the VNF.
	IngressPort string

	// EgressPort is the OVS port name where traffic exits the VNF.
	EgressPort string
}

// ChainStats holds statistics for a service function chain.
type ChainStats struct {
	// ProcessedPackets is the total number of packets processed by the chain.
	ProcessedPackets int64
}

// chainEntry tracks an installed service chain.
type chainEntry struct {
	hops   []ChainHop
	bridge string
}

// NFVAgent manages service function chain (SFC) programming via OVS flows.
type NFVAgent struct {
	client *pkgovs.Client
	logger *slog.Logger
	bridge string // default bridge for SFC flows

	mu     sync.RWMutex
	chains map[string]chainEntry // keyed by chainID
}

// NewNFVAgent creates a new NFVAgent.
func NewNFVAgent(client *pkgovs.Client, bridge string, logger *slog.Logger) *NFVAgent {
	return &NFVAgent{
		client: client,
		logger: logger.With("component", "nfv-agent"),
		bridge: bridge,
		chains: make(map[string]chainEntry),
	}
}

// InstallChain programs OVS flows for a service function chain.
// Each hop steers traffic: previous egress -> current ingress, current egress -> next ingress.
func (a *NFVAgent) InstallChain(ctx context.Context, chainID string, hops []ChainHop) error {
	a.logger.InfoContext(ctx, "installing service chain",
		"chainID", chainID, "hops", len(hops))

	if len(hops) == 0 {
		return fmt.Errorf("chain %s has no hops", chainID)
	}

	// Program forwarding flows between consecutive hops.
	for i := 0; i < len(hops); i++ {
		hop := hops[i]

		// Flow: forward traffic from hop's egress to next hop's ingress.
		if i < len(hops)-1 {
			nextHop := hops[i+1]
			flow := fmt.Sprintf("cookie=0x%s,priority=200,in_port=%s,actions=output:%s",
				chainFlowCookie(chainID, i), hop.EgressPort, nextHop.IngressPort)
			if err := a.client.AddFlow(ctx, a.bridge, flow); err != nil {
				return fmt.Errorf("adding chain flow for hop %d (%s -> %s): %w",
					i, hop.VNFName, nextHop.VNFName, err)
			}
			a.logger.DebugContext(ctx, "installed chain hop flow",
				"chainID", chainID, "from", hop.VNFName, "to", nextHop.VNFName)
		}
	}

	a.mu.Lock()
	a.chains[chainID] = chainEntry{
		hops:   hops,
		bridge: a.bridge,
	}
	a.mu.Unlock()

	a.logger.InfoContext(ctx, "service chain installed",
		"chainID", chainID, "hops", len(hops))
	return nil
}

// RemoveChain removes all OVS flows associated with a service function chain.
func (a *NFVAgent) RemoveChain(ctx context.Context, chainID string) error {
	a.logger.InfoContext(ctx, "removing service chain", "chainID", chainID)

	a.mu.RLock()
	entry, exists := a.chains[chainID]
	a.mu.RUnlock()

	if !exists {
		a.logger.Warn("chain not found for removal", "chainID", chainID)
		return nil
	}

	// Delete all flows with cookies belonging to this chain.
	for i := 0; i < len(entry.hops)-1; i++ {
		match := fmt.Sprintf("cookie=0x%s/-1", chainFlowCookie(chainID, i))
		if err := a.client.DeleteFlows(ctx, entry.bridge, match); err != nil {
			a.logger.Warn("failed to delete chain flow",
				"chainID", chainID, "hop", i, "error", err)
		}
	}

	a.mu.Lock()
	delete(a.chains, chainID)
	a.mu.Unlock()

	a.logger.InfoContext(ctx, "service chain removed", "chainID", chainID)
	return nil
}

// GetChainStats returns statistics for a service function chain.
func (a *NFVAgent) GetChainStats(ctx context.Context, chainID string) (*ChainStats, error) {
	a.mu.RLock()
	entry, exists := a.chains[chainID]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("chain %s not found", chainID)
	}

	stats := &ChainStats{}

	// Sum packet counts from all chain flows.
	flowDump, err := a.client.DumpFlows(ctx, entry.bridge)
	if err != nil {
		return nil, fmt.Errorf("dumping flows for chain stats: %w", err)
	}

	for i := 0; i < len(entry.hops)-1; i++ {
		cookie := chainFlowCookie(chainID, i)
		// Parse flow dump output for matching cookies (simplified).
		_ = cookie
		_ = flowDump
		// In production, parse flow stats from the dump output.
	}

	return stats, nil
}

// chainFlowCookie generates a deterministic cookie for a chain hop flow.
func chainFlowCookie(chainID string, hopIndex int) string {
	// Use a simple hash of chainID + hopIndex for the cookie.
	h := uint64(0)
	for _, c := range chainID {
		h = h*31 + uint64(c)
	}
	h = h*31 + uint64(hopIndex)
	return fmt.Sprintf("%016x", h)
}
