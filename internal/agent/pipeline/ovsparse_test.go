package pipeline

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestFlowsParseWithOVS feeds every scenario through `ovs-ofctl --bundle add-flows` against a
// throwaway bridge when OVS is available (set OVS_PARSE_TEST=1; needs root).
func TestFlowsParseWithOVS(t *testing.T) {
	if os.Getenv("OVS_PARSE_TEST") == "" {
		t.Skip("set OVS_PARSE_TEST=1 on a host with Open vSwitch")
	}
	for name, in := range map[string]Input{"services": testInput(), "isolation": isolatedInput(), "natfull": natInput("FullNAT"), "natdnat": natInput("DNAT")} {
		t.Run(name, func(t *testing.T) {
			o := Build(in)
			const br = "br-parsetest"
			exec.Command("ovs-vsctl", "--if-exists", "del-br", br).Run()
			if out, err := exec.Command("ovs-vsctl", "add-br", br).CombinedOutput(); err != nil {
				t.Skipf("no ovs: %v %s", err, out)
			}
			defer exec.Command("ovs-vsctl", "del-br", br).Run()
			for _, g := range o.Groups {
				if out, err := exec.Command("ovs-ofctl", "-O", "OpenFlow15", "add-group", br, "group_id="+strings.TrimSpace(itoaP(g.ID))+","+g.Def).CombinedOutput(); err != nil {
					t.Fatalf("group: %v %s", err, out)
				}
			}
			cmd := exec.Command("ovs-ofctl", "--bundle", "-O", "OpenFlow15", "add-flows", br, "-")
			cmd.Stdin = strings.NewReader(strings.Join(o.Flows, "\n") + "\n")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("OVS rejected the pipeline: %v\n%s", err, out)
			}
		})
	}
}

func itoaP(i int) string {
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}
