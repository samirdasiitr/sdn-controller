import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { deleteResource, friendlyError, listResources } from '../api';
import { useData, useRefresh } from '../context';
import { RESOURCES, detailPath } from '../resources';
import { age } from '../lib';
import ResourceTable, { type Column } from './ResourceTable';
import ResourceModal from './ResourceModal';
import ConfirmDialog from './ConfirmDialog';
import { ResourceStatus } from './StatusBadge';
import { EntityLink, EntityLinks, ErrorBanner, NodeLink } from './ui';

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
const activeNodeCol: Column = {
  key: 'status.activeNode',
  header: 'Active Node',
  render: (i) => <NodeLink name={i.status?.activeNode} />,
};

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
    activeNodeCol, statusCol,
  ],
  natgateways: [
    nameCol, nsCol, vpcCol, subnetsCol,
    { key: 'spec.externalIP', header: 'External IP' },
    { key: 'spec.type', header: 'Type' },
    activeNodeCol, statusCol,
  ],
  loadbalancers: [
    nameCol, nsCol, vpcCol,
    { key: 'vip', header: 'VIP', render: (i) => `${i.spec?.frontend?.ip ?? '-'}${i.spec?.frontend?.ports?.length ? ':' + i.spec.frontend.ports.map((p: any) => p.port).join(',') : ''}` },
    { key: 'spec.algorithm', header: 'Algorithm' },
    { key: 'backends', header: 'Backends', render: (i) => `${i.status?.activeBackends ?? '?'} / ${i.spec?.backends?.length ?? 0}` },
    statusCol,
  ],
  firewalls: [
    nameCol, nsCol, vpcCol, subnetsCol,
    { key: 'spec.defaultAction', header: 'Default' },
    { key: 'rules', header: 'Rules', render: (i) => i.spec?.rules?.length ?? 0 },
    { key: 'spec.stateful', header: 'Stateful', render: (i) => (i.spec?.stateful ? 'yes' : 'no') },
    statusCol,
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
    statusCol,
  ],
  virtualnetworkfunctions: [
    nameCol, nsCol,
    { key: 'spec.type', header: 'Type' },
    { key: 'spec.image', header: 'Image', render: (i) => i.spec?.image ?? i.spec?.vmConfig?.image ?? '-' },
    { key: 'replicas', header: 'Ready/Replicas', render: (i) => `${i.status?.readyReplicas ?? 0} / ${i.spec?.replicas ?? 1}` },
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

  const rows = (data ?? []).map((d) => ({ ...d, __resource: resource }));

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
      <ResourceTable
        title={title ?? `${meta.label}s`}
        columns={COLUMNS[resource]}
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
