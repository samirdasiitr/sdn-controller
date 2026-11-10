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

package bond

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// BondStatus describes the current state of a Linux bond interface.
type BondStatus struct {
	// Name is the bond interface name.
	Name string

	// Mode is the bonding mode (e.g., "802.3ad", "active-backup").
	Mode string

	// ActiveInterfaces lists the currently active member interfaces.
	ActiveInterfaces []string

	// Speed is the aggregated link speed (e.g., "200Gbps").
	Speed string
}

// BondManager manages Linux bonding interfaces on the node.
type BondManager struct {
	logger *slog.Logger
}

// NewBondManager creates a new BondManager.
func NewBondManager(logger *slog.Logger) *BondManager {
	return &BondManager{
		logger: logger.With("component", "bond-manager"),
	}
}

// EnsureBond creates or reconfigures a Linux bond interface with the specified parameters.
func (m *BondManager) EnsureBond(ctx context.Context, name, mode string, interfaces []string, mtu int) error {
	m.logger.InfoContext(ctx, "ensuring bond",
		"name", name, "mode", mode, "interfaces", interfaces, "mtu", mtu)

	// Create the bond master interface if it doesn't exist.
	if err := m.runIP(ctx, "link", "add", name, "type", "bond", "mode", mode); err != nil {
		// If it already exists, that's fine; try to set the mode.
		m.logger.DebugContext(ctx, "bond may already exist, continuing",
			"name", name, "error", err)
	}

	// Set MTU on the bond interface.
	if mtu > 0 {
		if err := m.runIP(ctx, "link", "set", name, "mtu", strconv.Itoa(mtu)); err != nil {
			return fmt.Errorf("setting MTU %d on bond %s: %w", mtu, name, err)
		}
	}

	// Bring the bond interface down before adding members.
	_ = m.runIP(ctx, "link", "set", name, "down")

	// Enslave member interfaces.
	for _, iface := range interfaces {
		// Bring the member down first (required for enslaving).
		if err := m.runIP(ctx, "link", "set", iface, "down"); err != nil {
			return fmt.Errorf("bringing down interface %s: %w", iface, err)
		}
		if err := m.runIP(ctx, "link", "set", iface, "master", name); err != nil {
			return fmt.Errorf("enslaving %s to bond %s: %w", iface, name, err)
		}
		m.logger.InfoContext(ctx, "enslaved interface to bond",
			"interface", iface, "bond", name)
	}

	// Bring the bond interface up.
	if err := m.runIP(ctx, "link", "set", name, "up"); err != nil {
		return fmt.Errorf("bringing up bond %s: %w", name, err)
	}

	// Bring member interfaces up.
	for _, iface := range interfaces {
		if err := m.runIP(ctx, "link", "set", iface, "up"); err != nil {
			return fmt.Errorf("bringing up interface %s: %w", iface, err)
		}
	}

	m.logger.InfoContext(ctx, "bond configured",
		"name", name, "mode", mode, "interfaces", interfaces, "mtu", mtu)
	return nil
}

// RemoveBond deletes a Linux bond interface.
func (m *BondManager) RemoveBond(ctx context.Context, name string) error {
	m.logger.InfoContext(ctx, "removing bond", "name", name)

	if err := m.runIP(ctx, "link", "set", name, "down"); err != nil {
		m.logger.Warn("failed to bring down bond before removal",
			"name", name, "error", err)
	}
	if err := m.runIP(ctx, "link", "delete", name, "type", "bond"); err != nil {
		return fmt.Errorf("deleting bond %s: %w", name, err)
	}
	return nil
}

// GetBondStatus returns the current status of a bond interface.
func (m *BondManager) GetBondStatus(ctx context.Context, name string) (*BondStatus, error) {
	status := &BondStatus{Name: name}

	// Read bond mode from sysfs.
	modeOutput, err := m.readSysfs(ctx, fmt.Sprintf("/sys/class/net/%s/bonding/mode", name))
	if err != nil {
		return nil, fmt.Errorf("reading bond mode for %s: %w", name, err)
	}
	// Mode output is like "802.3ad 4", take the first field.
	parts := strings.Fields(modeOutput)
	if len(parts) > 0 {
		status.Mode = parts[0]
	}

	// Read active slave interfaces.
	slavesOutput, err := m.readSysfs(ctx, fmt.Sprintf("/sys/class/net/%s/bonding/slaves", name))
	if err != nil {
		return nil, fmt.Errorf("reading bond slaves for %s: %w", name, err)
	}
	if slavesOutput != "" {
		status.ActiveInterfaces = strings.Fields(slavesOutput)
	}

	// Read speed.
	speedOutput, err := m.readSysfs(ctx, fmt.Sprintf("/sys/class/net/%s/speed", name))
	if err == nil && speedOutput != "" {
		speed, parseErr := strconv.Atoi(strings.TrimSpace(speedOutput))
		if parseErr == nil && speed > 0 {
			if speed >= 1000 {
				status.Speed = fmt.Sprintf("%dGbps", speed/1000)
			} else {
				status.Speed = fmt.Sprintf("%dMbps", speed)
			}
		}
	}

	return status, nil
}

// runIP executes an ip command with the given arguments.
func (m *BondManager) runIP(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %v: %w (output: %s)", args, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// readSysfs reads a sysfs file and returns its trimmed content.
func (m *BondManager) readSysfs(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "cat", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return strings.TrimSpace(string(output)), nil
}
