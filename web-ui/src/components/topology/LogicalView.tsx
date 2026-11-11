import 'reactflow/dist/style.css';
import { useEffect, useMemo } from 'react';
import ReactFlow, {
  Background, Controls, MarkerType, MiniMap, ReactFlowProvider, useReactFlow, type Edge, type Node,
} from 'reactflow';
import dagre from 'dagre';
import { useNavigate } from 'react-router-dom';
import type { TopologyData } from '../../api';
import { useTheme } from '../../context';
import { detailPath } from '../../resources';
import { PLACED } from '../../placement';
import { TAG_PREFIX, type Tag } from '../../tags';
import { CARD_H, CARD_W, GroupBox, ResourceCard } from './cards';
import { EDGE_COLORS, KIND_STYLE, facts } from './kinds';

const nodeTypes = { card: ResourceCard, group: GroupBox };
const EXCLUDED = new Set(['nodes', 'tunnels', 'nodenetworkconfigs']);
const HOST_EDGES = new Set(['active_on', 'standby_on', 'attached_node', 'runs_on']);
const PAD = 28;
const HEADER = 40;

const nodeTags = (n: { labels?: Record<string, string> }): Tag[] =>
  Object.entries(n.labels ?? {})
    .filter(([k]) => k.startsWith(TAG_PREFIX))
    .map(([k, v]) => ({ key: k.slice(TAG_PREFIX.length), value: v }))
    .sort((a, b) => a.key.localeCompare(b.key));

export function layoutLogical(topo: TopologyData, dir: 'TB' | 'LR', tagFilter: Tag[] = []) {
  const nodes = topo.nodes.filter((n) => !EXCLUDED.has(n.resource));
  const ids = new Set(nodes.map((n) => n.id));
  const edges = topo.edges.filter((e) => ids.has(e.source) && ids.has(e.target) && !HOST_EDGES.has(e.type));
  const hostsOf = new Map<string, string[]>();
  for (const e of topo.edges)
    if (HOST_EDGES.has(e.type) && e.type !== 'attached_node' && e.target.startsWith('nodes/'))
      hostsOf.set(e.source, [...(hostsOf.get(e.source) ?? []), e.target.replace('nodes//', '') + (e.type === 'standby_on' ? ' (standby)' : '')]);

  // assign each node to a VPC group (BFS from VPC nodes along parent->child edges)
  const children = new Map<string, string[]>();
  for (const e of edges) children.set(e.source, [...(children.get(e.source) ?? []), e.target]);
  const group = new Map<string, string>();
  for (const v of nodes.filter((n) => n.resource === 'vpcs')) {
    const q = [v.id];
    group.set(v.id, v.id);
    while (q.length) {
      const cur = q.shift()!;
      for (const c of children.get(cur) ?? []) {
        if (!group.has(c)) {
          group.set(c, v.id);
          q.push(c);
        }
      }
    }
  }
  const byGroup = new Map<string, typeof nodes>();
  for (const n of nodes) {
    const g = group.get(n.id) ?? '__none__';
    byGroup.set(g, [...(byGroup.get(g) ?? []), n]);
  }

  const outNodes: Node[] = [];
  let cursorX = 0, cursorY = 0, rowH = 0;
  const maxRow = 1800;
  const groupKeys = [...byGroup.keys()].sort((a, b) => (a === '__none__' ? 1 : b === '__none__' ? -1 : a.localeCompare(b)));
  for (const g of groupKeys) {
    const members = byGroup.get(g)!;
    const dg = new dagre.graphlib.Graph();
    dg.setGraph({ rankdir: dir, ranksep: 56, nodesep: 28, marginx: 0, marginy: 0 });
    dg.setDefaultEdgeLabel(() => ({}));
    const mids = new Set(members.map((m) => m.id));
    members.forEach((m) => dg.setNode(m.id, { width: CARD_W, height: CARD_H }));
    edges.filter((e) => mids.has(e.source) && mids.has(e.target)).forEach((e) => dg.setEdge(e.source, e.target));
    dagre.layout(dg);
    let w = 0, h = 0;
    members.forEach((m) => {
      const p = dg.node(m.id);
      w = Math.max(w, p.x + CARD_W / 2);
      h = Math.max(h, p.y + CARD_H / 2);
    });
    const boxW = w + PAD * 2, boxH = h + PAD * 2 + HEADER;
    if (dir === 'TB') {
      if (cursorX > 0 && cursorX + boxW > maxRow) { cursorX = 0; cursorY += rowH + 60; rowH = 0; }
    }
    const ox = cursorX, oy = cursorY;
    const vpc = g === '__none__' ? null : nodes.find((n) => n.id === g)!;
    outNodes.push({
      id: `group:${g}`,
      type: 'group',
      position: { x: ox, y: oy },
      data: {
        label: vpc ? `VPC ${vpc.name}` : 'Unassigned',
        sub: vpc ? [vpc.spec?.cidr, vpc.spec?.tunnelType, vpc.spec?.vni !== undefined ? `VNI ${vpc.spec.vni}` : ''].filter(Boolean).join(' · ') : 'not part of a VPC',
        color: vpc ? KIND_STYLE.vpcs.color : '#6b7280',
      },
      style: { width: boxW, height: boxH },
      draggable: false,
      selectable: false,
      zIndex: -1,
    });
    members.forEach((m) => {
      const p = dg.node(m.id);
      outNodes.push({
        id: m.id,
        type: 'card',
        position: { x: ox + PAD + p.x - CARD_W / 2, y: oy + PAD + HEADER + p.y - CARD_H / 2 },
        data: {
          resource: m.resource, kind: m.kind, name: m.name, phase: m.status?.phase,
          terminating: !!m.deletionTimestamp, facts: facts(m), dir,
          hosts: hostsOf.get(m.id), notPlaced: PLACED.includes(m.resource) && !hostsOf.has(m.id),
          tags: nodeTags(m),
          dimmed: tagFilter.length > 0 && !tagFilter.every(
            (t) => (m.labels ?? {})[TAG_PREFIX + t.key] === t.value,
          ),
        },
      });
    });
    if (dir === 'TB') { cursorX += boxW + 60; rowH = Math.max(rowH, boxH); }
    else { cursorY += boxH + 60; }
  }

  const outEdges: Edge[] = edges.map((e) => {
    const c = EDGE_COLORS[e.type] ?? '#94a3b8';
    return {
      id: e.id,
      source: e.source,
      target: e.target,
      type: 'smoothstep',
      label: e.label,
      labelStyle: { fontSize: 10 },
      labelBgPadding: [4, 2],
      labelBgBorderRadius: 4,
      style: { stroke: c, strokeWidth: 1.5, strokeDasharray: e.type === 'protected_by' ? '5 4' : undefined },
      markerEnd: { type: MarkerType.ArrowClosed, color: c, width: 16, height: 16 },
    } as Edge;
  });
  return { nodes: outNodes, edges: outEdges };
}

function Inner({ topo, dir, tagFilter }: { topo: TopologyData; dir: 'TB' | 'LR'; tagFilter: Tag[] }) {
  const navigate = useNavigate();
  const { theme } = useTheme();
  const { fitView } = useReactFlow();
  const { nodes, edges } = useMemo(() => layoutLogical(topo, dir, tagFilter), [topo, dir, tagFilter]);
  useEffect(() => {
    const t = setTimeout(() => fitView({ padding: 0.1, duration: 300 }), 50);
    return () => clearTimeout(t);
  }, [nodes, fitView]);
  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      minZoom={0.1}
      nodesConnectable={false}
      onNodeClick={(_, n) => {
        if (n.type !== 'card') return;
        const [res, ns, name] = n.id.split('/');
        navigate(detailPath(res, ns, name));
      }}
    >
      <Background color={theme === 'dark' ? '#374151' : '#d1d5db'} gap={20} />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable nodeColor={(n) => (n.type === 'group' ? 'transparent' : KIND_STYLE[(n.data as any)?.resource]?.color ?? '#9ca3af')}
        maskColor={theme === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(240,240,240,0.6)'} />
    </ReactFlow>
  );
}

export default function LogicalView(props: { topo: TopologyData; dir: 'TB' | 'LR'; tagFilter: Tag[] }) {
  return (
    <ReactFlowProvider>
      <Inner {...props} />
    </ReactFlowProvider>
  );
}
