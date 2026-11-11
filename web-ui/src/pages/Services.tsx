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

/* ---------- Load Balancers ---------- */

const lbColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  {
    key: 'frontend',
    header: 'Frontend',
    render: (item: any) => {
      const fe = item.spec?.frontend;
      return fe ? `${fe.ip ?? '-'}:${fe.port ?? '-'}` : '-';
    },
  },
  { key: 'spec.algorithm', header: 'Algorithm' },
  {
    key: 'backends',
    header: 'Backends',
    render: (item: any) => {
      const count = item.spec?.backends?.length ?? 0;
      return String(count);
    },
  },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

/* ---------- Firewalls ---------- */

const fwColumns: Column[] = [
  { key: 'metadata.name', header: 'Name' },
  { key: 'metadata.namespace', header: 'Namespace' },
  { key: 'spec.vpcRef', header: 'VPC' },
  {
    key: 'rules',
    header: 'Rules',
    render: (item: any) => {
      const count = item.spec?.rules?.length ?? 0;
      return String(count);
    },
  },
  { key: 'spec.defaultAction', header: 'Default Action' },
  {
    key: 'status.phase',
    header: 'Status',
    render: (item: any) => <StatusBadge phase={item.status?.phase} />,
  },
];

export default function Services() {
  const [lbData, setLbData] = useState<any[]>([]);
  const [fwData, setFwData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showLbCreate, setShowLbCreate] = useState(false);
  const [showFwCreate, setShowFwCreate] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [vpcs, setVpcs] = useState<string[]>([]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [lb, fw] = await Promise.all([
        listResources('loadbalancers'),
        listResources('firewalls'),
      ]);
      setLbData(lb);
      setFwData(fw);
    } catch {
      setLbData([]);
      setFwData([]);
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

  const lbFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-lb' },
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
    { name: 'frontendIP', label: 'Frontend IP', type: 'text', required: true, placeholder: '10.0.0.100' },
    { name: 'frontendPort', label: 'Frontend Port', type: 'number', required: true, defaultValue: 80 },
    {
      name: 'algorithm',
      label: 'Algorithm',
      type: 'select',
      options: ['RoundRobin', 'LeastConnections', 'IPHash'],
      required: true,
    },
  ];

  const fwFields: FieldDef[] = [
    { name: 'name', label: 'Name', type: 'text', required: true, placeholder: 'my-fw' },
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
      name: 'defaultAction',
      label: 'Default Action',
      type: 'select',
      options: ['Allow', 'Deny'],
      required: true,
    },
  ];

  const handleLbCreate = async (values: Record<string, any>) => {
    await createResource('loadbalancers', values.namespace, {
      apiVersion: 'service.sdncontroller.io/v1alpha1',
      kind: 'LoadBalancer',
      metadata: { name: values.name, namespace: values.namespace },
      spec: {
        vpcRef: values.vpcRef,
        frontend: { ip: values.frontendIP, port: Number(values.frontendPort) },
        algorithm: values.algorithm,
      },
    });
    setShowLbCreate(false);
    fetchData();
  };

  const handleFwCreate = async (values: Record<string, any>) => {
    await createResource('firewalls', values.namespace, {
      apiVersion: 'service.sdncontroller.io/v1alpha1',
      kind: 'Firewall',
      metadata: { name: values.name, namespace: values.namespace },
      spec: {
        vpcRef: values.vpcRef,
        defaultAction: values.defaultAction,
      },
    });
    setShowFwCreate(false);
    fetchData();
  };

  const handleLbDelete = async (item: any) => {
    if (!confirm(`Delete Load Balancer "${item.metadata.name}"?`)) return;
    await deleteResource('loadbalancers', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  const handleFwDelete = async (item: any) => {
    if (!confirm(`Delete Firewall "${item.metadata.name}"?`)) return;
    await deleteResource('firewalls', item.metadata.namespace, item.metadata.name);
    fetchData();
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Services</h1>
        <p className="text-sm text-gray-400">Load Balancer &amp; Firewall management</p>
      </div>

      <ResourceTable
        title="Load Balancers"
        columns={lbColumns}
        data={lbData}
        loading={loading}
        onCreate={() => setShowLbCreate(true)}
        onDelete={handleLbDelete}
      />

      <ResourceTable
        title="Firewalls"
        columns={fwColumns}
        data={fwData}
        loading={loading}
        onCreate={() => setShowFwCreate(true)}
        onDelete={handleFwDelete}
      />

      <CreateModal
        open={showLbCreate}
        onClose={() => setShowLbCreate(false)}
        onSave={handleLbCreate}
        title="Create Load Balancer"
        fields={lbFields}
      />

      <CreateModal
        open={showFwCreate}
        onClose={() => setShowFwCreate(false)}
        onSave={handleFwCreate}
        title="Create Firewall"
        fields={fwFields}
      />
    </div>
  );
}
