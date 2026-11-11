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

/* ---------- VNFs ---------- */

const vnfColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.type', header: 'Type' },
  { key: 'spec.replicas', header: 'Replicas' },
  {
    key: 'status.readyReplicas',
    header: 'Ready',
    render: (item: any) => {
      const ready = item.status?.readyReplicas ?? 0;
      const total = item.spec?.replicas ?? 0;
      return `${ready}/${total}`;
    },
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- Service Chains ---------- */

const scColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  { key: 'spec.direction', header: 'Direction' },
  {
    key: 'chain',
    header: 'Hops',
    render: (item: any) => {
      const count = item.spec?.chain?.length ?? 0;
      return String(count);
    },
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function NFV() {
  const [vnfData, setVnfData] = useState<any[]>([]);
  const [scData, setScData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showVnfCreate, setShowVnfCreate] = useState(false);
  const [showScCreate, setShowScCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [vpcs, setVpcs] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [vnf, sc] = await Promise.all([
        listResources('virtualnetworkfunctions'),
        listResources('servicechains'),
      ]);
      setVnfData(vnf);
      setScData(sc);
    } catch {
      setVnfData([]);
      setScData([]);
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

  const vnfFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-vnf' },
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
      options: ['VM', 'Container'],
      required: true,
    },
    { name: 'replicas', label: 'Replicas', type: 'number', defaultValue: 1 },
  ];

  const scFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-chain' },
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
      name: 'direction',
      label: 'Direction',
      type: 'select',
      options: ['Ingress', 'Egress', 'Both'],
      required: true,
    },
  ];

  const handleVnfCreate = async (values: Record<string, any>) => {
    await createResource('virtualnetworkfunctions', values.namespace, {
      apiVersion: 'nfv.sdncontroller.io/v1alpha1',
      kind: 'VirtualNetworkFunction',
      metadata: { name: values.name, namespace: values.namespace },
      spec: { type: values.type, replicas: Number(values.replicas) },
    });
    setShowVnfCreate(false);
    fetchData();
  };

  const handleScCreate = async (values: Record<string, any>) => {
    await createResource('servicechains', values.namespace, {
      apiVersion: 'nfv.sdncontroller.io/v1alpha1',
      kind: 'ServiceChain',
      metadata: { name: values.name, namespace: values.namespace },
      spec: {
        vpcRef: values.vpcRef,
        direction: values.direction,
      },
    });
    setShowScCreate(false);
    fetchData();
  };

  const handleVnfDelete = async (item: any) => {
    if (!confirm(`Delete VNF "${item.metadata.name}"?`)) return;
    await deleteResource('virtualnetworkfunctions', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  const handleScDelete = async (item: any) => {
    if (!confirm(`Delete Service Chain "${item.metadata.name}"?`)) return;
    await deleteResource('servicechains', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">NFV</h1>
        <p className="text-sm text-gray-400">
          Virtual Network Functions &amp; Service Chains
        </p>
      </div>

      <ResourceTable
        title="Virtual Network Functions"
        columns={vnfColumns}
        data={vnfData}
        loading={loading}
        onCreate={() => setShowVnfCreate(true)}
        onDelete={handleVnfDelete}
      />

      <ResourceTable
        title="Service Chains"
        columns={scColumns}
        data={scData}
        loading={loading}
        onCreate={() => setShowScCreate(true)}
        onDelete={handleScDelete}
      />

      <CreateModal
        open={showVnfCreate}
        onClose={() => setShowVnfCreate(false)}
        onSave={handleVnfCreate}
        title="Create VNF"
        fields={vnfFields}
      />

      <CreateModal
        open={showScCreate}
        onClose={() => setShowScCreate(false)}
        onSave={handleScCreate}
        title="Create Service Chain"
        fields={scFields}
      />
    </div>
  );
}
