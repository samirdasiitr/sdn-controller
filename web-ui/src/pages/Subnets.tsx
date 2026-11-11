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

const columns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  { key: 'spec.cidr', header: 'CIDR' },
  { key: 'spec.type', header: 'Type' },
  {
    key: 'status.gateway',
    header: 'Gateway',
    render: (item: any) => item.status?.gateway ?? '-',
  },
  {
    key: 'status.availableIPs',
    header: 'Available IPs',
    render: (item: any) => item.status?.availableIPs ?? '-',
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function Subnets() {
  const [data, setData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [vpcs, setVpcs] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const items = await listResources('subnets');
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
    listResources('vpcs')
      .then((items) => setVpcs(items.map((v: any) => v.metadata?.name).filter(Boolean)))
      .catch(() => setVpcs([]));
  }, []);

  const fields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-subnet' },
    {
      name: 'namespace',
      label: 'Namespace',
      type: 'select',
      options: namespaces.length > 0 ? namespaces : ['default'],
      required: true,
    },
    {
      name: 'vpcRef',
      label: 'VPC',
      type: 'select',
      options: vpcs.length > 0 ? vpcs : ['(none)'],
      required: true,
    },
    { name: 'cidr', label: 'CIDR', type: 'text', required: true, placeholder: '10.0.1.0/24' },
    {
      name: 'type',
      label: 'Type',
      type: 'select',
      options: ['Private', 'Public'],
      required: true,
    },
  ];

  const handleCreate = async (values: Record<string, any>) => {
    await createResource('subnets', values.namespace, {
      apiVersion: 'vpc.sdncontroller.io/v1alpha1',
      kind: 'Subnet',
      metadata: { name: values.name, namespace: values.namespace },
      spec: { vpcRef: values.vpcRef, cidr: values.cidr, type: values.type },
    });
    setShowCreate(false);
    fetchData();
  };

  const handleDelete = async (item: any) => {
    if (!confirm(`Delete Subnet "${item.metadata.name}"?`)) return;
    await deleteResource('subnets', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Subnets</h1>
        <p className="text-sm text-gray-400">Subnet management</p>
      </div>

      <ResourceTable
        title="Subnets"
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
        title="Create Subnet"
        fields={fields}
      />
    </div>
  );
}
