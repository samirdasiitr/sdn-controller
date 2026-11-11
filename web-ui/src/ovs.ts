import { queryMetric, type PromSample } from './api';
import { useData } from './context';

/* ---------- Shared loader for per-node OVS rule & offload metrics ---------- */

export interface OvsTableFlow {
  table: string;
  flows: number;
}

export interface OvsBridge {
  node: string;
  bridge: string;
  up?: boolean;
  ports?: number;
  flows?: number;
  pps?: number;
  bps?: number;
  tables: OvsTableFlow[];
}

export interface OvsNic {
  device: string;
  on: boolean;
}

export interface OvsDaemon {
  daemon: string;
  up: boolean;
}

export interface OvsNode {
  node: string;
  bridges: number;
  ports: number;
  rules: number; // sum of OpenFlow rules over bridges
  dpFlows?: number; // datapath flows currently installed
  offloaded?: number;
  software?: number;
  offloadPct?: number; // offloaded / (offloaded + software); undefined when no datapath flows
  hwOffload?: boolean; // OVS other_config:hw-offload
  nics: OvsNic[]; // physical NIC hw-tc-offload per device
  daemons: OvsDaemon[];
  dpByType: { dp: string; flows: number }[]; // dp=ovs|tc
}

export interface OvsData {
  nodes: OvsNode[];
  bridges: OvsBridge[];
}

const val = (s: PromSample): number | undefined => (s.value ? Number(s.value[1]) : undefined);

export async function loadOvs(node?: string): Promise<OvsData> {
  const sel = node ? `{node="${node}"}` : '';
  const q = async (query: string): Promise<PromSample[]> => {
    try {
      return await queryMetric(query);
    } catch {
      return [];
    }
  };
  const [up, ports, flows, tables, dp, off, sw, hw, nic, daemon, dpType, pps, bps] = await Promise.all([
    q(`sdn_ovs_bridge_up${sel}`),
    q(`sdn_ovs_bridge_ports${sel}`),
    q(`sdn_ovs_bridge_flows${sel}`),
    q(`sdn_ovs_table_flows${sel}`),
    q(`sdn_ovs_datapath_flows${sel}`),
    q(`sum by (node) (sdn_ovs_datapath_flows_offloaded${sel})`),
    q(`sum by (node) (sdn_ovs_datapath_flows_software${sel})`),
    q(`sdn_ovs_hw_offload_enabled${sel}`),
    q(`sdn_ovs_nic_hw_tc_offload${sel}`),
    q(`sdn_ovs_daemon_up${sel}`),
    q(`sdn_ovs_datapath_flows_by_type${sel}`),
    q(`sum by (node, bridge) (rate(sdn_ovs_bridge_flow_packets${sel}[1m]))`),
    q(`sum by (node, bridge) (rate(sdn_ovs_bridge_flow_bytes${sel}[1m]))`),
  ]);

  const bkey = (m: Record<string, string>) => `${m.node}/${m.bridge}`;
  const bmap = new Map<string, OvsBridge>();
  const bridge = (m: Record<string, string>): OvsBridge => {
    const k = bkey(m);
    let b = bmap.get(k);
    if (!b) {
      b = { node: m.node, bridge: m.bridge, tables: [] };
      bmap.set(k, b);
    }
    return b;
  };
  for (const s of up) bridge(s.metric).up = val(s) === 1;
  for (const s of ports) bridge(s.metric).ports = val(s);
  for (const s of flows) bridge(s.metric).flows = val(s);
  for (const s of tables) {
    const v = val(s);
    if (v !== undefined) bridge(s.metric).tables.push({ table: s.metric.table, flows: v });
  }
  for (const s of pps) bridge(s.metric).pps = val(s);
  for (const s of bps) bridge(s.metric).bps = val(s);
  for (const b of bmap.values()) b.tables.sort((a, b2) => Number(a.table) - Number(b2.table));

  const nmap = new Map<string, OvsNode>();
  const nodeRow = (name: string): OvsNode => {
    let n = nmap.get(name);
    if (!n) {
      n = { node: name, bridges: 0, ports: 0, rules: 0, nics: [], daemons: [], dpByType: [] };
      nmap.set(name, n);
    }
    return n;
  };
  for (const b of bmap.values()) {
    const n = nodeRow(b.node);
    n.bridges += 1;
    n.ports += b.ports ?? 0;
    n.rules += b.flows ?? 0;
  }
  for (const s of dp) nodeRow(s.metric.node).dpFlows = (nodeRow(s.metric.node).dpFlows ?? 0) + (val(s) ?? 0);
  for (const s of off) nodeRow(s.metric.node).offloaded = val(s) ?? 0;
  for (const s of sw) nodeRow(s.metric.node).software = val(s) ?? 0;
  for (const s of hw) nodeRow(s.metric.node).hwOffload = val(s) === 1;
  for (const s of nic) {
    const v = val(s);
    if (v !== undefined) nodeRow(s.metric.node).nics.push({ device: s.metric.device, on: v === 1 });
  }
  for (const s of daemon) nodeRow(s.metric.node).daemons.push({ daemon: s.metric.daemon, up: val(s) === 1 });
  for (const s of dpType) nodeRow(s.metric.node).dpByType.push({ dp: s.metric.dp, flows: val(s) ?? 0 });
  for (const n of nmap.values()) {
    const total = (n.offloaded ?? 0) + (n.software ?? 0);
    if (total > 0) n.offloadPct = ((n.offloaded ?? 0) / total) * 100;
    n.nics.sort((a, b) => a.device.localeCompare(b.device));
    n.daemons.sort((a, b) => a.daemon.localeCompare(b.daemon));
  }

  const bridges = [...bmap.values()].sort((a, b) => bkey(a as any).localeCompare(bkey(b as any)));
  const nodes = [...nmap.values()].sort((a, b) => a.node.localeCompare(b.node));
  return { nodes, bridges };
}

export function useOvs(node?: string) {
  return useData(() => loadOvs(node), [node]);
}

export const fmtPct = (n?: number): string => (n === undefined ? '-' : `${n.toFixed(0)}%`);
export const fmtOrDash = (n?: number): string | number => (n === undefined ? '-' : Math.round(n * 100) / 100);
