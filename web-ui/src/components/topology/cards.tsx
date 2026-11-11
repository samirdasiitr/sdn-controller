import { Handle, Position, type NodeProps } from 'reactflow';
import { Link } from 'react-router-dom';
import { Server } from 'lucide-react';
import { KIND_STYLE } from './kinds';
import { phaseDot } from '../StatusBadge';
import { detailPath } from '../../resources';

export const CARD_W = 210;
export const CARD_H = 82;

export interface CardData {
  resource: string;
  kind: string;
  name: string;
  phase?: string;
  terminating?: boolean;
  facts: string[];
  dir: 'TB' | 'LR';
  external?: boolean;
}

export function ResourceCard({ data, selected }: NodeProps<CardData>) {
  const st = KIND_STYLE[data.resource] ?? KIND_STYLE.vpcs;
  const Icon = st.icon;
  const phase = data.terminating ? 'Terminating' : data.phase;
  const tp = data.dir === 'TB' ? Position.Top : Position.Left;
  const sp = data.dir === 'TB' ? Position.Bottom : Position.Right;
  return (
    <div
      style={{ width: CARD_W, height: CARD_H, borderLeftColor: st.color }}
      className={`cursor-pointer overflow-hidden rounded-lg border border-l-4 bg-white px-3 py-2 shadow-sm transition-shadow hover:shadow-md dark:bg-gray-800 ${selected ? 'border-blue-500 ring-2 ring-blue-400/50' : 'border-gray-200 dark:border-gray-600'} ${data.external ? 'border-dashed' : ''}`}
    >
      <Handle type="target" position={tp} className="!h-2 !w-2 !border-0 !bg-gray-400" />
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide" style={{ color: st.color }}>
        <Icon className="h-3.5 w-3.5" />
        {st.label}
        <span
          className="ml-auto h-2.5 w-2.5 rounded-full"
          style={{ background: phaseDot[phase ?? ''] ?? '#9ca3af' }}
          title={phase ?? 'Unknown'}
        />
      </div>
      <div className="truncate text-sm font-semibold text-gray-900 dark:text-gray-100" title={data.name}>{data.name}</div>
      {data.facts.slice(0, 2).map((f, i) => (
        <div key={i} className="truncate text-[11px] leading-tight text-gray-500 dark:text-gray-400" title={f}>{f}</div>
      ))}
      <Handle type="source" position={sp} className="!h-2 !w-2 !border-0 !bg-gray-400" />
    </div>
  );
}

export interface GroupData {
  label: string;
  sub?: string;
  color: string;
}

export function GroupBox({ data }: NodeProps<GroupData>) {
  return (
    <div
      className="h-full w-full rounded-2xl border-2 border-dashed"
      style={{ borderColor: data.color + '99', background: data.color + '0d', pointerEvents: 'none' }}
    >
      <div className="px-4 py-2 text-sm font-semibold" style={{ color: data.color }}>
        {data.label}
        {data.sub && <span className="ml-2 text-xs font-normal opacity-80">{data.sub}</span>}
      </div>
    </div>
  );
}

export interface HostData {
  name: string;
  ip?: string;
  ready?: boolean;
  external?: boolean;
  note?: string;
  bridges: { name: string; up: boolean; ports?: number; flows?: number }[];
  vpcs: { name: string; ns: string; vni?: number; tunnelIP?: string; bridge?: string }[];
  roles: { resource: string; ns: string; name: string }[];
}

export const hostHeight = (d: HostData) =>
  64 + (d.bridges.length ? 22 + d.bridges.length * 20 : 0) + (d.vpcs.length ? 22 + d.vpcs.length * 20 : 0) + (d.roles.length ? 22 + d.roles.length * 20 : 0) + 12;
export const HOST_W = 340;

export function HostCard({ data, selected }: NodeProps<HostData>) {
  const sec = 'mt-2 border-t border-gray-100 pt-1.5 dark:border-gray-700';
  const h = 'mb-0.5 text-[10px] font-semibold uppercase tracking-wide text-gray-400';
  return (
    <div
      style={{ width: HOST_W }}
      className={`rounded-xl border bg-white p-3 shadow-sm dark:bg-gray-800 ${selected ? 'border-blue-500 ring-2 ring-blue-400/50' : 'border-gray-200 dark:border-gray-600'} ${data.external ? 'border-dashed' : ''}`}
    >
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !border-0 !bg-gray-400" />
      <div className="flex items-center gap-2">
        <Server className="h-4 w-4 text-sky-500" />
        <span className="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">{data.name}</span>
        <span
          className="ml-auto h-2.5 w-2.5 shrink-0 rounded-full"
          style={{ background: data.ready === undefined ? '#9ca3af' : data.ready ? '#22c55e' : '#ef4444' }}
          title={data.ready === undefined ? 'Unknown' : data.ready ? 'Ready' : 'NotReady'}
        />
      </div>
      <div className="text-[11px] text-gray-500 dark:text-gray-400">{[data.ip, data.note].filter(Boolean).join(' · ')}</div>
      {data.bridges.length > 0 && (
        <div className={sec}>
          <div className={h}>OVS bridges</div>
          {data.bridges.map((b) => (
            <div key={b.name} className="flex h-5 items-center gap-2 text-xs text-gray-700 dark:text-gray-300">
              <span className="h-2 w-2 rounded-full" style={{ background: b.up ? '#22c55e' : '#ef4444' }} />
              <span className="font-mono">{b.name}</span>
              <span className="ml-auto text-gray-500">{b.ports ?? '?'} ports · {b.flows ?? '?'} flows</span>
            </div>
          ))}
        </div>
      )}
      {data.vpcs.length > 0 && (
        <div className={sec}>
          <div className={h}>VPC attachments</div>
          {data.vpcs.map((v) => (
            <div key={v.name} className="flex h-5 items-center gap-2 text-xs text-gray-700 dark:text-gray-300">
              <Link to={detailPath('vpcs', v.ns, v.name)} className="nodrag shrink-0 whitespace-nowrap text-blue-600 hover:underline dark:text-blue-400">{v.name}</Link>
              <span className="ml-auto min-w-0 truncate text-gray-500" title={v.tunnelIP}>{[v.vni !== undefined ? `VNI ${v.vni}` : '', v.bridge, v.tunnelIP].filter(Boolean).join(' · ')}</span>
            </div>
          ))}
        </div>
      )}
      {data.roles.length > 0 && (
        <div className={sec}>
          <div className={h}>Hosts</div>
          {data.roles.map((r) => (
            <div key={r.resource + r.name} className="flex h-5 items-center gap-2 text-xs">
              <span className="text-gray-500">{KIND_STYLE[r.resource]?.label}</span>
              <Link to={detailPath(r.resource, r.ns, r.name)} className="nodrag text-blue-600 hover:underline dark:text-blue-400">{r.name}</Link>
            </div>
          ))}
        </div>
      )}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-0 !bg-gray-400" />
    </div>
  );
}

