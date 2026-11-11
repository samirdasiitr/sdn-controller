// Package vnfvm boots and removes the guest VMs behind VM-type VNFs on this node.
package vnfvm

import (
	"fmt"
	"strings"
)

// NIC is one guest network interface (derived from a VNic).
type NIC struct {
	MAC, IP, Gateway string
	PrefixLen        int
	MTU              int
	Bridge           string
}

// VM is everything needed to define one guest.
type VM struct {
	Domain, Hostname string
	MemoryMB, VCPUs  int
	DiskPath, Seed   string
	NICs             []NIC
	Packages         []string
	Commands         []string
	AuthorizedKey    string
}

const marker = "sdn-managed-vnf"

func (v VM) metaData() string {
	return fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", v.Domain, v.Hostname)
}

func (v VM) userData() string {
	var b strings.Builder
	b.WriteString("#cloud-config\nhostname: " + v.Hostname + "\n")
	if v.AuthorizedKey != "" {
		b.WriteString("users:\n  - name: ubuntu\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    shell: /bin/bash\n    ssh_authorized_keys:\n      - " + v.AuthorizedKey + "\n")
	}
	b.WriteString("package_update: true\npackages:\n  - qemu-guest-agent\n")
	for _, p := range v.Packages {
		b.WriteString("  - " + p + "\n")
	}
	b.WriteString("runcmd:\n  - systemctl enable --now qemu-guest-agent\n")
	for _, c := range v.Commands {
		b.WriteString("  - " + yamlQuote(c) + "\n")
	}
	return b.String()
}

func yamlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (v VM) networkConfig() string {
	var b strings.Builder
	b.WriteString("version: 2\nethernets:\n")
	for i, n := range v.NICs {
		fmt.Fprintf(&b, "  nic%d:\n    match: {macaddress: \"%s\"}\n    set-name: eth%d\n", i, n.MAC, i)
		if n.MTU > 0 {
			fmt.Fprintf(&b, "    mtu: %d\n", n.MTU)
		}
		fmt.Fprintf(&b, "    addresses: [%s/%d]\n", n.IP, n.PrefixLen)
		if i == 0 && n.Gateway != "" {
			fmt.Fprintf(&b, "    routes: [{to: default, via: %s}]\n    nameservers: {addresses: [8.8.8.8, 1.1.1.1]}\n", n.Gateway)
		}
	}
	return b.String()
}

func (v VM) domainXML() string {
	var ifs strings.Builder
	for _, n := range v.NICs {
		fmt.Fprintf(&ifs, `    <interface type='bridge'>
      <mac address='%s'/>
      <source bridge='%s'/>
      <virtualport type='openvswitch'/>
      <model type='virtio'/>
    </interface>
`, n.MAC, n.Bridge)
	}
	return fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <description>%s</description>
  <memory unit='MiB'>%d</memory>
  <vcpu>%d</vcpu>
  <os><type arch='x86_64' machine='q35'>hvm</type><boot dev='hd'/></os>
  <features><acpi/><apic/></features>
  <cpu mode='host-passthrough'/>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='file' device='disk'><driver name='qemu' type='qcow2'/><source file='%s'/><target dev='vda' bus='virtio'/></disk>
    <disk type='file' device='cdrom'><driver name='qemu' type='raw'/><source file='%s'/><target dev='sda' bus='sata'/><readonly/></disk>
%s    <serial type='pty'><target port='0'/></serial>
    <console type='pty'/>
    <channel type='unix'><target type='virtio' name='org.qemu.guest_agent.0'/></channel>
  </devices>
</domain>
`, v.Domain, marker, v.MemoryMB, v.VCPUs, v.DiskPath, v.Seed, ifs.String())
}
