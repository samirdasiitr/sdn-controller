import { Link, useParams } from 'react-router-dom';
import { Server } from 'lucide-react';
import { getNode, queryMetric } from '../api';
import { useData } from '../context';
import { RESOURCES, detailPath } from '../resources';
import { age } from '../lib';
import { Badge, Breadcrumb, Card, ErrorBanner, MiniTable, PageHeader, Props, linkCls } from '../components/ui';

async function loadBridges(node: string) {
  try {
    const sel = `{node="${node}"}`;
    const [up, ports, flows] = await Promise.all([
      queryMetric(`sdn_ovs_bridge_up${sel}`), queryMetric(`sdn_ovs_bridge_ports${sel}`), queryMetric(`sdn_ovs_bridge_flows${sel}`),
    ]);
    const m = new Map<string, { bridge: string; up: boolean; ports?: number; flows?: number }>();
    for (const s of up) m.set(s.metric.bridge, { bridge: s.metric.bridge, up: Number(s.value?.[1]) === 1 });
    for (const s of ports) { const r = m.get(s.metric.bridge); if (r) r.ports = Number(s.value?.[1]); }
    for (const s of flows) { const r = m.get(s.metric.bridge); if (r) r.flows = Number(s.value?.[1]); }
    return [...m.values()];
  } catch {
    return [];
  }
}

export default function NodeDetail() {
  const { name = '' } = useParams();
  const { data: node, error, loading } = useData(() => getNode(name), [name]);
  const { data: bridges } = useData(() => loadBridges(name), [name]);
  const crumbs = [{ label: 'Dashboard', to: '/' }, { label: 'Nodes', to: '/nodes' }, { label: name }];

  if (loading && !node) return <div><Breadcrumb items={crumbs} /><div className="text-gray-400">Loading…</div></div>;
  if (!node) {
    return (
      <div>
        <Breadcrumb items={crumbs} />
        <ErrorBanner message={error} />
        <Link to="/nodes" className={linkCls}>← Back to Nodes</Link>
      </div>
    );
  }

  const groups = new Map<string, typeof node.hosted>();
  for (const h of node.hosted ?? []) groups.set(h.resource, [...(groups.get(h.resource) ?? []), h]);
  const labels = Object.entries(node.labels ?? {});

  return (
    <div className="space-y-6">
      <Breadcrumb items={crumbs} />
      <PageHeader
        title={<span className="flex items-center gap-3"><Server className="h-6 w-6 text-sky-500" />{node.name} <Badge tone={node.ready ? 'green' : 'yellow'}>{node.ready ? 'Ready' : 'NotReady'}</Badge></span>}
        subtitle="Kubernetes node"
        actions={<Link to={`/topology?view=physical&node=${encodeURIComponent(name)}`} className={linkCls}>Show in physical topology →</Link>}
      />
      <ErrorBanner message={error} />
      <Card title="Node info">
        <Props items={[
          ['Internal IP', node.internalIP],
          ['Kubelet', node.kubeletVersion],
          ['Ready reason', node.readyReason],
          ['Created', node.creationTimestamp && `${age(node.creationTimestamp)} ago`],
          ['Hosted resources', node.hosted?.length ?? 0],
        ]} />
      </Card>
      <Card title={`Labels (${labels.length})`}>
        {labels.length === 0 ? <div className="px-5 py-6 text-center text-sm text-gray-400">No labels</div> : (
          <div className="flex flex-wrap gap-1 p-5">
            {labels.map(([k, v]) => <span key={k} className="break-all rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">{k}={v}</span>)}
          </div>
        )}
      </Card>
      <Card title={`Hosted resources (${node.hosted?.length ?? 0})`}>
        {groups.size === 0 ? <div className="px-5 py-6 text-center text-sm text-gray-400">No SDN resources are placed on this node</div> : (
          <div className="divide-y divide-gray-100 dark:divide-gray-800">
            {[...groups.entries()].map(([res, list]) => (
              <div key={res} className="px-5 py-3">
                <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{RESOURCES[res]?.label ?? res} ({list.length})</div>
                <ul className="flex flex-wrap gap-2">
                  {list.map((h) => (
                    <li key={h.namespace + h.name} className="flex items-center gap-1 rounded-full border border-gray-200 px-3 py-1 text-sm dark:border-gray-700">
                      <Link to={detailPath(res, h.namespace, h.name)} className={linkCls}>{h.name}</Link>
                      <span className="text-xs text-gray-400">{h.namespace} · {h.role}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </Card>
      <Card title="OVS bridges (Prometheus)">
        <MiniTable rows={bridges ?? []} empty="No OVS bridge metrics available for this node" cols={[
          { header: 'Bridge', render: (b) => <span className="font-mono">{b.bridge}</span> },
          { header: 'State', render: (b) => <Badge tone={b.up ? 'green' : 'yellow'}>{b.up ? 'up' : 'down'}</Badge> },
          { header: 'Ports', render: (b) => b.ports ?? '-' },
          { header: 'Flows', render: (b) => b.flows ?? '-' },
        ]} />
      </Card>
    </div>
  );
}
