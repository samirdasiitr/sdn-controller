import 'reactflow/dist/style.css';
import { useEffect, useMemo } from 'react';
import ReactFlow, {
  Background, Controls, Handle, MarkerType, Position, ReactFlowProvider, useReactFlow,
  type Edge, type Node, type NodeProps,
} from 'reactflow';
import { useNavigate } from 'react-router-dom';
import { Cable, MonitorSmartphone, Network, Server } from 'lucide-react';
import { getNodeOvs, listNodes, listResources, type NodeOvsTopology } from '../api';
import { useData, useTheme } from '../context';
import { fmtBytes } from '../lib';
import { detailPath, nodePath } from '../resources';
import { ErrorBanner } from './ui';

/* ---------------- node data ---------------- */

const COL_W = 340;
const COL_X = [0, COL_W, COL_W * 2, COL_W * 3];

interface VMD {
  kind: 'vm';
  vm: string;
  ip?: string;
  mac?: string;
  port: string;
  ofport: number;
  linkState?: string;
  vnicNs?: string;
  vnicName?: string;
  rx?: number;
  tx?: number;
  tip: string;
}
interface BrD {
  kind: 'br';
  name: string;
  datapath?: string;
  flows?: number;
  gateways: string[];
  onClick?: () => void;
}
interface UlD {
  kind: 'ul';
  name: string;
  mac?: string;
  mtu?: number;
  state?: string;
  ips: string[];
  defaultRoute?: boolean;
  tip: string;
}
interface RnD {
  kind: 'rn';
  ip: string;
  nodeName?: string;
  via: string; // e.g. "VXLAN VNI 1000"
}
type D = VMD | BrD | UlD | RnD;

const box =
  'nodrag w-[300px] rounded-xl border bg-white px-3 py-2 text-left shadow-sm transition-shadow hover:shadow-md dark:bg-gray-800';
const roleBorder: Record<D['kind'], string> = {
  vm: 'border-emerald-400 border-l-4 dark:border-emerald-500',
  br: 'border-sky-400 border-l-4 dark:border-sky-500',
  ul: 'border-amber-400 border-l-4 dark:border-amber-500',
  rn: 'border-violet-400 border-l-4 dark:border-violet-500',
};
const dot = (ok?: boolean) => (
  <span className="inline-block h-2 w-2 rounded-full" style={{ background: ok === undefined ? '#9ca3af' : ok ? '#22c55e' : '#ef4444' }} />
);

function ConnNode({ data }: NodeProps<D>) {
  const navigate = useNavigate();
  const clickable = data.kind === 'br' || (data.kind === 'rn' && !!data.nodeName) || (data.kind === 'vm' && !!data.vnicName);
  const go = () => {
    if (data.kind === 'br') data.onClick?.();
    else if (data.kind === 'rn' && data.nodeName) navigate(nodePath(data.nodeName));
    else if (data.kind === 'vm' && data.vnicName) navigate(detailPath('vnics', data.vnicNs ?? 'default', data.vnicName));
  };
  return (
    <button type="button" title={'tip' in data ? data.tip : undefined} onClick={go}
      className={`${box} ${roleBorder[data.kind]} ${clickable ? 'cursor-pointer' : 'cursor-default'}`}>
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !border-0 !bg-gray-400" />
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-0 !bg-gray-400" />
      {data.kind === 'vm' && (
        <>
          <div className="flex items-center gap-1.5 text-xs font-semibold text-emerald-700 dark:text-emerald-300">
            <MonitorSmartphone className="h-3.5 w-3.5" />{data.vm}
            <span className="ml-auto">{dot(data.linkState === undefined ? undefined : data.linkState === 'up')}</span>
          </div>
          <div className="mt-0.5 text-[11px] text-gray-600 dark:text-gray-300">
            {data.ip && <span className="font-mono">{data.ip}</span>}
            {data.mac && <span className="ml-2 font-mono text-gray-400">{data.mac}</span>}
          </div>
          <div className="text-[10px] text-gray-400">port {data.port} · ofport {data.ofport} · vnic {data.vnicName}</div>
          {(data.rx !== undefined || data.tx !== undefined) && (
            <div className="text-[10px] text-gray-400">rx {fmtBytes(data.rx ?? 0)} · tx {fmtBytes(data.tx ?? 0)}</div>
          )}
        </>
      )}
      {data.kind === 'br' && (
        <>
          <div className="flex items-center gap-1.5 text-xs font-semibold text-sky-700 dark:text-sky-300">
            <Network className="h-3.5 w-3.5" />{data.name}
          </div>
          <div className="mt-0.5 text-[11px] text-gray-600 dark:text-gray-300">
            OVS bridge · {data.datapath ?? '?'} datapath{data.flows !== undefined ? ` · ${data.flows} rules` : ''}
          </div>
          {data.gateways.length > 0 && (
            <div className="text-[10px] text-gray-400">gateway: <span className="font-mono">{data.gateways.join(', ')}</span></div>
          )}
          <div className="text-[10px] text-sky-500">click → OpenFlow rules</div>
        </>
      )}
      {data.kind === 'ul' && (
        <>
          <div className="flex items-center gap-1.5 text-xs font-semibold text-amber-700 dark:text-amber-300">
            <Cable className="h-3.5 w-3.5" />{data.name} <span className="font-normal text-gray-400">(underlay)</span>
            <span className="ml-auto">{dot(data.state === undefined ? undefined : data.state === 'up')}</span>
          </div>
          <div className="mt-0.5 font-mono text-[11px] text-gray-600 dark:text-gray-300">{data.ips.join(', ') || 'no IP'}</div>
          <div className="text-[10px] text-gray-400">
            {[data.mac, data.mtu ? `mtu ${data.mtu}` : '', data.defaultRoute ? 'default route' : ''].filter(Boolean).join(' · ')}
          </div>
        </>
      )}
      {data.kind === 'rn' && (
        <>
          <div className="flex items-center gap-1.5 text-xs font-semibold text-violet-700 dark:text-violet-300">
            <Server className="h-3.5 w-3.5" />{data.nodeName ?? 'remote node'}
          </div>
          <div className="mt-0.5 font-mono text-[11px] text-gray-600 dark:text-gray-300">{data.ip}</div>
          <div className="text-[10px] text-gray-400">{data.via}</div>
          {data.nodeName && <div className="text-[10px] text-violet-500">click → node page</div>}
        </>
      )}
    </button>
  );
}

const nodeTypes = { conn: ConnNode };

/* ---------------- layout ---------------- */

function buildGraph(
  ovs: NodeOvsTopology,
  vnics: any[],
  nodeNames: { name: string; internalIP: string }[],
  onBridgeClick: (br: string) => void,
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  const vnicOf = (key?: string) => vnics.find((v) => key === `${v.metadata.namespace}/${v.metadata.name}`);
  const remoteName = (ip?: string) => nodeNames.find((n) => n.internalIP === ip)?.name;

  let vmY = 0;
  const add = (id: string, col: number, y: number, data: D) =>
    nodes.push({ id, type: 'conn', position: { x: COL_X[col], y }, data });

  const edge = (id: string, source: string, target: string, label?: string, color = '#94a3b8', dashed = false) =>
    edges.push({
      id, source, target, type: 'smoothstep', label,
      labelStyle: { fontSize: 10, fill: color },
      labelBgPadding: [4, 2], labelBgBorderRadius: 4,
      style: { stroke: color, strokeWidth: 1.5, strokeDasharray: dashed ? '5 4' : undefined },
      markerEnd: { type: MarkerType.ArrowClosed, color, width: 14, height: 14 },
    });

  for (const b of ovs.bridges) {
    const bid = `br:${b.name}`;
    add(bid, 1, vmY, {
      kind: 'br', name: b.name, datapath: b.datapathType, flows: b.flows,
      gateways: b.addresses ?? [], onClick: () => onBridgeClick(b.name),
    });
    let yOff = 0;

    for (const p of b.ports) {
      if (p.role === 'workload') {
        const v = vnicOf(p.vnic);
        const id = `vm:${b.name}:${p.name}`;
        add(id, 0, vmY + yOff, {
          kind: 'vm',
          vm: v?.spec?.attachment?.name ?? p.vnic ?? p.name,
          ip: v?.status?.ipAddress ?? v?.spec?.ipAddress,
          mac: v?.status?.macAddress ?? p.mac,
          port: p.name, ofport: p.ofport, linkState: p.linkState,
          vnicNs: p.vnic?.split('/')[0], vnicName: p.vnic?.split('/')[1],
          rx: p.stats?.rxBytes, tx: p.stats?.txBytes,
          tip: `${p.name} ofport=${p.ofport} mac=${p.mac ?? '-'} mtu=${p.mtu ?? '-'} rx=${fmtBytes(p.stats?.rxBytes ?? 0)} tx=${fmtBytes(p.stats?.txBytes ?? 0)} rxErr=${p.stats?.rxErrors ?? 0} txErr=${p.stats?.txErrors ?? 0} rxDrop=${p.stats?.rxDropped ?? 0} txDrop=${p.stats?.txDropped ?? 0}`,
        });
        edge(`${id}->${bid}`, id, bid, `${p.name} (of ${p.ofport})`, '#34d399');
        yOff += 92;
      } else if (p.role === 'tunnel') {
        const target = ovs.uplinks[0]?.name ?? 'uplink';
        const ulId = `ul:${target}`;
        edge(`${bid}->${ulId}:${p.name}`, bid, ulId, `${p.type || 'tunnel'} ${p.name}${p.vni ? ` · VNI ${p.vni}` : ''} → ${p.remoteIP ?? '?'}`, '#f59e0b', true);
        const rid = `rn:${p.remoteIP}`;
        if (!nodes.some((n) => n.id === rid)) {
          add(rid, 3, 0, { kind: 'rn', ip: p.remoteIP ?? '?', nodeName: remoteName(p.remoteIP), via: `${(p.type || 'tunnel').toUpperCase()}${p.vni ? ` VNI ${p.vni}` : ''} via ${target}` });
        }
        edge(`${ulId}->${rid}:${p.name}`, ulId, rid, `${(p.type || 'tunnel').toUpperCase()}${p.vni ? ` VNI ${p.vni}` : ''}`, '#f59e0b');
      } else if (p.role === 'uplink') {
        edge(`${bid}->ul:${p.name}:${p.name}`, bid, `ul:${p.name}`, `${p.name} (of ${p.ofport})`, '#f59e0b');
      } else if (p.role !== 'internal' && p.role !== 'patch' && p.role !== '') {
        // 'other' ports: small edge note on the bridge
      }
    }
    // internal port NAT edge(s)
    for (const nat of ovs.nat) {
      const ulId = `ul:${nat.uplink}`;
      edge(`${bid}->${ulId}:nat`, bid, ulId, `NAT (SNAT) for ${nat.vpc} ${nat.source}`, '#fb7185', true);
    }
    vmY += Math.max(yOff, 80) + 24;
  }

  ovs.uplinks.forEach((u, i) => {
    add(`ul:${u.name}`, 2, 40 + i * 96, {
      kind: 'ul', name: u.name, mac: u.mac, mtu: u.mtu, state: u.state,
      ips: u.addresses ?? [], defaultRoute: u.defaultRoute,
      tip: `${u.name} mac=${u.mac ?? '-'} mtu=${u.mtu ?? '-'} state=${u.state ?? '-'} ${u.physical ? 'physical NIC' : ''} ${u.defaultRoute ? 'default route' : ''}`,
    });
  });

  // stack remote nodes
  let ry = 0;
  for (const n of nodes.filter((n) => n.id.startsWith('rn:'))) {
    n.position.y = ry;
    ry += 92;
  }
  return { nodes, edges };
}

/* ---------------- component ---------------- */

function Inner({ ovs, vnics, k8sNodes, onBridgeClick }: {
  ovs: NodeOvsTopology; vnics: any[]; k8sNodes: { name: string; internalIP: string }[];
  onBridgeClick: (br: string) => void;
}) {
  const { theme } = useTheme();
  const { fitView } = useReactFlow();
  const { nodes, edges } = useMemo(
    () => buildGraph(ovs, vnics, k8sNodes, onBridgeClick),
    [ovs, vnics, k8sNodes, onBridgeClick],
  );
  useEffect(() => {
    const t = setTimeout(() => fitView({ padding: 0.15, duration: 200 }), 50);
    return () => clearTimeout(t);
  }, [nodes, fitView]);
  return (
    <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView minZoom={0.2} nodesConnectable={false} proOptions={{ hideAttribution: true }}>
      <Background color={theme === 'dark' ? '#374151' : '#d1d5db'} gap={20} />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}

export default function NodeConnections({ node, onBridgeClick }: { node: string; onBridgeClick: (br: string) => void }) {
  const { data: ovs, error, loading } = useData(() => getNodeOvs(node), [node]);
  const { data: vnics } = useData(() => listResources('vnics').catch(() => []), []);
  const { data: k8sNodes } = useData(() => listNodes().catch(() => []), []);

  if (loading && !ovs) return <div className="px-5 py-8 text-center text-sm text-gray-400">Loading node datapath…</div>;
  if (error) return <ErrorBanner message={`Cannot load datapath view: ${error}`} />;
  if (!ovs || (ovs.bridges.length === 0 && ovs.uplinks.length === 0))
    return <div className="px-5 py-8 text-center text-sm text-gray-400">No OVS datapath information reported by the agent on this node.</div>;

  return (
    <div className="h-[420px]">
      <ReactFlowProvider>
        <Inner ovs={ovs} vnics={vnics ?? []} k8sNodes={(k8sNodes ?? []).map((n) => ({ name: n.name, internalIP: n.internalIP }))} onBridgeClick={onBridgeClick} />
      </ReactFlowProvider>
      <div className="flex flex-wrap gap-x-4 gap-y-1 border-t border-gray-100 px-4 py-2 text-[11px] text-gray-500 dark:border-gray-800 dark:text-gray-400">
        <span className="flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-emerald-400" />guest VM / workload</span>
        <span className="flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-sky-400" />OVS bridge</span>
        <span className="flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-amber-400" />host uplink (underlay)</span>
        <span className="flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-violet-400" />remote node</span>
        <span className="ml-auto">dashed = tunnel/NAT · click a bridge for its flow rules</span>
      </div>
    </div>
  );
}
