import { Link, useParams } from 'react-router-dom';
import { Server } from 'lucide-react';
import { getNode } from '../api';
import { useData } from '../context';
import { RESOURCES, detailPath } from '../resources';
import { age, fmtBytes, fmtNum } from '../lib';
import { fmtOrDash, fmtPct, useOvs } from '../ovs';
import { Badge, Breadcrumb, Card, ErrorBanner, MiniTable, PageHeader, Props, linkCls } from '../components/ui';
import { DaemonBadges, HwOffloadBadge, NicOffloadBadges, TableChips } from '../components/ovsui';

export default function NodeDetail() {
  const { name = '' } = useParams();
  const { data: node, error, loading } = useData(() => getNode(name), [name]);
  const { data: ovs } = useOvs(name);
  const ovsNode = ovs?.nodes.find((n) => n.node === name);
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
      <Card title="OVS datapath & offload (Prometheus)">
        <Props items={[
          ['OpenFlow rules', ovsNode && fmtNum(ovsNode.rules)],
          ['Datapath flows', ovsNode && fmtOrDash(ovsNode.dpFlows)],
          ['Offloaded flows', ovsNode && fmtOrDash(ovsNode.offloaded)],
          ['Software flows', ovsNode && fmtOrDash(ovsNode.software)],
          ['Offload %', ovsNode && fmtPct(ovsNode.offloadPct)],
          ['HW offload', ovsNode && <HwOffloadBadge on={ovsNode.hwOffload} />],
          ['NIC hw-tc-offload', ovsNode && <NicOffloadBadges nics={ovsNode.nics} />],
          ['OVS daemons', ovsNode && <DaemonBadges daemons={ovsNode.daemons} />],
          ['Datapath flows by type', ovsNode && (ovsNode.dpByType.length === 0 ? undefined : (
            <span className="flex flex-wrap gap-1">
              {ovsNode.dpByType.map((d) => <Badge key={d.dp} tone="blue">{d.dp}: {d.flows}</Badge>)}
            </span>
          ))],
        ]} />
      </Card>
      <Card title="OVS bridges (Prometheus)">
        <MiniTable rows={ovs?.bridges ?? []} empty="No OVS bridge metrics available for this node" cols={[
          { header: 'Bridge', render: (b) => <span className="font-mono">{b.bridge}</span> },
          { header: 'State', render: (b) => (b.up === undefined ? '-' : <Badge tone={b.up ? 'green' : 'yellow'}>{b.up ? 'up' : 'down'}</Badge>) },
          { header: 'Ports', render: (b) => b.ports ?? '-' },
          { header: 'OpenFlow rules', render: (b) => b.flows ?? '-' },
          { header: 'Rule pps', render: (b) => (b.pps === undefined ? '-' : `${fmtNum(b.pps)}/s`) },
          { header: 'Rule Bps', render: (b) => (b.bps === undefined ? '-' : `${fmtBytes(b.bps)}/s`) },
          { header: 'Rules per table', render: (b) => <TableChips bridge={b} /> },
        ]} />
      </Card>
    </div>
  );
}
