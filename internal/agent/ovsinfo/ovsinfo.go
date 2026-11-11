// Package ovsinfo reads the node's OVS state (bridges, ports, uplinks, OpenFlow
// rules) and returns it in a form suitable for the UI.
package ovsinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PortStats struct {
	RxBytes   int64 `json:"rxBytes"`
	TxBytes   int64 `json:"txBytes"`
	RxPackets int64 `json:"rxPackets"`
	TxPackets int64 `json:"txPackets"`
	RxDropped int64 `json:"rxDropped"`
	TxDropped int64 `json:"txDropped"`
	RxErrors  int64 `json:"rxErrors"`
	TxErrors  int64 `json:"txErrors"`
}

type Port struct {
	Name      string            `json:"name"`
	OFPort    int               `json:"ofport"`
	Type      string            `json:"type"`
	Role      string            `json:"role"` // uplink | tunnel | workload | internal | patch | other
	MAC       string            `json:"mac,omitempty"`
	LinkState string            `json:"linkState,omitempty"`
	MTU       int               `json:"mtu,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
	RemoteIP  string            `json:"remoteIP,omitempty"`
	VNI       string            `json:"vni,omitempty"`
	Peer      string            `json:"peer,omitempty"`
	VNic      string            `json:"vnic,omitempty"`
	Stats     PortStats         `json:"stats"`
}

type Bridge struct {
	Name         string            `json:"name"`
	DatapathType string            `json:"datapathType"`
	ExternalIDs  map[string]string `json:"externalIDs,omitempty"`
	Addresses    []string          `json:"addresses"` // IPs on the bridge's local port (gateway IPs)
	Ports        []Port            `json:"ports"`
	Flows        int               `json:"flows"`
}

type HostIface struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	MTU       int      `json:"mtu"`
	State     string   `json:"state"`
	Addresses []string `json:"addresses"`
	Default   bool     `json:"defaultRoute"`
	Physical  bool     `json:"physical"`
}

type NATRule struct {
	VPC    string `json:"vpc"`
	Source string `json:"source"`
	Uplink string `json:"uplink"`
}

type Topology struct {
	Node       string      `json:"node"`
	Time       time.Time   `json:"time"`
	OVSVersion string      `json:"ovsVersion,omitempty"`
	HWOffload  bool        `json:"hwOffload"`
	Uplinks    []HostIface `json:"uplinks"`
	Bridges    []Bridge    `json:"bridges"`
	NAT        []NATRule   `json:"nat"`
}

type Flow struct {
	Table    int     `json:"table"`
	Priority int     `json:"priority"`
	Cookie   string  `json:"cookie"`
	Match    string  `json:"match"`
	Actions  string  `json:"actions"`
	Packets  int64   `json:"packets"`
	Bytes    int64   `json:"bytes"`
	Duration float64 `json:"duration"`
	IdleAge  int     `json:"idleAge"`
}

func out(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, err := exec.CommandContext(cctx, name, args...).Output()
	return strings.TrimSpace(string(b)), err
}

// unwrap converts OVSDB JSON encodings ("set"/"map"/"uuid") into plain Go values.
func unwrap(v interface{}) interface{} {
	arr, ok := v.([]interface{})
	if !ok || len(arr) != 2 {
		return v
	}
	kind, _ := arr[0].(string)
	switch kind {
	case "map":
		m := map[string]string{}
		pairs, _ := arr[1].([]interface{})
		for _, p := range pairs {
			kv, _ := p.([]interface{})
			if len(kv) == 2 {
				m[fmt.Sprint(unwrap(kv[0]))] = fmt.Sprint(unwrap(kv[1]))
			}
		}
		return m
	case "set":
		items, _ := arr[1].([]interface{})
		res := make([]interface{}, 0, len(items))
		for _, it := range items {
			res = append(res, unwrap(it))
		}
		return res
	case "uuid":
		return fmt.Sprint(arr[1])
	}
	return v
}

type table []map[string]interface{}

func listTable(ctx context.Context, tbl string, cols ...string) (table, error) {
	o, err := out(ctx, "ovs-vsctl", "--format=json", "--columns="+strings.Join(cols, ","), "list", tbl)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Headings []string        `json:"headings"`
		Data     [][]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(o), &raw); err != nil {
		return nil, err
	}
	var t table
	for _, row := range raw.Data {
		m := map[string]interface{}{}
		for i, h := range raw.Headings {
			if i < len(row) {
				m[h] = unwrap(row[i])
			}
		}
		t = append(t, m)
	}
	return t, nil
}

func str(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case []interface{}:
		if len(x) == 0 {
			return ""
		}
		return fmt.Sprint(x[0])
	}
	return fmt.Sprint(v)
}

func num(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case []interface{}:
		if len(x) > 0 {
			return num(x[0])
		}
	}
	return 0
}

func smap(v interface{}) map[string]string {
	if m, ok := v.(map[string]string); ok {
		return m
	}
	return map[string]string{}
}

func isPhysical(name string) bool {
	_, err := os.Stat("/sys/class/net/" + name + "/device")
	return err == nil
}

func stat(m map[string]string, k string) int64 {
	n, _ := strconv.ParseInt(m[k], 10, 64)
	return n
}

// GetTopology collects bridges, ports, host uplinks and NAT rules.
func GetTopology(ctx context.Context, node string) (*Topology, error) {
	t := &Topology{Node: node, Time: time.Now().UTC()}
	if v, err := out(ctx, "ovs-vsctl", "--version"); err == nil {
		t.OVSVersion = strings.TrimSpace(strings.SplitN(v, "\n", 2)[0])
	}
	if v, err := out(ctx, "ovs-vsctl", "--if-exists", "get", "Open_vSwitch", ".", "other_config:hw-offload"); err == nil {
		t.HWOffload = strings.Contains(v, "true")
	}

	brs, err := listTable(ctx, "bridge", "name", "datapath_type", "external_ids")
	if err != nil {
		return nil, fmt.Errorf("listing bridges: %w", err)
	}
	ifs, err := listTable(ctx, "interface", "name", "type", "options", "ofport", "mac_in_use", "link_state", "mtu", "external_ids", "statistics")
	if err != nil {
		return nil, fmt.Errorf("listing interfaces: %w", err)
	}
	byName := map[string]map[string]interface{}{}
	for _, i := range ifs {
		byName[str(i["name"])] = i
	}

	host := hostInterfaces(ctx)
	hostByName := map[string]HostIface{}
	for _, h := range host {
		hostByName[h.Name] = h
	}

	uplinkSeen := map[string]bool{}
	for _, b := range brs {
		br := Bridge{Addresses: []string{}, Name: str(b["name"]), DatapathType: str(b["datapath_type"]), ExternalIDs: smap(b["external_ids"])}
		if br.DatapathType == "" {
			br.DatapathType = "system"
		}
		portsOut, _ := out(ctx, "ovs-vsctl", "list-ports", br.Name)
		names := append([]string{br.Name}, strings.Fields(portsOut)...)
		for _, pn := range names {
			i, ok := byName[pn]
			if !ok {
				continue
			}
			opts := smap(i["options"])
			ext := smap(i["external_ids"])
			st := smap(i["statistics"])
			p := Port{
				Name: pn, OFPort: num(i["ofport"]), Type: str(i["type"]), MAC: str(i["mac_in_use"]),
				LinkState: str(i["link_state"]), MTU: num(i["mtu"]), Options: opts,
				RemoteIP: opts["remote_ip"], VNI: opts["key"], Peer: opts["peer"], VNic: ext["sdn-vnic"],
				Stats: PortStats{
					RxBytes: stat(st, "rx_bytes"), TxBytes: stat(st, "tx_bytes"),
					RxPackets: stat(st, "rx_packets"), TxPackets: stat(st, "tx_packets"),
					RxDropped: stat(st, "rx_dropped"), TxDropped: stat(st, "tx_dropped"),
					RxErrors: stat(st, "rx_errors"), TxErrors: stat(st, "tx_errors"),
				},
			}
			switch {
			case pn == br.Name:
				p.Role = "internal"
				if h, ok := hostByName[pn]; ok {
					br.Addresses = h.Addresses
				}
			case p.Type == "vxlan" || p.Type == "gre" || p.Type == "ipip" || p.Type == "geneve":
				p.Role = "tunnel"
			case p.Type == "patch":
				p.Role = "patch"
			case isPhysical(pn) && ext["sdn-vnic"] == "" && !strings.HasPrefix(pn, "vnet") && !strings.HasPrefix(pn, "tap"):
				p.Role = "uplink"
				uplinkSeen[pn] = true
			case ext["sdn-vnic"] != "" || strings.HasPrefix(pn, "vnet") || strings.HasPrefix(pn, "tap"):
				p.Role = "workload"
			default:
				p.Role = "other"
			}
			br.Ports = append(br.Ports, p)
		}
		if fl, err := GetFlows(ctx, br.Name); err == nil {
			br.Flows = len(fl)
		}
		t.Bridges = append(t.Bridges, br)
	}
	sort.Slice(t.Bridges, func(i, j int) bool { return t.Bridges[i].Name < t.Bridges[j].Name })

	// Underlay uplinks: the NIC that carries tunnel traffic (default route) and any NIC enslaved to OVS.
	for _, h := range host {
		if h.Default || uplinkSeen[h.Name] {
			t.Uplinks = append(t.Uplinks, h)
		}
	}
	t.NAT = natRules(ctx)
	return t, nil
}

func hostInterfaces(ctx context.Context) []HostIface {
	o, err := out(ctx, "ip", "-j", "addr")
	if err != nil {
		return nil
	}
	var raw []struct {
		Name      string `json:"ifname"`
		MAC       string `json:"address"`
		MTU       int    `json:"mtu"`
		OperState string `json:"operstate"`
		AddrInfo  []struct {
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
			Family    string `json:"family"`
		} `json:"addr_info"`
	}
	if json.Unmarshal([]byte(o), &raw) != nil {
		return nil
	}
	def := ""
	if r, err := out(ctx, "ip", "-j", "route", "show", "default"); err == nil {
		var routes []struct {
			Dev string `json:"dev"`
		}
		if json.Unmarshal([]byte(r), &routes) == nil && len(routes) > 0 {
			def = routes[0].Dev
		}
	}
	var res []HostIface
	for _, r := range raw {
		h := HostIface{Addresses: []string{}, Name: r.Name, MAC: r.MAC, MTU: r.MTU, State: strings.ToLower(r.OperState), Default: r.Name == def, Physical: isPhysical(r.Name)}
		for _, a := range r.AddrInfo {
			if a.Family == "inet" {
				h.Addresses = append(h.Addresses, fmt.Sprintf("%s/%d", a.Local, a.PrefixLen))
			}
		}
		res = append(res, h)
	}
	return res
}

var natRe = regexp.MustCompile(`-s (\S+).*-o (\S+).*--comment sdn-igw-(\S+)`)

func natRules(ctx context.Context) []NATRule {
	o, err := out(ctx, "iptables", "-t", "nat", "-S", "POSTROUTING")
	if err != nil {
		return nil
	}
	var res []NATRule
	for _, l := range strings.Split(o, "\n") {
		if m := natRe.FindStringSubmatch(l); m != nil {
			res = append(res, NATRule{Source: m[1], Uplink: m[2], VPC: m[3]})
		}
	}
	return res
}

var flowRe = regexp.MustCompile(`^cookie=(0x[0-9a-fA-F]+), duration=([0-9.]+)s, table=(\d+), n_packets=(\d+), n_bytes=(\d+), (.*?) ?actions=(.*)$`)

// GetFlows returns the OpenFlow rules of a bridge.
func GetFlows(ctx context.Context, bridge string) ([]Flow, error) {
	o, err := out(ctx, "ovs-ofctl", "dump-flows", bridge)
	if err != nil {
		return nil, err
	}
	var flows []Flow
	for _, line := range strings.Split(o, "\n") {
		line = strings.TrimSpace(line)
		m := flowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		f := Flow{Cookie: m[1], Priority: 32768, Actions: m[7]}
		f.Duration, _ = strconv.ParseFloat(m[2], 64)
		f.Table, _ = strconv.Atoi(m[3])
		f.Packets, _ = strconv.ParseInt(m[4], 10, 64)
		f.Bytes, _ = strconv.ParseInt(m[5], 10, 64)
		var match []string
		for _, part := range strings.Split(m[6], ", ") {
			switch {
			case strings.HasPrefix(part, "idle_age="):
				f.IdleAge, _ = strconv.Atoi(strings.TrimPrefix(part, "idle_age="))
			case strings.HasPrefix(part, "hard_age="), strings.HasPrefix(part, "idle_timeout="), strings.HasPrefix(part, "hard_timeout="):
			case part != "":
				match = append(match, part)
			}
		}
		fields := strings.Join(match, ",")
		if strings.HasPrefix(fields, "priority=") {
			rest := strings.SplitN(fields, ",", 2)
			f.Priority, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "priority="))
			fields = ""
			if len(rest) > 1 {
				fields = rest[1]
			}
		}
		f.Match = fields
		if f.Match == "" {
			f.Match = "(any)"
		}
		flows = append(flows, f)
	}
	sort.SliceStable(flows, func(i, j int) bool {
		if flows[i].Table != flows[j].Table {
			return flows[i].Table < flows[j].Table
		}
		return flows[i].Priority > flows[j].Priority
	})
	return flows, nil
}
