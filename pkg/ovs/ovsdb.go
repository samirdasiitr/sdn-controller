package ovs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps OVS command-line tools for bridge/port/flow management.
// In production, this would use the OVSDB protocol directly; for now we shell out to ovs-vsctl/ovs-ofctl.
type Client struct {
	ovsVsctl string
	ovsOfctl string
}

// NewClient creates a new OVS client.
func NewClient() *Client {
	return &Client{
		ovsVsctl: "ovs-vsctl",
		ovsOfctl: "ovs-ofctl",
	}
}

// BridgeExists checks if an OVS bridge exists.
func (c *Client) BridgeExists(ctx context.Context, bridge string) (bool, error) {
	err := c.run(ctx, c.ovsVsctl, "br-exists", bridge)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateBridge creates an OVS bridge with the specified datapath type.
func (c *Client) CreateBridge(ctx context.Context, bridge, datapathType string) error {
	args := []string{"--may-exist", "add-br", bridge}
	if datapathType != "" && datapathType != "system" {
		args = append(args, "--", "set", "bridge", bridge, fmt.Sprintf("datapath_type=%s", datapathType))
	}
	return c.run(ctx, c.ovsVsctl, args...)
}

// DeleteBridge deletes an OVS bridge.
func (c *Client) DeleteBridge(ctx context.Context, bridge string) error {
	return c.run(ctx, c.ovsVsctl, "--if-exists", "del-br", bridge)
}

// AddPort adds a port to a bridge.
func (c *Client) AddPort(ctx context.Context, bridge, port string, options map[string]string) error {
	args := []string{"--may-exist", "add-port", bridge, port}
	if len(options) > 0 {
		args = append(args, "--")
		for k, v := range options {
			args = append(args, "set", "interface", port, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return c.run(ctx, c.ovsVsctl, args...)
}

// DeletePort deletes a port from a bridge.
func (c *Client) DeletePort(ctx context.Context, bridge, port string) error {
	return c.run(ctx, c.ovsVsctl, "--if-exists", "del-port", bridge, port)
}

// AddVXLANPort adds a VXLAN tunnel port.
func (c *Client) AddVXLANPort(ctx context.Context, bridge, port, remoteIP string, vni int32) error {
	return c.run(ctx, c.ovsVsctl, "--may-exist", "add-port", bridge, port,
		"--", "set", "interface", port,
		"type=vxlan",
		fmt.Sprintf("options:remote_ip=%s", remoteIP),
		fmt.Sprintf("options:key=%d", vni),
	)
}

// AddIPIPPort adds an IPIP tunnel port (using ip_gre type with csum).
func (c *Client) AddIPIPPort(ctx context.Context, bridge, port, localIP, remoteIP string) error {
	return c.run(ctx, c.ovsVsctl, "--may-exist", "add-port", bridge, port,
		"--", "set", "interface", port,
		"type=ipip",
		fmt.Sprintf("options:local_ip=%s", localIP),
		fmt.Sprintf("options:remote_ip=%s", remoteIP),
	)
}

// AddGREPort adds a GRE tunnel port.
func (c *Client) AddGREPort(ctx context.Context, bridge, port, remoteIP string) error {
	return c.run(ctx, c.ovsVsctl, "--may-exist", "add-port", bridge, port,
		"--", "set", "interface", port,
		"type=gre",
		fmt.Sprintf("options:remote_ip=%s", remoteIP),
	)
}

// SetPortVLAN sets the VLAN tag on a port.
func (c *Client) SetPortVLAN(ctx context.Context, port string, vlan int32) error {
	return c.run(ctx, c.ovsVsctl, "set", "port", port, fmt.Sprintf("tag=%d", vlan))
}

// AddFlow adds an OpenFlow rule to a bridge.
func (c *Client) AddFlow(ctx context.Context, bridge, flow string) error {
	return c.run(ctx, c.ovsOfctl, "add-flow", bridge, flow)
}

// DeleteFlows deletes OpenFlow rules matching a criteria.
func (c *Client) DeleteFlows(ctx context.Context, bridge, match string) error {
	if match == "" {
		return c.run(ctx, c.ovsOfctl, "del-flows", bridge)
	}
	return c.run(ctx, c.ovsOfctl, "del-flows", bridge, match)
}

// DumpFlows returns all flows on a bridge.
func (c *Client) DumpFlows(ctx context.Context, bridge string) (string, error) {
	return c.runOutput(ctx, c.ovsOfctl, "dump-flows", bridge)
}

// GetOVSVersion returns the OVS version.
func (c *Client) GetOVSVersion(ctx context.Context) (string, error) {
	out, err := c.runOutput(ctx, c.ovsVsctl, "--version")
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}
	return "unknown", nil
}

// SetInterfaceMTU sets the MTU on an OVS interface.
func (c *Client) SetInterfaceMTU(ctx context.Context, iface string, mtu int) error {
	return c.run(ctx, c.ovsVsctl, "set", "interface", iface, fmt.Sprintf("mtu_request=%d", mtu))
}

func (c *Client) run(ctx context.Context, cmd string, args ...string) error {
	command := exec.CommandContext(ctx, cmd, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w (output: %s)", cmd, args, err, string(output))
	}
	return nil
}

func (c *Client) runOutput(ctx context.Context, cmd string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, cmd, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v failed: %w (output: %s)", cmd, args, err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}
