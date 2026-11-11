package vnfvm

import (
	"strings"
	"testing"
)

func TestGenerators(t *testing.T) {
	vm := VM{
		Domain: "vnf-sdn-demo-ids-vnf-0", Hostname: "ids-vnf-0", MemoryMB: 1024, VCPUs: 2,
		DiskPath: "/d/disk.qcow2", Seed: "/d/seed.iso",
		NICs: []NIC{
			{MAC: "52:54:00:aa:bb:01", IP: "10.100.1.2", Gateway: "10.100.1.1", PrefixLen: 24, MTU: 1450, Bridge: "br-production-v"},
			{MAC: "52:54:00:aa:bb:02", IP: "10.100.2.2", Gateway: "10.100.2.1", PrefixLen: 24, Bridge: "br-production-v"},
		},
		Packages: []string{"suricata"}, Commands: []string{"systemctl enable --now suricata", "echo it's fine"},
		AuthorizedKey: "ssh-ed25519 AAAA test",
	}
	nc := vm.networkConfig()
	for _, want := range []string{`macaddress: "52:54:00:aa:bb:01"`, "set-name: eth0", "mtu: 1450", "10.100.1.2/24", "to: default, via: 10.100.1.1", "set-name: eth1", "10.100.2.2/24"} {
		if !strings.Contains(nc, want) {
			t.Errorf("network-config missing %q:\n%s", want, nc)
		}
	}
	if strings.Count(nc, "to: default") != 1 {
		t.Errorf("default route must be set only on the first NIC:\n%s", nc)
	}
	ud := vm.userData()
	for _, want := range []string{"- suricata", "'systemctl enable --now suricata'", "'echo it''s fine'", "ssh-ed25519 AAAA test", "hostname: ids-vnf-0"} {
		if !strings.Contains(ud, want) {
			t.Errorf("user-data missing %q:\n%s", want, ud)
		}
	}
	x := vm.domainXML()
	for _, want := range []string{"<name>vnf-sdn-demo-ids-vnf-0</name>", "<virtualport type='openvswitch'/>", "mac address='52:54:00:aa:bb:02'", "<description>sdn-managed-vnf</description>", "<memory unit='MiB'>1024</memory>"} {
		if !strings.Contains(x, want) {
			t.Errorf("domain xml missing %q", want)
		}
	}
	if strings.Count(x, "<interface ") != 2 {
		t.Errorf("expected 2 interfaces")
	}
}
