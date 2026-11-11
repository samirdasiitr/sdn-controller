/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/sdncontroller/sdncontroller/pkg/hw"
	pkgovs "github.com/sdncontroller/sdncontroller/pkg/ovs"
)

const usage = `sdnctl - SDN controller debugging CLI

Usage:
  sdnctl <command> [flags]

Commands:
  status    Show node agent status (OVS version, bridges)
  flows     Dump OVS flows for a bridge
  tunnels   List tunnel ports on a bridge
  hw        Show hardware NIC and DPU information

Flags:
  --bridge  OVS bridge name (default: "br-int")
  --help    Show this help message
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	command := os.Args[1]

	// Handle help.
	if command == "--help" || command == "-h" || command == "help" {
		fmt.Print(usage)
		os.Exit(0)
	}

	// Parse flags for subcommands.
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	bridge := fs.String("bridge", "br-int", "OVS bridge name")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	switch command {
	case "status":
		runStatus(ctx, *bridge)
	case "flows":
		runFlows(ctx, *bridge)
	case "tunnels":
		runTunnels(ctx, *bridge)
	case "hw":
		runHW()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		fmt.Print(usage)
		os.Exit(1)
	}
}

// runStatus shows the OVS version and bridge list.
func runStatus(ctx context.Context, bridge string) {
	client := pkgovs.NewClient()

	fmt.Println("=== SDN Node Agent Status ===")
	fmt.Println()

	// OVS version.
	version, err := client.GetOVSVersion(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting OVS version: %v\n", err)
	} else {
		fmt.Printf("OVS Version: %s\n", version)
	}

	// Bridge list.
	fmt.Println()
	fmt.Println("Bridges:")
	out, err := runCommand("ovs-vsctl", "list-br")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error listing bridges: %v\n", err)
	} else {
		for _, br := range strings.Split(strings.TrimSpace(out), "\n") {
			if br == "" {
				continue
			}
			exists, _ := client.BridgeExists(ctx, br)
			status := "unknown"
			if exists {
				status = "active"
			}
			fmt.Printf("  %-20s %s\n", br, status)
		}
	}

	// Flow count for the specified bridge.
	fmt.Println()
	flows, err := client.DumpFlows(ctx, bridge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting flow count for %s: %v\n", bridge, err)
	} else {
		count := 0
		for _, line := range strings.Split(flows, "\n") {
			l := strings.TrimSpace(line)
			if l != "" && !strings.HasPrefix(l, "NXST_FLOW") && !strings.HasPrefix(l, "OFPST_FLOW") {
				count++
			}
		}
		fmt.Printf("Flow count on %s: %d\n", bridge, count)
	}
}

// runFlows dumps OVS flows for the specified bridge.
func runFlows(ctx context.Context, bridge string) {
	client := pkgovs.NewClient()

	fmt.Printf("=== OVS Flows on %s ===\n\n", bridge)

	flows, err := client.DumpFlows(ctx, bridge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error dumping flows: %v\n", err)
		os.Exit(1)
	}

	if flows == "" {
		fmt.Println("No flows found.")
		return
	}

	fmt.Println(flows)
}

// runTunnels lists tunnel ports on the specified bridge.
func runTunnels(ctx context.Context, bridge string) {
	fmt.Printf("=== Tunnels on %s ===\n\n", bridge)

	// Use ovs-vsctl to list ports and their types.
	out, err := runCommand("ovs-vsctl", "list-ports", bridge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing ports on %s: %v\n", bridge, err)
		os.Exit(1)
	}

	ports := strings.Split(strings.TrimSpace(out), "\n")
	if len(ports) == 0 || (len(ports) == 1 && ports[0] == "") {
		fmt.Println("No ports found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tTYPE\tREMOTE_IP\tOPTIONS")

	for _, port := range ports {
		port = strings.TrimSpace(port)
		if port == "" {
			continue
		}

		// Get interface type.
		ifType, _ := runCommand("ovs-vsctl", "get", "interface", port, "type")
		ifType = strings.TrimSpace(strings.Trim(ifType, "\""))

		// Only show tunnel ports.
		if ifType != "vxlan" && ifType != "gre" && ifType != "ipip" && ifType != "geneve" {
			continue
		}

		// Get options.
		opts, _ := runCommand("ovs-vsctl", "get", "interface", port, "options")
		opts = strings.TrimSpace(opts)

		// Extract remote_ip from options.
		remoteIP := ""
		for _, kv := range strings.Split(strings.Trim(opts, "{}"), ",") {
			kv = strings.TrimSpace(kv)
			if strings.HasPrefix(kv, "remote_ip=") {
				remoteIP = strings.TrimPrefix(kv, "remote_ip=")
				remoteIP = strings.Trim(remoteIP, "\"")
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", port, ifType, remoteIP, opts)
	}

	w.Flush()
}

// runHW shows hardware NIC and DPU information.
func runHW() {
	fmt.Println("=== Hardware Information ===")
	fmt.Println()

	// NIC detection.
	fmt.Println("Network Interfaces:")
	nics, err := hw.DiscoverNICs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error discovering NICs: %v\n", err)
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "  DEVICE\tVENDOR\tMODEL\tDRIVER\tSPEED\tOFFLOAD\tSWITCHDEV\tVFs\tPCI")
		for _, nic := range nics {
			offload := "no"
			if nic.OffloadCapable {
				offload = "yes"
			}
			switchdev := "no"
			if nic.SwitchdevMode {
				switchdev = "yes"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
				nic.Device, nic.Vendor, nic.Model, nic.Driver,
				nic.Speed, offload, switchdev, nic.NumVFs, nic.PCIAddr)
		}
		w.Flush()
	}

	// DPU detection.
	fmt.Println()
	fmt.Println("DPU:")
	present, model := hw.DetectDPU()
	if present {
		fmt.Printf("  Model: %s\n", model)
		fmt.Println("  Status: detected")
	} else {
		fmt.Println("  Status: not detected")
	}
}

// runCommand executes a command and returns its stdout output.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w (output: %s)", name, args, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
