import 'reactflow/dist/style.css';
import { useEffect, useMemo } from 'react';
import ReactFlow, {
  Background, Controls, MarkerType, MiniMap, ReactFlowProvider, useReactFlow, type Edge, type Node,
} from 'reactflow';
import dagre from 'dagre';
import type { K8sNode, TopologyData, TopologyNode } from '../../api';
import { useTheme } from '../../context';
import { EntityLink, MiniTable } from '../ui';
import StatusBadge from '../StatusBadge';
import { HOST_W, HostCard, hostHeight, type HostData } from './cards';
import { EDGE_COLORS } from './kinds';

export interface Bridge { node: string; bridge: string; up: boolean; ports?: number; flows?: number }

const nodeTypes = { host: HostCard };

export function layoutPhysical(topo: TopologyData, k8s: K8sNode[], bridges: Bridge[], selected?: string | null) {
  const hosts = new Map<string, HostData>();
  const host = (name: string): HostData => {
    let h = hosts.get(name);
    if (!h) {
      h = { name, bridges: [], vpcs: [], roles: [], external: true, note: 'not a Kubernetes node' };
      hosts.set(name, h);
    }
    return h;
  };
  for (const n of k8s) {
    hosts.set(n.name, { name: n.name, ip: n.internalIP, ready: n.ready, bridges: [], vpcs: [], roles: [], note: n.kubeletVersion });
  }
  for (const b of bridges) {
    const h = host(b.node);
    if (h.external) h.note = 'OVS host (not a Kubernetes node)';
    h.bridges.push({ name: b.bridge, up: b.up, ports: b.ports, flows: b.flows });
  }
  for (const n of topo.nodes) {
    if (n.resource === 'vpcs') {
      for (const a of n.status?.attachedNodes ?? [])
        host(a.nodeName).vpcs.push({ name: n.name, ns: n.namespace, vni: n.spec?.vni ?? n.status?.allocatedVNI, tunnelIP: a.tunnelIP, bridge: a.bridge });
    }
    if (n.status?.activeNode && (n.resource === 'internetgateways' || n.resource === 'natgateways'))
      host(n.status.activeNode).roles.push({ resource: n.resource, ns: n.namespace, name: n.name });
    if (n.resource === 'tunnels') {
      for (const k of ['localEndpoint', 'remoteEndpoint']) {
        const e = n.spec?.[k];
        if (e?.nodeName) {
          const h = host(e.nodeName);
          if (!h.ip && e.ip) h.ip = e.ip;
        }
      }
    }
  }

  // group tunnels per node pair
  const pairs = new Map<string, { a: string; b: string; items: TopologyNode[] }>();
  for (const t of topo.nodes.filter((n) => n.resource === 'tunnels')) {
    const a = t.spec?.localEndpoint?.nodeName, b = t.spec?.remoteEndpoint?.nodeName;
    if (!a || !b) continue;
    const key = [a, b].sort().join('|');
    const p = pairs.get(key) ?? { a, b, items: [] as TopologyNode[] };
    p.items.push(t);
    pairs.set(key, p);
  }

  const dg = new dagre.graphlib.Graph();
  dg.setGraph({ rankdir: 'LR', ranksep: 140, nodesep: 40 });
  dg.setDefaultEdgeLabel(() => ({}));
  for (const [name, h] of hosts) dg.setNode(name, { width: HOST_W, height: hostHeight(h) });
  for (const p of pairs.values()) dg.setEdge(p.a, p.b);
  dagre.layout(dg);

  const nodes: Node[] = [...hosts.entries()].map(([name, h]) => {
    const p = dg.node(name);
    return { id: name, type: 'host', position: { x: p.x - HOST_W / 2, y: p.y - hostHeight(h) / 2 }, data: h, selected: name === selected };
  });
  const edges: Edge[] = [...pairs.values()].map((p) => {
    const ok = p.items.every((t) => t.status?.phase === 'Established');
    const c = ok ? EDGE_COLORS.tunnel_endpoint : '#eab308';
    return {
      id: `${p.a}|${p.b}`,
      source: p.a,
      target: p.b,
      type: 'default',
      label: p.items.map((t) => `${t.spec?.type ?? 'tunnel'}${t.spec?.vni !== undefined ? ` ${t.spec.vni}` : ''}`).join(' · '),
      labelStyle: { fontSize: 10 },
      labelBgPadding: [4, 2],
      labelBgBorderRadius: 4,
      animated: ok,
      style: { stroke: c, strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: c },
      markerStart: { type: MarkerType.ArrowClosed, color: c },
    } as Edge;
  });
  return { nodes, edges };
}

function Inner({ topo, k8s, bridges, selected }: Props) {
  const { theme } = useTheme();
  const { fitView } = useReactFlow();
  const { nodes, edges } = useMemo(() => layoutPhysical(topo, k8s, bridges, selected), [topo, k8s, bridges, selected]);
  useEffect(() => {
    const t = setTimeout(() => fitView({ padding: 0.15, duration: 300 }), 50);
    return () => clearTimeout(t);
  }, [nodes.length, fitView]);
  return (
    <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView minZoom={0.2} nodesConnectable={false}>
      <Background color={theme === 'dark' ? '#374151' : '#d1d5db'} gap={20} />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable nodeColor="#0ea5e9" maskColor={theme === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(240,240,240,0.6)'} />
    </ReactFlow>
  );
}

interface Props { topo: TopologyData; k8s: K8sNode[]; bridges: Bridge[]; selected?: string | null }

export default function PhysicalView(props: Props) {
  return (
    <ReactFlowProvider>
      <Inner {...props} />
    </ReactFlowProvider>
  );
}

export function TunnelTable({ topo }: { topo: TopologyData }) {
  const tunnels = topo.nodes.filter((n) => n.resource === 'tunnels');
  return (
    <MiniTable
      rows={tunnels}
      empty="No tunnels defined"
      cols={[
        { header: 'Tunnel', render: (t) => <EntityLink resource="tunnels" ns={t.namespace} name={t.name} /> },
        { header: 'Type', render: (t) => t.spec?.type },
        { header: 'VNI', render: (t) => t.spec?.vni ?? '-' },
        { header: 'Local', render: (t) => `${t.spec?.localEndpoint?.nodeName} (${t.spec?.localEndpoint?.ip})` },
        { header: 'Remote', render: (t) => `${t.spec?.remoteEndpoint?.nodeName} (${t.spec?.remoteEndpoint?.ip})` },
        { header: 'Status', render: (t) => <StatusBadge phase={t.status?.phase} /> },
      ]}
    />
  );
}
