package pipeline

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
)

func has(t *testing.T, flows []string, sub string) {
	t.Helper()
	for _, f := range flows {
		if strings.Contains(f, sub) {
			return
		}
	}
	t.Errorf("no flow contains %q\n%s", sub, strings.Join(flows, "\n"))
}

func hasNot(t *testing.T, flows []string, sub string) {
	t.Helper()
	for _, f := range flows {
		if strings.Contains(f, sub) {
			t.Errorf("unexpected flow %q", f)
		}
	}
}

func TestPortMasks(t *testing.T) {
	cases := map[[2]int][]string{
		{80, 80}:      {"0x0050"},
		{1024, 65535}: {"0x0400/0xfc00", "0x0800/0xf800", "0x1000/0xf000", "0x2000/0xe000", "0x4000/0xc000", "0x8000/0x8000"},
		{8080, 8081}:  {"0x1f90/0xfffe"},
		{1, 3}:        {"0x0001", "0x0002/0xfffe"},
	}
	for r, want := range cases {
		got := PortMasks(r[0], r[1])
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("PortMasks%v = %v, want %v", r, got, want)
		}
	}
}

func testInput() Input {
	fw := servicev1alpha1.Firewall{
		ObjectMeta: metav1.ObjectMeta{Name: "web-fw", Namespace: "d"},
		Spec: servicev1alpha1.FirewallSpec{
			DefaultAction: servicev1alpha1.FirewallActionDeny, Stateful: true, SubnetRefs: []string{"web"},
			Rules: []servicev1alpha1.FirewallRule{
				{Name: "http", Priority: 100, Direction: servicev1alpha1.FirewallDirectionIngress, Action: servicev1alpha1.FirewallActionAllow, Match: servicev1alpha1.FirewallRuleMatch{Protocol: "TCP", DstPort: "80"}},
				{Name: "ssh-deny", Priority: 50, Direction: servicev1alpha1.FirewallDirectionIngress, Action: servicev1alpha1.FirewallActionDeny, Match: servicev1alpha1.FirewallRuleMatch{Protocol: "TCP", SrcCIDR: "10.9.0.0/16", DstPort: "22"}},
				{Name: "out", Priority: 10, Direction: servicev1alpha1.FirewallDirectionEgress, Action: servicev1alpha1.FirewallActionAllow, Match: servicev1alpha1.FirewallRuleMatch{Protocol: "Any"}},
			},
		},
	}
	lb := servicev1alpha1.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "d"},
		Spec: servicev1alpha1.LoadBalancerSpec{
			Frontend: servicev1alpha1.LBFrontend{IP: "10.0.1.100", Ports: []servicev1alpha1.LBPort{{Protocol: "TCP", Port: 80, TargetPort: 8080}}},
			Backends: []servicev1alpha1.LBBackend{{IP: "10.0.1.10", Weight: 100}, {IP: "10.0.1.11", Weight: 50}},
		},
	}
	nat := gwv1alpha1.NATGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "nat", Namespace: "d"},
		Spec:       gwv1alpha1.NATGatewaySpec{SubnetRefs: []string{"app"}, ExternalIP: "203.0.113.20"},
	}
	return Input{
		VPCCIDRs: []string{"10.0.0.0/16"}, Subnets: map[string]string{"web": "10.0.1.0/24", "app": "10.0.2.0/24"},
		Routes: []string{"10.0.1.0/24"}, TunnelPorts: []int{7}, RemoteMACs: map[string]int{"52:54:00:00:00:99": 7},
		IPToMAC: map[string]string{"10.0.1.10": "52:54:00:00:00:10", "10.0.1.11": "52:54:00:00:00:11"},
		Ports: []Port{
			{Key: "d/a", IP: "10.0.1.10", MAC: "52:54:00:00:00:10", Subnet: "web", OFPort: 2, PortSecurity: true},
			{Key: "d/b", IP: "10.0.2.10", MAC: "52:54:00:00:00:20", Subnet: "app", OFPort: 3, PortSecurity: false},
		},
		Firewalls: []servicev1alpha1.Firewall{fw}, LBs: []servicev1alpha1.LoadBalancer{lb}, NATs: []gwv1alpha1.NATGateway{nat},
	}
}

func TestBuildFirewall(t *testing.T) {
	o := Build(testInput())
	// rule priorities: lower number = higher OpenFlow priority
	has(t, o.Flows, "table=30,priority=65486,ct_state=+trk+new,reg1=1,tcp,nw_src=10.9.0.0/16,tp_dst=0x0016,actions=drop")
	has(t, o.Flows, "table=30,priority=65436,ct_state=+trk+new,reg1=1,tcp,tp_dst=0x0050,actions=ct(commit,zone=NXM_NX_REG0[0..15]),ct_clear,resubmit(,40)")
	has(t, o.Flows, "table=10,priority=65526,ct_state=+trk+new,reg1=1,ip,actions=ct(commit")
	has(t, o.Flows, "table=30,priority=0,reg1=1,actions=drop") // default Deny
	has(t, o.Flows, "table=30,priority=100,ct_state=+trk+est,reg1=1")
	// only the VNic in the protected subnet gets dispatched
	has(t, o.Flows, "table=5,priority=100,ip,in_port=2,actions=load:2->NXM_NX_REG0[0..15],load:1->NXM_NX_REG1[],ct(table=10,zone=NXM_NX_REG0[0..15])")
	hasNot(t, o.Flows, "in_port=3,actions=load")
	// port security only where enabled
	has(t, o.Flows, "in_port=2,dl_src=52:54:00:00:00:10,ip,nw_src=10.0.1.10,actions=resubmit(,5)")
	hasNot(t, o.Flows, "in_port=3,actions=drop")
}

func TestBuildLBAndNAT(t *testing.T) {
	o := Build(testInput())
	if len(o.Groups) != 2 {
		t.Fatalf("want 2 groups (local+tunnel), got %d", len(o.Groups))
	}
	g := o.Groups[0].Def
	for _, w := range []string{"selection_method=dp_hash", "bucket=weight:100", "bucket=weight:50", "set_field:52:54:00:00:00:10->eth_dst", "nat(dst=10.0.1.10:8080)", "zone=40000"} {
		if !strings.Contains(g, w) {
			t.Errorf("group missing %q: %s", w, g)
		}
	}
	has(t, o.Flows, "table=15,priority=110,ip,tcp,nw_dst=10.0.1.100,tp_dst=80,ct_state=-trk,actions=group:1001")
	has(t, o.Flows, "table=15,priority=115,in_port=7,ip,tcp,nw_dst=10.0.1.100,tp_dst=80,ct_state=-trk,actions=group:1002")
	has(t, o.Flows, "arp,arp_tpa=10.0.1.100,arp_op=1,actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],set_field:0a:5d:0a:00:01:64->eth_src")
	has(t, o.Flows, "table=15,priority=100,ip,tcp,nw_src=10.0.1.10,tp_src=8080,ct_state=-trk,actions=ct(table=19,zone=40000,nat)")
	has(t, o.Flows, "table=16,priority=100,ip,ct_zone=40000,ct_state=+trk+new,actions=ct_clear,ct(commit,table=19,zone=40001,nat(src=10.0.1.100))")

	has(t, o.Flows, "table=16,priority=100,ip,ct_zone=40000,ct_state=+trk+est,actions=ct_clear,ct(table=19,zone=40001,nat)")
	has(t, o.Flows, "table=20,priority=300,ip,nw_dst=10.0.0.0/16,actions=resubmit(,25)")
	has(t, o.Flows, "table=20,priority=200,ip,ct_state=-trk,nw_src=10.0.2.0/24,actions=ct(table=21,zone=50000,nat)")
	has(t, o.Flows, "table=21,priority=210,ip,ct_zone=50000,ct_state=+trk+new,nw_src=10.0.2.0/24,actions=ct(commit,zone=50000,nat(src=203.0.113.20)),ct_clear,resubmit(,25)")
	has(t, o.Flows, "table=20,priority=200,ip,ct_state=-trk,nw_dst=203.0.113.20,actions=ct(table=21,zone=50000,nat)")

	has(t, o.Flows, "table=40,priority=100,ip,dl_dst=0a:5d:cb:00:71:14,nw_dst=10.0.1.10,actions=set_field:52:54:00:00:00:10->eth_dst,resubmit(,40)")
	if r := o.Reports["LoadBalancer/d/lb"]; r == nil || r.Flows == 0 || r.Error != "" {
		t.Errorf("bad LB report: %+v", r)
	}
}

func TestZonesDifferPerVPC(t *testing.T) {
	a, b := testInput(), testInput()
	a.VNI, b.VNI = 1000, 1001
	if a.aclZone(2) == b.aclZone(2) || a.lbZoneBase() == b.lbZoneBase() || a.natZoneBase() == b.natZoneBase() {
		t.Errorf("zones must differ between VPCs: %d/%d %d/%d %d/%d", a.aclZone(2), b.aclZone(2), a.lbZoneBase(), b.lbZoneBase(), a.natZoneBase(), b.natZoneBase())
	}
}

func TestHairpinForwarding(t *testing.T) {
	o := Build(testInput())
	has(t, o.Flows, "table=40,priority=120,in_port=7,dl_dst=52:54:00:00:00:99,actions=IN_PORT")
	has(t, o.Flows, "table=40,priority=110,dl_dst=52:54:00:00:00:99,actions=output:7")
}

func TestUnsupported(t *testing.T) {
	in := testInput()
	in.LBs[0].Spec.Type = servicev1alpha1.LBTypeL7
	o := Build(in)
	if o.Reports["LoadBalancer/d/lb"].Error == "" {
		t.Errorf("L7 LB must report an error: %+v", o.Reports["LoadBalancer/d/lb"])
	}
}

func lbKey() string { return ReportKey("LoadBalancer", "d", "lb") }

func TestLBHealthExcludesBackend(t *testing.T) {
	in := testInput()
	in.LBRuntime = map[string]LBRuntime{lbKey(): {Health: map[string]bool{"10.0.1.11": false}}}
	o := Build(in)
	if strings.Contains(o.Groups[0].Def, "10.0.1.11") || !strings.Contains(o.Groups[0].Def, "10.0.1.10") {
		t.Errorf("unhealthy backend must be removed from the group: %s", o.Groups[0].Def)
	}
	// replies of the unhealthy backend are still un-NATed (existing connections)
	has(t, o.Flows, "nw_src=10.0.1.11,tp_src=8080,ct_state=-trk,actions=ct(table=19")
	in.LBRuntime[lbKey()] = LBRuntime{Health: map[string]bool{"10.0.1.10": false, "10.0.1.11": false}}
	o = Build(in)
	if !strings.Contains(o.Groups[0].Def, "10.0.1.11") || !strings.Contains(strings.Join(o.Reports[lbKey()].Warnings, ";"), "fail-open") {
		t.Errorf("all unhealthy must fail open with a warning: %+v", o.Reports[lbKey()])
	}
}

func TestLBLeastConnWeights(t *testing.T) {
	in := testInput()
	in.LBs[0].Spec.Algorithm = servicev1alpha1.LBAlgorithmLeastConn
	in.LBs[0].Spec.Backends = []servicev1alpha1.LBBackend{{IP: "10.0.1.10", Weight: 1}, {IP: "10.0.1.11", Weight: 1}}
	in.LBRuntime = map[string]LBRuntime{lbKey(): {Conns: map[string]int64{"10.0.1.10": 9, "10.0.1.11": 0}}}
	g := Build(in).Groups[0].Def
	// busy backend gets a much smaller weight than the idle one
	i10, i11 := strings.Index(g, "10.0.1.10"), strings.Index(g, "10.0.1.11")
	if i10 < 0 || i11 < 0 || !strings.Contains(g, "bucket=weight:10,") || !strings.Contains(g, "bucket=weight:100,") {
		t.Errorf("unexpected LeastConn weights: %s", g)
	}
}

func TestLBBackendMACFromRuntime(t *testing.T) {
	in := testInput()
	in.LBs[0].Spec.Backends = append(in.LBs[0].Spec.Backends, servicev1alpha1.LBBackend{IP: "10.0.1.50", Weight: 10})
	in.LBRuntime = map[string]LBRuntime{lbKey(): {MACs: map[string]string{"10.0.1.50": "aa:bb:cc:dd:ee:50"}}}
	o := Build(in)
	if !strings.Contains(o.Groups[0].Def, "set_field:aa:bb:cc:dd:ee:50->eth_dst") {
		t.Errorf("non-VNic backend MAC from runtime must be used: %s", o.Groups[0].Def)
	}
}

func TestLBSessionAffinityLearn(t *testing.T) {
	in := testInput()
	in.LBs[0].Spec.SessionAffinity = &servicev1alpha1.SessionAffinity{Enabled: true, Timeout: metav1.Duration{Duration: 90e9}}
	o := Build(in)
	if strings.Contains(o.Groups[0].Def, "learn(") {
		t.Errorf("learn must not be inside group buckets (OVS does not execute it there): %s", o.Groups[0].Def)
	}
	learn := "learn(table=18,idle_timeout=90,priority=100,eth_type=0x800,ip_proto=6,NXM_OF_IP_SRC[],nw_dst=10.0.1.100,tcp_dst=80,load:1->NXM_NX_REG2[])"
	has(t, o.Flows, "table=19,priority=110,ip,tcp,ct_zone=40000,ct_state=+trk+new,nw_dst=10.0.1.10,tp_dst=8080,actions="+learn+",ct_clear,resubmit(,20)")
	has(t, o.Flows, "table=16,priority=110,ip,tcp,ct_zone=40000,ct_state=+trk+new,nw_dst=10.0.1.11,tp_dst=8080,actions=learn(")
	has(t, o.Flows, "table=15,priority=140,ip,tcp,nw_dst=10.0.1.100,tp_dst=80,ct_state=-trk,reg3=0,actions=load:1->NXM_NX_REG3[],resubmit(,18),resubmit(,15)")
	has(t, o.Flows, "reg3=1,reg2=1,actions=set_field:52:54:00:00:00:10->eth_dst,ct(commit")
	has(t, o.Flows, "reg3=1,actions=group:1001")
}

func TestLBCounterFlows(t *testing.T) {
	o := Build(testInput())
	has(t, o.Flows, "table=19,priority=100,ip,ct_zone=40000,ct_state=+trk+new,actions=ct_clear,resubmit(,20)")
	found := false
	for ck, ref := range o.Cookies {
		if ref.Kind == "LoadBalancer" && ref.Rule == "connections" && ck != 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected a connections cookie for the LB")
	}
}

func natInput(typ gwv1alpha1.NATType) Input {
	in := testInput()
	in.NATs[0].Spec.Type = typ
	in.NATs[0].Spec.PortForwards = []gwv1alpha1.PortForward{{Name: "web", Protocol: "TCP", ExternalPort: 8080, InternalIP: "10.0.1.10", InternalPort: 80}}
	in.SubnetGateways = map[string]string{"web": "10.0.1.1", "app": "10.0.2.1"}
	return in
}

func TestNATDNAT(t *testing.T) {
	o := Build(natInput(gwv1alpha1.NATTypeDNAT))
	has(t, o.Flows, "table=20,priority=400,ip,tcp,nw_dst=203.0.113.20,tp_dst=8080,ct_state=-trk,actions=ct(table=21,zone=50030,nat)")
	has(t, o.Flows, "table=21,priority=220,ip,ct_zone=50030,ct_state=+trk+new,tcp,nw_dst=203.0.113.20,tp_dst=8080,actions=set_field:52:54:00:00:00:10->eth_dst,ct(commit,table=25,zone=50030,nat(dst=10.0.1.10:80))")
	has(t, o.Flows, "table=20,priority=400,ip,tcp,nw_src=10.0.1.10,tp_src=80,ct_state=-trk,actions=ct(table=21,zone=50030,nat)")
	hasNot(t, o.Flows, "nat(src=203.0.113.20") // DNAT-only gateways do not translate outbound traffic
	if r := o.Reports["NATGateway/d/nat"]; r.Error != "" {
		t.Errorf("DNAT must compile: %+v", r)
	}
}

func TestNATFullNAT(t *testing.T) {
	o := Build(natInput(gwv1alpha1.NATTypeFullNAT))
	has(t, o.Flows, "actions=set_field:52:54:00:00:00:10->eth_dst,ct(commit,table=22,zone=50030,nat(dst=10.0.1.10:80))")
	has(t, o.Flows, "table=22,priority=100,ip,ct_zone=50030,ct_state=+trk+new,nw_dst=10.0.1.10,actions=ct_clear,ct(commit,table=25,zone=50060,nat(src=10.0.1.1))")
	has(t, o.Flows, "table=21,priority=200,ip,ct_zone=50030,ct_state=+trk+est-rpl,actions=ct_clear,ct(table=25,zone=50060,nat)")
	has(t, o.Flows, "table=20,priority=400,ip,tcp,nw_src=10.0.1.10,tp_src=80,nw_dst=10.0.1.1,ct_state=-trk,actions=ct(table=23,zone=50060,nat)")
	has(t, o.Flows, "table=23,priority=100,ip,ct_zone=50060,actions=ct_clear,ct(table=21,zone=50030,nat)")
	has(t, o.Flows, "nat(src=203.0.113.20)") // FullNAT also provides outbound SNAT
}

func TestNATDNATNeedsForwards(t *testing.T) {
	in := testInput()
	in.NATs[0].Spec.Type = gwv1alpha1.NATTypeDNAT
	o := Build(in)
	if o.Reports["NATGateway/d/nat"].Error == "" {
		t.Error("DNAT without portForwards must report an error")
	}
}

func isolatedInput() Input {
	in := testInput()
	in.Firewalls, in.LBs, in.NATs = nil, nil, nil
	in.InterSubnetDeny = true
	return in
}

func TestInterSubnetIsolationWithoutFirewall(t *testing.T) {
	o := Build(isolatedInput())
	// both VNics get an implicit stateful policy even though no Firewall applies
	has(t, o.Flows, "table=25,priority=100,ip,nw_dst=10.0.1.10,actions=load:2->NXM_NX_REG0[0..15]")
	has(t, o.Flows, "table=25,priority=100,ip,nw_dst=10.0.2.10,actions=load:3->NXM_NX_REG0[0..15],load:1->NXM_NX_REG1[]")
	// own subnet allowed, rest of the VPC dropped, outside the VPC allowed by the default
	has(t, o.Flows, "table=30,priority=3,ct_state=+trk+new,reg1=2,ip,nw_src=10.0.1.0/24,actions=ct(commit")
	has(t, o.Flows, "table=30,priority=3,ct_state=+trk+new,reg1=1,ip,nw_src=10.0.2.0/24,actions=ct(commit")
	has(t, o.Flows, "table=30,priority=2,ct_state=+trk+new,reg1=2,ip,nw_src=10.0.0.0/16,actions=drop")
	has(t, o.Flows, "table=30,priority=2,ct_state=+trk+new,reg1=1,ip,nw_src=10.0.0.0/16,actions=drop")
	has(t, o.Flows, "table=30,priority=0,reg1=1,actions=ct(commit")
	// replies of connections the VNic started stay allowed
	has(t, o.Flows, "table=30,priority=100,ct_state=+trk+est,reg1=1")
	// egress is not restricted
	has(t, o.Flows, "table=10,priority=0,reg1=1,actions=ct(commit")
}

func TestInterSubnetAllowDisablesIsolation(t *testing.T) {
	in := isolatedInput()
	in.InterSubnetDeny = false
	o := Build(in)
	hasNot(t, o.Flows, "priority=2,ct_state=+trk+new,reg1=")
	hasNot(t, o.Flows, "table=25,priority=100,ip,nw_dst=10.0.1.10")
}

func TestFirewallSubnetRefRules(t *testing.T) {
	in := isolatedInput()
	in.Subnets = map[string]string{"web": "10.0.1.0/24", "app": "10.0.2.0/24"}
	in.Firewalls = []servicev1alpha1.Firewall{{
		ObjectMeta: metav1.ObjectMeta{Name: "db-fw", Namespace: "d"},
		Spec: servicev1alpha1.FirewallSpec{
			DefaultAction: servicev1alpha1.FirewallActionDeny, Stateful: true, SubnetRefs: []string{"web"},
			Rules: []servicev1alpha1.FirewallRule{
				{Name: "pg", Priority: 100, Direction: servicev1alpha1.FirewallDirectionIngress, Action: servicev1alpha1.FirewallActionAllow,
					Match: servicev1alpha1.FirewallRuleMatch{Protocol: "TCP", SrcSubnetRef: "app", DstPort: "5432"}},
				{Name: "bad-ref", Priority: 110, Direction: servicev1alpha1.FirewallDirectionEgress, Action: servicev1alpha1.FirewallActionAllow,
					Match: servicev1alpha1.FirewallRuleMatch{Protocol: "TCP", DstSubnetRef: "nope"}},
				{Name: "both", Priority: 120, Direction: servicev1alpha1.FirewallDirectionEgress, Action: servicev1alpha1.FirewallActionAllow,
					Match: servicev1alpha1.FirewallRuleMatch{Protocol: "TCP", DstSubnetRef: "app", DstCIDR: "1.2.3.0/24"}},
			},
		},
	}}
	o := Build(in)
	has(t, o.Flows, "table=30,priority=65436,ct_state=+trk+new,reg1=1,tcp,nw_src=10.0.2.0/24,tp_dst=0x1538,actions=ct(commit")
	w := strings.Join(o.Reports["Firewall/d/db-fw"].Warnings, ";")
	if !strings.Contains(w, `dstSubnetRef "nope" is not a subnet`) || !strings.Contains(w, "mutually exclusive") {
		t.Errorf("invalid subnet refs must be reported: %s", w)
	}
}
