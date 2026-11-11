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

/* ---------- Internet Gateways ---------- */

const igwColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  {
    key: 'status.activeNode',
    header: 'Active Node',
    render: (item: any) => item.status?.activeNode ?? '-',
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- NAT Gateways ---------- */

const natColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  { key: 'spec.subnetRef', header: 'Subnet' },
  { key: 'spec.natType', header: 'NAT Type' },
  {
    key: 'status.activeNode',
    header: 'Active Node',
    render: (item: any) => item.status?.activeNode ?? '-',
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function Gateways() {
  const [igwData, setIgwData] = useState<any[]>([]);
  const [natData, setNatData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showIgwCreate, setShowIgwCreate] = useState(false);
  const [showNatCreate, setShowNatCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [vpcs, setVpcs] = useState<string[]>([]);
  const [subnets, setSubnets] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [igw, nat] = await Promise.all([
        listResources('internetgateways'),
        listResources('natgateways'),
      ]);
      setIgwData(igw);
      setNatData(nat);
    } catch {
      setIgwData([]);
      setNatData([]);
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
    listResources('subnets')
      .then((items) => setSubnets(items.map((s: any) => s.metadata?.name).filter(Boolean)))
      .catch(() => setSubnets([]));
  }, []);

  const igwFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-igw' },
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
  ];

  const natFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-nat-gw' },
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
    {
      name: 'subnetRef',
      label: 'Subnet',
      type: 'select',
      options: subnets.length > 0 ? subnets : ['(none)'],
      required: true,
    },
    {
      name: 'natType',
      label: 'NAT Type',
      type: 'select',
      options: ['SNAT', 'DNAT', 'Both'],
      required: true,
    },
  ];

  const handleIgwCreate = async (values: Record<string, any>) => {
    await createResource('internetgateways', values.namespace, {
      apiVersion: 'gateway.sdncontroller.io/v1alpha1',
      kind: 'InternetGateway',
      metadata: { name: values.name, namespace: values.namespace },
      spec: { vpcRef: values.vpcRef },
    });
    setShowIgwCreate(false);
    fetchData();
  };

  const handleNatCreate = async (values: Record<string, any>) => {
    await createResource('natgateways', values.namespace, {
      apiVersion: 'gateway.sdncontroller.io/v1alpha1',
      kind: 'NATGateway',
      metadata: { name: values.name, namespace: values.namespace },
      spec: {
        vpcRef: values.vpcRef,
        subnetRef: values.subnetRef,
        natType: values.natType,
      },
    });
    setShowNatCreate(false);
    fetchData();
  };

  const handleIgwDelete = async (item: any) => {
    if (!confirm(`Delete Internet Gateway "${item.metadata.name}"?`)) return;
    await deleteResource('internetgateways', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  const handleNatDelete = async (item: any) => {
    if (!confirm(`Delete NAT Gateway "${item.metadata.name}"?`)) return;
    await deleteResource('natgateways', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Gateways</h1>
        <p className="text-sm text-gray-400">Internet &amp; NAT Gateway management</p>
      </div>

      <ResourceTable
        title="Internet Gateways"
        columns={igwColumns}
        data={igwData}
        loading={loading}
        onCreate={() => setShowIgwCreate(true)}
        onDelete={handleIgwDelete}
      />

      <ResourceTable
        title="NAT Gateways"
        columns={natColumns}
        data={natData}
        loading={loading}
        onCreate={() => setShowNatCreate(true)}
        onDelete={handleNatDelete}
      />

      <CreateModal
        open={showIgwCreate}
        onClose={() => setShowIgwCreate(false)}
        onSave={handleIgwCreate}
        title="Create Internet Gateway"
        fields={igwFields}
      />

      <CreateModal
        open={showNatCreate}
        onClose={() => setShowNatCreate(false)}
        onSave={handleNatCreate}
        title="Create NAT Gateway"
        fields={natFields}
      />
    </div>
  );
}
