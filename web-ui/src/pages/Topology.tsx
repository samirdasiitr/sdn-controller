import { useState, useEffect, useCallback, useMemo } from 'react';
import ReactFlow, { MiniMap, Controls, Background } from 'reactflow';
import type { Node, Edge } from 'reactflow';
import 'reactflow/dist/style.css';
import { getTopology } from '../api';
import { Loader2 } from 'lucide-react';

const typeColors: Record<string, string> = {
  VPC: '#3b82f6',
  Subnet: '#8b5cf6',
  InternetGateway: '#f97316',
  NATGateway: '#f97316',
  LoadBalancer: '#eab308',
  Firewall: '#ef4444',
  VirtualNetworkFunction: '#22c55e',
  ServiceChain: '#ec4899',
  Tunnel: '#14b8a6',
  Bond: '#06b6d4',
  NetworkInterface: '#6366f1',
  NodeNetworkConfig: '#64748b',
};

const typeOrder: Record<string, number> = {
  VPC: 0,
  Subnet: 1,
  InternetGateway: 2,
  NATGateway: 2,
  LoadBalancer: 3,
  Firewall: 3,
  VirtualNetworkFunction: 4,
  ServiceChain: 4,
  Tunnel: 5,
  Bond: 5,
  NetworkInterface: 6,
  NodeNetworkConfig: 6,
};

function layoutNodes(rawNodes: any[]): Node[] {
  const groups: Record<number, any[]> = {};

  for (const n of rawNodes) {
    const order = typeOrder[n.data?.kind] ?? typeOrder[n.type] ?? 7;
    if (!groups[order]) groups[order] = [];
    groups[order].push(n);
  }

  const sortedLevels = Object.keys(groups)
    .map(Number)
    .sort((a, b) => a - b);

  const nodes: Node[] = [];
  const yGap = 150;
  const xGap = 250;

  for (let levelIdx = 0; levelIdx < sortedLevels.length; levelIdx++) {
    const level = sortedLevels[levelIdx];
    const group = groups[level];
    const totalWidth = (group.length - 1) * xGap;
    const startX = -totalWidth / 2;

    for (let i = 0; i < group.length; i++) {
      const n = group[i];
      const kind = n.data?.kind ?? n.type ?? 'Unknown';
      const bgColor = typeColors[kind] ?? '#64748b';

      nodes.push({
        id: n.id,
        position: { x: startX + i * xGap, y: levelIdx * yGap },
        data: {
          label: (
            <div className="text-center">
              <div
                className="text-[10px] font-semibold uppercase tracking-wide"
                style={{ color: bgColor }}
              >
                {kind}
              </div>
              <div className="text-sm font-medium text-gray-100">
                {n.data?.label ?? n.data?.name ?? n.id}
              </div>
              {n.data?.namespace && (
                <div className="text-[10px] text-gray-400">
                  {n.data.namespace}
                </div>
              )}
            </div>
          ),
        },
        style: {
          background: '#1f2937',
          border: `2px solid ${bgColor}`,
          borderRadius: 12,
          padding: 12,
          minWidth: 160,
        },
      });
    }
  }

  return nodes;
}

function mapEdges(rawEdges: any[]): Edge[] {
  return rawEdges.map((e: any, i: number) => ({
    id: e.id ?? `edge-${i}`,
    source: e.source,
    target: e.target,
    label: e.label,
    style: { stroke: '#4b5563' },
    labelStyle: { fill: '#9ca3af', fontSize: 10 },
    animated: !!e.animated,
  }));
}

export default function Topology() {
  const [rawNodes, setRawNodes] = useState<any[]>([]);
  const [rawEdges, setRawEdges] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchTopology = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getTopology();
      setRawNodes(data.nodes ?? []);
      setRawEdges(data.edges ?? []);
    } catch {
      setRawNodes([]);
      setRawEdges([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTopology();
  }, [fetchTopology]);

  const nodes = useMemo(() => layoutNodes(rawNodes), [rawNodes]);
  const edges = useMemo(() => mapEdges(rawEdges), [rawEdges]);

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-12 w-12 animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col space-y-4">
      <div className="shrink-0">
        <h1 className="text-2xl font-bold text-gray-100">Network Topology</h1>
        <p className="text-sm text-gray-400">
          Visual overview of network resources and connections
        </p>
      </div>

      {/* Legend */}
      <div className="flex shrink-0 flex-wrap gap-4">
        {Object.entries(typeColors).map(([kind, color]) => (
          <div key={kind} className="flex items-center gap-1.5 text-xs text-gray-400">
            <div
              className="h-3 w-3 rounded-full"
              style={{ backgroundColor: color }}
            />
            {kind}
          </div>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden rounded-xl border border-gray-700">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          attributionPosition="bottom-left"
          style={{ background: '#030712' }}
        >
          <MiniMap
            nodeColor={(n) => {
              const kind = n.data?.kind ?? '';
              return typeColors[kind] ?? '#64748b';
            }}
            style={{ background: '#111827' }}
          />
          <Controls
            style={{ background: '#1f2937', borderColor: '#374151' }}
          />
          <Background color="#1f2937" gap={20} />
        </ReactFlow>
      </div>
    </div>
  );
}
