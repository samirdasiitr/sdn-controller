import { useState, useEffect, useCallback } from 'react';
import { listResources, createResource, deleteResource, listNamespaces } from '../api';
import ResourceTable from '../components/ResourceTable';
import type { Column } from '../components/ResourceTable';
import CreateModal from '../components/CreateModal';
import type { FieldDef } from '../components/CreateModal';
import StatusBadge from '../components/StatusBadge';

const columns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.cidr', header: 'CIDR' },
  { key: 'spec.mtu', header: 'MTU' },
  {
    key: 'status.allocatedVNI',
    header: 'VNI',
    render: (item: any) => item.status?.allocatedVNI ?? '-',
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function VPCs() {
  const [data, setData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const items = await listResources('vpcs');
      setData(items);
    } catch {
      setData([]);
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

  const fields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-vpc' },
    {
      name: 'namespace',
      label: 'Namespace',
      type: 'select',
      options: namespaces.length > 0 ? namespaces : ['default'],
      required: true,
    },
    { name: 'cidr', label: 'CIDR', type: 'text', required: true, placeholder: '10.0.0.0/16' },
    { name: 'mtu', label: 'MTU', type: 'number', defaultValue: 9000 },
  ];

  const handleCreate = async (values: Record<string, any>) => {
    await createResource('vpcs', values.namespace, {
      apiVersion: 'vpc.sdncontroller.io/v1alpha1',
      kind: 'VPC',
      metadata: { name: values.name, namespace: values.namespace },
      spec: { cidr: values.cidr, mtu: Number(values.mtu) },
    });
    setShowCreate(false);
    fetchData();
  };

  const handleDelete = async (item: any) => {
    if (!confirm(`Delete VPC "${item.metadata.name}"?`)) return;
    await deleteResource('vpcs', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">VPCs</h1>
        <p className="text-sm text-gray-400">Virtual Private Cloud management</p>
      </div>

      <ResourceTable
        title="VPCs"
        columns={columns}
        data={data}
        loading={loading}
        onCreate={() => setShowCreate(true)}
        onDelete={handleDelete}
      />

      <CreateModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onSave={handleCreate}
        title="Create VPC"
        fields={fields}
      />
    </div>
  );
}
