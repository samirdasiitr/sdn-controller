import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ArrowDown, ArrowRight } from 'lucide-react';
import { getTopology, listNodes, queryMetric } from '../api';
import { useData } from '../context';
import { Card, ErrorBanner, PageHeader } from '../components/ui';
import LogicalView from '../components/topology/LogicalView';
import PhysicalView, { TunnelTable, type Bridge, type NodeStats } from '../components/topology/PhysicalView';
import { EDGE_COLORS, KIND_STYLE } from '../components/topology/kinds';
import { phaseDot } from '../components/StatusBadge';

const LEGEND_KINDS = ['vpcs', 'subnets', 'internetgateways', 'natgateways', 'vnics', 'loadbalancers', 'firewalls', 'servicechains', 'virtualnetworkfunctions'];
const EDGE_LEGEND: [string, string][] = [
  ['belongs_to', 'in VPC'], ['attached_to', 'attached to subnet'], ['uses_vnf', 'chain hop → VNF'], ['tunnel_endpoint', 'tunnel'],
];

function Legend({ physical }: { physical: boolean }) {
  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-gray-200 px-4 py-2 text-xs text-gray-600 dark:border-gray-700 dark:text-gray-400">
      <span className="font-semibold text-gray-800 dark:text-gray-200">Legend</span>
      {!physical && LEGEND_KINDS.map((k) => {
        const s = KIND_STYLE[k];
        return <span key={k} className="flex items-center gap-1"><span className="h-3 w-1 rounded" style={{ background: s.color }} /><s.icon className="h-3.5 w-3.5" style={{ color: s.color }} />{s.label}</span>;
      })}
      {!physical && EDGE_LEGEND.slice(0, 3).map(([k, l]) => (
        <span key={k} className="flex items-center gap-1"><span className="h-0.5 w-5" style={{ background: EDGE_COLORS[k] }} />{l}</span>
      ))}
      {physical && <span className="flex items-center gap-1"><span className="h-0.5 w-5" style={{ background: EDGE_COLORS.tunnel_endpoint }} />Tunnel (yellow = not all established)</span>}
      <span className="ml-auto flex items-center gap-3">
        {[['Ready', 'Ready/Active/Up'], ['Pending', 'Pending/Degraded'], ['Failed', 'Failed/Down'], ['Terminating', 'Terminating']].map(([k, l]) => (
          <span key={k} className="flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-full" style={{ background: phaseDot[k] }} />{l}</span>
        ))}
      </span>
    </div>
  );
}

export default function Topology() {
  const [params, setParams] = useSearchParams();
  const view = params.get('view') === 'physical' ? 'physical' : 'logical';
  const selected = params.get('node');
  const [dir, setDir] = useState<'TB' | 'LR'>('TB');

  const topo = useData(getTopology);
  const nodes = useData(() => listNodes().catch(() => []));
  const bridges = useData(async (): Promise<Bridge[]> => {
    try {
      const [up, ports, flows] = await Promise.all([
        queryMetric('sdn_ovs_bridge_up'), queryMetric('sdn_ovs_bridge_ports'), queryMetric('sdn_ovs_bridge_flows'),
      ]);
      const key = (m: Record<string, string>) => `${m.node}/${m.bridge}`;
      const map = new Map<string, Bridge>();
      for (const s of up) map.set(key(s.metric), { node: s.metric.node, bridge: s.metric.bridge, up: Number(s.value?.[1]) === 1 });
      for (const s of ports) { const r = map.get(key(s.metric)); if (r) r.ports = Number(s.value?.[1]); }
      for (const s of flows) { const r = map.get(key(s.metric)); if (r) r.flows = Number(s.value?.[1]); }
      return [...map.values()];
    } catch {
      return [];
    }
  });
  const nodeStats = useData(async (): Promise<Map<string, NodeStats>> => {
    const m = new Map<string, NodeStats>();
    try {
      const [flows, off] = await Promise.all([
        queryMetric('sum by (node) (sdn_ovs_bridge_flows)'),
        queryMetric('sdn_ovs_datapath_flows_offloaded'),
      ]);
      const get = (n: string) => m.get(n) ?? m.set(n, { rules: 0, offloaded: 0 }).get(n)!;
      for (const s of flows) get(s.metric.node).rules = Number(s.value?.[1] ?? 0);
      for (const s of off) get(s.metric.node).offloaded = Number(s.value?.[1] ?? 0);
    } catch {
      /* no metrics */
    }
    return m;
  });

  const tab = (v: 'logical' | 'physical', label: string) => (
    <button
      onClick={() => setParams(v === 'logical' ? {} : { view: v })}
      className={`px-4 py-2 text-sm font-medium border-b-2 ${view === v ? 'border-blue-500 text-blue-600 dark:text-blue-400' : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'}`}
    >
      {label}
    </button>
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Topology"
        subtitle={view === 'logical' ? 'VPC → Subnet → gateways, load balancers, firewalls, service chains → VNFs' : 'Kubernetes nodes, OVS bridges, VPC attachments and tunnels'}
        actions={
          view === 'logical' && (
            <div className="flex overflow-hidden rounded-lg border border-gray-300 dark:border-gray-600">
              {(['TB', 'LR'] as const).map((d) => (
                <button key={d} onClick={() => setDir(d)} title={d === 'TB' ? 'Top to bottom' : 'Left to right'}
                  className={`flex items-center gap-1 px-3 py-2 text-sm ${dir === d ? 'bg-blue-600 text-white' : 'bg-white text-gray-700 hover:bg-gray-100 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'}`}>
                  {d === 'TB' ? <ArrowDown className="h-4 w-4" /> : <ArrowRight className="h-4 w-4" />}
                  {d === 'TB' ? 'Vertical' : 'Horizontal'}
                </button>
              ))}
            </div>
          )
        }
      />
      <ErrorBanner message={topo.error} />
      <div className="mb-0 flex border-b border-gray-200 dark:border-gray-700">
        {tab('logical', 'Logical')}
        {tab('physical', 'Physical')}
      </div>
      <div className="flex min-h-[560px] flex-1 flex-col overflow-hidden rounded-b-xl border border-t-0 border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900">
        <div className="relative flex-1">
          {!topo.data ? (
            <div className="flex h-full items-center justify-center text-gray-400">Loading topology…</div>
          ) : view === 'logical' ? (
            <LogicalView topo={topo.data} dir={dir} />
          ) : (
            <PhysicalView topo={topo.data} k8s={nodes.data ?? []} bridges={bridges.data ?? []} stats={nodeStats.data} selected={selected} />
          )}
        </div>
        <Legend physical={view === 'physical'} />
      </div>
      {view === 'physical' && topo.data && (
        <Card title="Tunnels" className="mt-6">
          <TunnelTable topo={topo.data} />
        </Card>
      )}
    </div>
  );
}
