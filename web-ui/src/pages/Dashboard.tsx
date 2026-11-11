import { Link } from 'react-router-dom';
import {
  ArrowUpDown, Cable, Plug, Cpu, GitBranch, Globe, Layers, Link2, Network, Router, Server, Settings2, Shield,
  type LucideIcon,
} from 'lucide-react';
import { getTopology, listNodes, listResources, queryMetric, queryRange } from '../api';
import { countByResource, hostedByNode } from '../placement';
import { useData } from '../context';
import { RESOURCES, detailPath } from '../resources';
import { age } from '../lib';
import { ResourceStatus } from '../components/StatusBadge';
import { Card, EntityLink, ErrorBanner, MiniTable, NodeLink, PageHeader } from '../components/ui';
import { MetricChart, Sparkline, FORMATS, toPoints } from '../components/charts';

const TYPES: { type: string; label: string; to: string; icon: LucideIcon; color: string }[] = [
  { type: 'vpcs', label: 'VPCs', to: '/vpcs', icon: Network, color: 'text-blue-600 bg-blue-100 dark:text-blue-400 dark:bg-blue-500/20' },
  { type: 'subnets', label: 'Subnets', to: '/subnets', icon: Layers, color: 'text-purple-600 bg-purple-100 dark:text-purple-400 dark:bg-purple-500/20' },
  { type: 'vnics', label: 'VNICs', to: '/vnics', icon: Plug, color: 'text-fuchsia-600 bg-fuchsia-100 dark:text-fuchsia-400 dark:bg-fuchsia-500/20' },
  { type: 'internetgateways', label: 'Internet GWs', to: '/gateways', icon: Globe, color: 'text-orange-600 bg-orange-100 dark:text-orange-400 dark:bg-orange-500/20' },
  { type: 'natgateways', label: 'NAT GWs', to: '/gateways', icon: Router, color: 'text-amber-600 bg-amber-100 dark:text-amber-400 dark:bg-amber-500/20' },
  { type: 'loadbalancers', label: 'Load Balancers', to: '/services', icon: ArrowUpDown, color: 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-500/20' },
  { type: 'firewalls', label: 'Firewalls', to: '/services', icon: Shield, color: 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-500/20' },
  { type: 'servicechains', label: 'Service Chains', to: '/nfv', icon: Cpu, color: 'text-pink-600 bg-pink-100 dark:text-pink-400 dark:bg-pink-500/20' },
  { type: 'virtualnetworkfunctions', label: 'VNFs', to: '/nfv', icon: Server, color: 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-500/20' },
  { type: 'tunnels', label: 'Tunnels', to: '/infra', icon: GitBranch, color: 'text-teal-600 bg-teal-100 dark:text-teal-400 dark:bg-teal-500/20' },
  { type: 'bonds', label: 'Bonds', to: '/infra', icon: Link2, color: 'text-cyan-600 bg-cyan-100 dark:text-cyan-400 dark:bg-cyan-500/20' },
  { type: 'networkinterfaces', label: 'NICs', to: '/infra', icon: Cable, color: 'text-indigo-600 bg-indigo-100 dark:text-indigo-400 dark:bg-indigo-500/20' },
  { type: 'nodenetworkconfigs', label: 'Node Configs', to: '/infra', icon: Settings2, color: 'text-slate-600 bg-slate-200 dark:text-slate-300 dark:bg-slate-500/20' },
];

const GOOD = ['Ready', 'Active', 'Running', 'Established', 'Configured'];
const BAD = ['Failed', 'Error'];

function classify(i: any): 'healthy' | 'degraded' | 'failed' | 'terminating' | 'pending' {
  if (i.metadata?.deletionTimestamp) return 'terminating';
  const p = i.status?.phase;
  if (GOOD.includes(p)) return 'healthy';
  if (BAD.includes(p)) return 'failed';
  if (p === 'Degraded') return 'degraded';
  return 'pending';
}

export default function Dashboard() {
  const { data, loading, error } = useData(async () => {
    const res = await Promise.allSettled(TYPES.map((t) => listResources(t.type)));
    return TYPES.map((t, i) => ({ ...t, items: res[i].status === 'fulfilled' ? (res[i] as PromiseFulfilledResult<any[]>).value : [] }));
  });
  const placementNodes = useData(() => listNodes().catch(() => []));
  const placementTopo = useData(() => getTopology().catch(() => undefined));
  const hosted = hostedByNode(placementTopo.data);
  const spark = useData(async () => {
    const q = (s: string) => queryRange(s, 1800, 30).then((r) => r.flatMap(toPoints)).catch(() => []);
    const [recon, flows, tx, queue, offloaded] = await Promise.all([
      q('sum(rate(controller_runtime_reconcile_total[1m]))'),
      q('sum(sdn_ovs_bridge_flows)'),
      q('sum(rate(sdn_ovs_port_tx_bytes[1m]))'),
      q('sum(workqueue_depth)'),
      q('sum(sdn_ovs_datapath_flows_offloaded)'),
    ]);
    const hw = await queryMetric('sdn_ovs_hw_offload_enabled').catch(() => []);
    const hwNodes = hw.length;
    const hwOn = hw.filter((s) => Number(s.value?.[1]) === 1).length;
    return { recon, flows, tx, queue, offloaded, hwNodes, hwOn };
  });

  const groups = data ?? [];
  const all = groups.flatMap((g) => g.items.map((i) => ({ ...i, __resource: g.type })));
  const health = { healthy: 0, degraded: 0, failed: 0, terminating: 0, pending: 0 };
  all.forEach((i) => health[classify(i)]++);
  const attention = all.filter((i) => classify(i) !== 'healthy');
  const recent = [...all].sort((a, b) => (b.metadata?.creationTimestamp ?? '').localeCompare(a.metadata?.creationTimestamp ?? '')).slice(0, 8);

  const hwHint = spark.data
    ? spark.data.hwNodes > 0
      ? `HW offload: ${spark.data.hwOn === 0 ? 'off' : 'on'} (${spark.data.hwOn} of ${spark.data.hwNodes} nodes)`
      : 'HW offload: -'
    : undefined;
  const sparkCards: [string, string, any[], string, string?][] = [
    ['Reconciles / s', spark.data ? last(spark.data.recon).toFixed(2) : '-', spark.data?.recon ?? [], '#3b82f6'],
    ['OpenFlow rules', spark.data ? String(Math.round(last(spark.data.flows))) : '-', spark.data?.flows ?? [], '#22c55e'],
    ['Offloaded flows', spark.data ? String(Math.round(last(spark.data.offloaded))) : '-', spark.data?.offloaded ?? [], '#14b8a6', hwHint],
    ['TX throughput', spark.data ? FORMATS.bps(last(spark.data.tx)) : '-', spark.data?.tx ?? [], '#f59e0b'],
    ['Workqueue depth', spark.data ? String(Math.round(last(spark.data.queue))) : '-', spark.data?.queue ?? [], '#a855f7'],
  ];

  return (
    <div className="space-y-6">
      <PageHeader title="Dashboard" subtitle="Overview of your SDN infrastructure" />
      <ErrorBanner message={error} />

      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
        {groups.map((g) => (
          <Link key={g.type} to={g.to} className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm transition-colors hover:border-blue-400 dark:border-gray-700 dark:bg-gray-900 dark:hover:border-blue-500">
            <div className="flex items-center justify-between">
              <div className={`rounded-lg p-2 ${g.color}`}><g.icon className="h-5 w-5" /></div>
              <span className="text-3xl font-bold text-gray-900 dark:text-gray-100">{g.items.length}</span>
            </div>
            <div className="mt-3 text-sm font-medium text-gray-600 dark:text-gray-300">{g.label}</div>
            <div className="text-xs text-gray-500 dark:text-gray-400">
              {g.items.filter((i) => classify(i) === 'healthy').length} healthy
              {g.items.some((i) => classify(i) !== 'healthy') && <span className="text-yellow-600 dark:text-yellow-400"> · {g.items.filter((i) => classify(i) !== 'healthy').length} attention</span>}
            </div>
          </Link>
        ))}
        {loading && groups.length === 0 && <div className="col-span-full text-gray-400">Loading…</div>}
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card title="Health summary" bodyClass="p-5">
          <div className="grid grid-cols-2 gap-3 text-center">
            {([
              ['Healthy', health.healthy, 'text-green-600 dark:text-green-400'],
              ['Degraded', health.degraded, 'text-yellow-600 dark:text-yellow-400'],
              ['Failed', health.failed, 'text-red-600 dark:text-red-400'],
              ['Terminating', health.terminating, 'text-orange-600 dark:text-orange-400'],
            ] as const).map(([l, n, c]) => (
              <div key={l} className="rounded-lg bg-gray-50 py-3 dark:bg-gray-800/60">
                <div className={`text-2xl font-bold ${c}`}>{n}</div>
                <div className="text-xs text-gray-500 dark:text-gray-400">{l}</div>
              </div>
            ))}
          </div>
          <div className="mt-4 flex h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
            {all.length > 0 && ([['healthy', '#22c55e'], ['degraded', '#eab308'], ['pending', '#9ca3af'], ['terminating', '#f97316'], ['failed', '#ef4444']] as const).map(([k, c]) => (
              <div key={k} style={{ width: `${(health[k] / all.length) * 100}%`, background: c }} title={`${k}: ${health[k]}`} />
            ))}
          </div>
          <div className="mt-4 text-sm">
            {attention.length === 0 ? (
              <span className="text-green-600 dark:text-green-400">All {all.length} resources healthy.</span>
            ) : (
              <>
                <div className="mb-1 font-medium text-gray-700 dark:text-gray-300">Needs attention</div>
                <ul className="space-y-1">
                  {attention.slice(0, 6).map((i) => (
                    <li key={`${i.__resource}/${i.metadata.namespace}/${i.metadata.name}`} className="flex items-center justify-between gap-2">
                      <EntityLink resource={i.__resource} ns={i.metadata.namespace} name={i.metadata.name} />
                      <ResourceStatus item={i} showBlockers={false} />
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        </Card>

        <Card title="Live metrics (last 30m)" className="lg:col-span-2" bodyClass="grid grid-cols-1 gap-3 p-4 sm:grid-cols-2">
          {sparkCards.map(([label, val, pts, color, sub]) => (
            <div key={label} className="flex items-center justify-between rounded-lg bg-gray-50 p-3 dark:bg-gray-800/60">
              <div>
                <div className="text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">{label}</div>
                <div className="text-xl font-bold text-gray-900 dark:text-gray-100">{val}</div>
                {sub && <div className="text-[11px] text-gray-500 dark:text-gray-400">{sub}</div>}
              </div>
              <Sparkline points={pts} color={color} width={130} height={36} />
            </div>
          ))}
          <div className="sm:col-span-2 text-right text-xs">
            <Link to="/metrics" className="text-blue-600 hover:underline dark:text-blue-400">All metrics →</Link>
          </div>
        </Card>
      </div>

      <Card title="Placement (services per node)">
        <MiniTable
          rows={placementNodes.data ?? []}
          empty="No nodes"
          cols={[
            { header: 'Node', render: (n) => <NodeLink name={n.name} /> },
            ...([['LB', ['loadbalancers']], ['FW', ['firewalls']], ['GW', ['internetgateways', 'natgateways']], ['VNF', ['virtualnetworkfunctions']], ['Chains', ['servicechains']], ['VNICs', ['vnics']]] as [string, string[]][]).map(([h, rs]) => ({
              header: h,
              render: (n: any) => {
                const c = countByResource(hosted.get(n.name));
                return rs.reduce((a, r) => a + (c[r] ?? 0), 0) || <span className="text-gray-400">0</span>;
              },
            })),
          ]}
        />
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <MetricChart title="Controller reconcile rate by controller (/s)" query="sum by (controller) (rate(controller_runtime_reconcile_total[1m]))" labelKey="controller" rangeSec={1800} />
        <Card title="Recently created">
          <ul className="divide-y divide-gray-100 dark:divide-gray-800">
            {recent.map((i) => (
              <li key={`${i.__resource}/${i.metadata.namespace}/${i.metadata.name}`} className="flex items-center justify-between px-5 py-2.5 text-sm">
                <Link to={detailPath(i.__resource, i.metadata.namespace, i.metadata.name)} className="min-w-0 truncate hover:underline">
                  <span className="text-gray-500 dark:text-gray-400">{RESOURCES[i.__resource].label} / </span>
                  <span className="font-medium">{i.metadata.name}</span>
                </Link>
                <span className="flex items-center gap-3"><span className="text-xs text-gray-400">{age(i.metadata.creationTimestamp)}</span><ResourceStatus item={i} showBlockers={false} /></span>
              </li>
            ))}
            {recent.length === 0 && <li className="px-5 py-6 text-center text-sm text-gray-400">No resources</li>}
          </ul>
        </Card>
      </div>
    </div>
  );
}

function last(pts: [number, number][]): number {
  return pts.length ? pts[pts.length - 1][1] : 0;
}
