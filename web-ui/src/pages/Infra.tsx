import { useState, useEffect, useCallback } from 'react';
import {
  listResources,
  createResource,
  deleteResource,
  listNamespaces,
} from '../api';
import ResourceTable from '../components/ResourceTable';
import type { Column } from '../components/ResourceTable';
import CreateModal from '../components/CreateModal';
import type { FieldDef } from '../components/CreateModal';
import StatusBadge from '../components/StatusBadge';

/* ---------- Tunnels ---------- */

const tunnelColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.type', header: 'Type' },
  { key: 'spec.localNode', header: 'Local Node' },
  { key: 'spec.remoteNode', header: 'Remote Node' },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- Bonds ---------- */

const bondColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.mode', header: 'Mode' },
  {
    key: 'interfaces',
    header: 'Interfaces',
    render: (item: any) => {
      const count = item.spec?.interfaces?.length ?? 0;
      return String(count);
    },
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- Network Interfaces ---------- */

const nicColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.node', header: 'Node' },
  { key: 'spec.interfaceName', header: 'Interface' },
  { key: 'spec.type', header: 'Type' },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- Node Network Configs ---------- */

const nncColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.node', header: 'Node' },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function Infra() {
  const [tunnels, setTunnels] = useState<any[]>([]);
  const [bonds, setBonds] = useState<any[]>([]);
  const [nics, setNics] = useState<any[]>([]);
  const [nncs, setNncs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showTunnelCreate, setShowTunnelCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [t, b, n, nc] = await Promise.all([
        listResources('tunnels'),
        listResources('bonds'),
        listResources('networkinterfaces'),
        listResources('nodenetworkconfigs'),
      ]);
      setTunnels(t);
      setBonds(b);
      setNics(n);
      setNncs(nc);
    } catch {
      setTunnels([]);
      setBonds([]);
      setNics([]);
      setNncs([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    listNamespaces()
      .then(setNamespaces)
      .catch(() => setNamespaces(['default']));
  }, []);

  const tunnelFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'tunnel-01' },
    {
      name: 'namespace',
      label: 'Namespace',
      type: 'select',
      options: namespaces.length > 0 ? namespaces : ['default'],
      required: true,
    },
    {
      name: 'type',
      label: 'Type',
      type: 'select',
      options: ['VXLAN', 'IPIP', 'GRE'],
      required: true,
    },
    { name: 'localNode', label: 'Local Node', type: 'text', required: true, placeholder: 'node-1' },
    { name: 'localIP', label: 'Local IP', type: 'text', required: true, placeholder: '192.168.1.1' },
    { name: 'remoteNode', label: 'Remote Node', type: 'text', required: true, placeholder: 'node-2' },
    { name: 'remoteIP', label: 'Remote IP', type: 'text', required: true, placeholder: '192.168.1.2' },
  ];

  const handleTunnelCreate = async (values: Record<string, any>) => {
    await createResource('tunnels', values.namespace, {
      apiVersion: 'infra.sdncontroller.io/v1alpha1',
      kind: 'Tunnel',
      metadata: { name: values.name, namespace: values.namespace },
      spec: {
        type: values.type,
        localNode: values.localNode,
        localIP: values.localIP,
        remoteNode: values.remoteNode,
        remoteIP: values.remoteIP,
      },
    });
    setShowTunnelCreate(false);
    fetchData();
  };

  const handleTunnelDelete = async (item: any) => {
    if (!confirm(`Delete Tunnel "${item.metadata.name}"?`)) return;
    await deleteResource('tunnels', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Infrastructure</h1>
        <p className="text-sm text-gray-400">
          Tunnels, Bonds, Network Interfaces &amp; Node Configs
        </p>
      </div>

      <ResourceTable
        title="Tunnels"
        columns={tunnelColumns}
        data={tunnels}
        loading={loading}
        onCreate={() => setShowTunnelCreate(true)}
        onDelete={handleTunnelDelete}
      />

      <ResourceTable
        title="Bonds"
        columns={bondColumns}
        data={bonds}
        loading={loading}
      />

      <ResourceTable
        title="Network Interfaces"
        columns={nicColumns}
        data={nics}
        loading={loading}
      />

      <ResourceTable
        title="Node Network Configs"
        columns={nncColumns}
        data={nncs}
        loading={loading}
      />

      <CreateModal
        open={showTunnelCreate}
        onClose={() => setShowTunnelCreate(false)}
        onSave={handleTunnelCreate}
        title="Create Tunnel"
        fields={tunnelFields}
      />
    </div>
  );
}
