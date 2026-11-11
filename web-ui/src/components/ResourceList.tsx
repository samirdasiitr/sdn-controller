import { useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { deleteResource, friendlyError, listResources } from '../api';
import { useData, useRefresh } from '../context';
import { RESOURCES, detailPath } from '../resources';
import { age, fmtNum } from '../lib';
import { hasTag, parseTagParam, tagStr, type Tag } from '../tags';
import ResourceTable, { type Column } from './ResourceTable';
import ResourceModal from './ResourceModal';
import ConfirmDialog from './ConfirmDialog';
import { ResourceStatus } from './StatusBadge';
import { TagChips, TagFilterBar } from './tags';
import { EntityLink, EntityLinks, ErrorBanner, NodeLink, RunsOn } from './ui';

const nameCol: Column = {
  key: 'metadata.name',
  header: 'Name',
  render: (i) => <EntityLink resource={i.__resource} ns={i.metadata.namespace} name={i.metadata.name} />,
};
const nsCol: Column = { key: 'metadata.namespace', header: 'Namespace' };
const statusCol: Column = { key: 'status.phase', header: 'Status', render: (i) => <ResourceStatus item={i} /> };
const ageCol: Column = { key: 'age', header: 'Age', render: (i) => age(i.metadata?.creationTimestamp) };
const vpcCol: Column = {
  key: 'spec.vpcRef',
  header: 'VPC',
  render: (i) => <EntityLink resource="vpcs" ns={i.metadata.namespace} name={i.spec?.vpcRef} />,
};
const subnetCol = (key = 'subnetRef', header = 'Subnet'): Column => ({
  key: `spec.${key}`,
  header,
  render: (i) => <EntityLink resource="subnets" ns={i.metadata.namespace} name={i.spec?.[key]} />,
});
const subnetsCol: Column = {
  key: 'spec.subnetRefs',
  header: 'Subnets',
  render: (i) => <EntityLinks resource="subnets" ns={i.metadata.namespace} names={i.spec?.subnetRefs} />,
};
const runsOnCol = (resource: string): Column => ({
  key: 'status.nodes',
  header: 'Runs on',
  render: (i) => <RunsOn resource={resource} item={i} />,
});

export const COLUMNS: Record<string, Column[]> = {
  vpcs: [
    nameCol, nsCol,
    { key: 'spec.cidr', header: 'CIDR' },
    { key: 'spec.tunnelType', header: 'Tunnel', render: (i) => i.spec?.tunnelType ?? '-' },
    { key: 'spec.vni', header: 'VNI', render: (i) => i.spec?.vni ?? i.status?.allocatedVNI ?? '-' },
    { key: 'spec.mtu', header: 'MTU' },
    { key: 'status.activeNodes', header: 'Nodes' },
    statusCol, ageCol,
  ],
  subnets: [
    nameCol, nsCol, vpcCol,
    { key: 'spec.cidr', header: 'CIDR' },
    { key: 'spec.gateway', header: 'Gateway' },
    { key: 'spec.type', header: 'Type' },
    {
      key: 'usage', header: 'IPs (used/total)',
      render: (i) => {
        const a = i.status?.allocatedIPs ?? 0, v = i.status?.availableIPs;
        return v === undefined ? '-' : `${a} / ${a + v}`;
      },
    },
    statusCol,
  ],
  internetgateways: [
    nameCol, nsCol, vpcCol,
    { key: 'spec.externalInterface', header: 'Interface' },
    { key: 'spec.externalIPs', header: 'External IPs', render: (i) => (i.spec?.externalIPs ?? []).join(', ') || '-' },
    runsOnCol('internetgateways'), statusCol,
  ],
  natgateways: [
    nameCol, nsCol, vpcCol, subnetsCol,
    { key: 'spec.type', header: 'Type', render: (i) => i.spec?.type ?? 'SNAT' },
    { key: 'spec.externalIP', header: 'Ext IP' },
    { key: 'forwards', header: 'Forwards', render: (i) => (['DNAT', 'FullNAT'].includes(i.spec?.type) ? i.spec?.portForwards?.length ?? 0 : '-') },
    { key: 'status.activeNode', header: 'Active node', render: (i) => <NodeLink name={i.status?.activeNode} /> },
    { key: 'status.translationCount', header: 'Translations (active / total)', render: (i) => `${fmtNum(i.status?.translationCount ?? 0)} / ${fmtNum(i.status?.totalTranslations ?? 0)}` },
    runsOnCol('natgateways'), statusCol,
  ],
  loadbalancers: [
    nameCol, nsCol, vpcCol,
    { key: 'vip', header: 'VIP', render: (i) => `${i.spec?.frontend?.ip ?? '-'}${i.spec?.frontend?.ports?.length ? ':' + i.spec.frontend.ports.map((p: any) => p.port).join(',') : ''}` },
    { key: 'spec.algorithm', header: 'Algorithm' },
    {
      key: 'backends', header: 'Backends healthy/total',
      render: (i) => {
        const sb: any[] | undefined = i.status?.backends;
        if (sb?.length) return `${sb.filter((b) => b.healthy).length} / ${sb.length}`;
        return `? / ${i.spec?.backends?.length ?? 0}`;
      },
    },
    { key: 'status.activeConnections', header: 'Connections (active / total)', render: (i) => `${fmtNum(i.status?.activeConnections ?? 0)} / ${fmtNum(i.status?.totalConnections ?? 0)}` },
    runsOnCol('loadbalancers'), statusCol,
  ],
  firewalls: [
    nameCol, nsCol, vpcCol, subnetsCol,
    { key: 'spec.defaultAction', header: 'Default' },
    { key: 'rules', header: 'Rules', render: (i) => i.spec?.rules?.length ?? 0 },
    { key: 'spec.stateful', header: 'Stateful', render: (i) => (i.spec?.stateful ? 'yes' : 'no') },
    runsOnCol('firewalls'), statusCol,
  ],
  servicechains: [
    nameCol, nsCol, vpcCol, subnetCol(),
    { key: 'spec.direction', header: 'Direction' },
    {
      key: 'hops', header: 'Hops',
      render: (i) => (
        <span className="flex flex-wrap items-center gap-1">
          {(i.spec?.chain ?? []).map((h: any, n: number) => (
            <span key={n} className="flex items-center gap-1">
              {n > 0 && <span className="text-gray-400">→</span>}
              <EntityLink resource="virtualnetworkfunctions" ns={i.metadata.namespace} name={h.vnfRef} />
            </span>
          ))}
        </span>
      ),
    },
    runsOnCol('servicechains'), statusCol,
  ],
  virtualnetworkfunctions: [
    nameCol, nsCol,
    { key: 'spec.type', header: 'Type' },
    { key: 'spec.image', header: 'Image', render: (i) => i.spec?.image ?? i.spec?.vmConfig?.image ?? '-' },
    { key: 'replicas', header: 'Ready/Replicas', render: (i) => `${i.status?.readyReplicas ?? 0} / ${i.spec?.replicas ?? 1}` },
    runsOnCol('virtualnetworkfunctions'), statusCol,
  ],
  vnics: [
    nameCol, nsCol, subnetCol(),
    { key: 'status.vpcRef', header: 'VPC', render: (i) => <EntityLink resource="vpcs" ns={i.metadata.namespace} name={i.status?.vpcRef} /> },
    { key: 'status.ipAddress', header: 'IP', render: (i) => i.status?.ipAddress ?? i.spec?.ipAddress ?? '-' },
    { key: 'status.macAddress', header: 'MAC', render: (i) => <span className="font-mono text-xs">{i.status?.macAddress ?? i.spec?.macAddress ?? '-'}</span> },
    { key: 'spec.attachment', header: 'Workload', render: (i) => (i.spec?.attachment ? `${i.spec.attachment.kind}/${i.spec.attachment.name}` : '-') },
    { key: 'status.boundNode', header: 'Node', render: (i) => <NodeLink name={i.status?.boundNode || i.spec?.nodeName} /> },
    { key: 'status.ovsPort', header: 'Port', render: (i) => (i.status?.ovsPort ? `${i.status.bridge ?? '?'}/${i.status.ovsPort}` : '-') },
    { key: 'spec.portSecurity', header: 'Port sec.', render: (i) => (i.spec?.portSecurity === false ? 'off' : 'on') },
    statusCol,
  ],
  tunnels: [
    nameCol, nsCol,
    { key: 'spec.type', header: 'Type' },
    { key: 'spec.vni', header: 'VNI' },
    { key: 'local', header: 'Local', render: (i) => <span><NodeLink name={i.spec?.localEndpoint?.nodeName} /> <span className="text-gray-400">{i.spec?.localEndpoint?.ip}</span></span> },
    { key: 'remote', header: 'Remote', render: (i) => <span><NodeLink name={i.spec?.remoteEndpoint?.nodeName} /> <span className="text-gray-400">{i.spec?.remoteEndpoint?.ip}</span></span> },
    statusCol,
  ],
  bonds: [
    nameCol, nsCol,
    { key: 'spec.nodeName', header: 'Node', render: (i) => <NodeLink name={i.spec?.nodeName} /> },
    { key: 'spec.mode', header: 'Mode' },
    { key: 'spec.interfaces', header: 'Interfaces', render: (i) => (i.spec?.interfaces ?? []).join(', ') },
    subnetCol(),
    statusCol,
  ],
  networkinterfaces: [
    nameCol, nsCol,
    { key: 'spec.nodeName', header: 'Node', render: (i) => <NodeLink name={i.spec?.nodeName} /> },
    { key: 'spec.device', header: 'Device' },
    { key: 'spec.type', header: 'Type' },
    vpcCol, subnetCol(),
    statusCol,
  ],
  nodenetworkconfigs: [
    nameCol, nsCol,
    { key: 'spec.nodeName', header: 'Node', render: (i) => <NodeLink name={i.spec?.nodeName} /> },
    { key: 'spec.nodeType', header: 'Node Type' },
    { key: 'spec.tunnelProtocol', header: 'Tunnel' },
    { key: 'bridges', header: 'Bridges', render: (i) => (i.spec?.ovsBridges ?? []).map((b: any) => b.name).join(', ') || '-' },
    statusCol,
  ],
};

export default function ResourceList({ resource, title }: { resource: string; title?: string }) {
  const meta = RESOURCES[resource];
  const navigate = useNavigate();
  const { refresh } = useRefresh();
  const { data, loading, error } = useData(() => listResources(resource), [resource]);
  const [modal, setModal] = useState<{ open: boolean; target: { namespace: string; name: string } | null }>({ open: false, target: null });
  const [del, setDel] = useState<any>(null);
  const [delError, setDelError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [params, setParams] = useSearchParams();
  const tagFilters: Tag[] = useMemo(
    () => params.getAll('tag').map(parseTagParam).filter((t): t is Tag => !!t),
    [params],
  );
  const setTagFilters = (tags: Tag[]) => {
    const next = new URLSearchParams(params);
    next.delete('tag');
    for (const t of tags) next.append('tag', tagStr(t));
    setParams(next, { replace: true });
  };
  const addTagFilter = (t: Tag) => {
    if (!tagFilters.some((x) => x.key === t.key && x.value === t.value)) setTagFilters([...tagFilters, t]);
  };

  const tagsCol: Column = useMemo(() => ({
    key: 'tags',
    header: 'Tags',
    render: (i) => <TagChips item={i} onClickTag={addTagFilter} />,
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [tagFilters]);

  const columns = useMemo(() => {
    const cols = [...COLUMNS[resource]];
    // insert Tags before the Status column (last or near-last)
    const idx = cols.findIndex((c) => c.header === 'Status');
    cols.splice(idx >= 0 ? idx : cols.length, 0, tagsCol);
    return cols;
  }, [resource, tagsCol]);

  const rows = (data ?? [])
    .map((d) => ({ ...d, __resource: resource }))
    .filter((d) => tagFilters.every((t) => hasTag(d, t)));

  const doDelete = async () => {
    setBusy(true);
    setDelError(null);
    try {
      await deleteResource(resource, del.metadata.namespace, del.metadata.name);
      setDel(null);
      refresh();
    } catch (e) {
      setDelError(friendlyError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <ErrorBanner message={error} />
      <div className="mb-3 rounded-xl border border-gray-200 bg-white px-4 py-2.5 dark:border-gray-700 dark:bg-gray-900">
        <TagFilterBar selected={tagFilters} onChange={setTagFilters} />
      </div>
      <ResourceTable
        title={title ?? `${meta.label}s`}
        columns={columns}
        data={rows}
        loading={loading}
        onCreate={() => setModal({ open: true, target: null })}
        onEdit={(i) => setModal({ open: true, target: { namespace: i.metadata.namespace, name: i.metadata.name } })}
        onDelete={(i) => { setDelError(null); setDel(i); }}
        onClick={(i) => navigate(detailPath(resource, i.metadata.namespace, i.metadata.name))}
      />
      <ResourceModal
        resource={resource}
        open={modal.open}
        target={modal.target}
        onClose={() => setModal({ open: false, target: null })}
        onSaved={refresh}
      />
      <ConfirmDialog
        open={!!del}
        title={`Delete ${meta.label}`}
        message={<>Delete <b>{del?.metadata?.name}</b> in namespace <b>{del?.metadata?.namespace}</b>? Deletion is blocked while other resources depend on it.</>}
        error={delError}
        busy={busy}
        onConfirm={doDelete}
        onCancel={() => setDel(null)}
      />
    </>
  );
}
