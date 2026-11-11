import { Handle, Position, type NodeProps } from 'reactflow';
import { Link } from 'react-router-dom';
import { Server } from 'lucide-react';
import { KIND_STYLE } from './kinds';
import { phaseDot } from '../StatusBadge';
import { detailPath } from '../../resources';

export const CARD_W = 210;
export const CARD_H = 96;

export interface CardData {
  resource: string;
  kind: string;
  name: string;
  phase?: string;
  terminating?: boolean;
  facts: string[];
  dir: 'TB' | 'LR';
  external?: boolean;
  hosts?: string[];
  notPlaced?: boolean;
  tags?: { key: string; value: string }[];
  dimmed?: boolean;
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
      className={`cursor-pointer overflow-hidden rounded-lg border border-l-4 bg-white px-3 py-2 shadow-sm transition-shadow hover:shadow-md dark:bg-gray-800 ${selected ? 'border-blue-500 ring-2 ring-blue-400/50' : 'border-gray-200 dark:border-gray-600'} ${data.external ? 'border-dashed' : ''} ${data.dimmed ? 'opacity-30 saturate-50' : ''}`}
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
      {data.tags && data.tags.length > 0 && (
        <div className="mt-0.5 flex flex-wrap gap-0.5">
          {data.tags.slice(0, 3).map((t) => (
            <span key={t.key} title={`${t.key}=${t.value}`}
              className="max-w-[95px] truncate rounded-full bg-purple-100 px-1.5 text-[9px] font-medium leading-4 text-purple-700 dark:bg-purple-500/20 dark:text-purple-300">
              {t.key}={t.value === '' ? '∅' : t.value}
            </span>
          ))}
          {data.tags.length > 3 && <span className="text-[9px] text-gray-400">+{data.tags.length - 3}</span>}
        </div>
      )}
      {data.facts.slice(0, 2).map((f, i) => (
        <div key={i} className="truncate text-[11px] leading-tight text-gray-500 dark:text-gray-400" title={f}>{f}</div>
      ))}
      {data.hosts && data.hosts.length > 0 && (
        <div className="flex items-center gap-1 truncate text-[10px] text-sky-600 dark:text-sky-400" title={`node: ${data.hosts.join(', ')}`}>
          <Server className="h-3 w-3 shrink-0" />
          <span className="truncate">node: {data.hosts.join(', ')}</span>
        </div>
      )}
      {data.notPlaced && <div className="text-[10px] text-yellow-600 dark:text-yellow-400">not placed</div>}
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
  stats?: { rules: number; offloaded: number };
  vpcs: { name: string; ns: string; vni?: number; tunnelIP?: string; bridge?: string }[];
  chips: { resource: string; ns?: string; name: string; label?: string; title?: string; role?: string }[];
}

export const hostHeight = (d: HostData) =>
  64 + (d.stats ? 14 : 0) + (d.bridges.length ? 22 + d.bridges.length * 20 : 0) + (d.vpcs.length ? 22 + d.vpcs.length * 20 : 0) + (d.chips.length ? 22 + Math.ceil(d.chips.length / 2) * 26 : 0) + 12;
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
      {data.stats && (
        <div className="text-[11px] text-gray-500 dark:text-gray-400">
          {data.stats.rules} rules · {data.stats.offloaded} offloaded
        </div>
      )}
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
      {data.chips.length > 0 && (
        <div className={sec}>
          <div className={h}>Hosted services</div>
          <div className="flex flex-wrap gap-1">
            {data.chips.map((c) => {
              const st = KIND_STYLE[c.resource];
              const body = (
                <>
                  <span className="font-semibold" style={{ color: st?.color }}>{c.label ?? st?.label}</span>
                  <span className="max-w-[110px] truncate">{c.name}</span>
                  {c.role && c.role !== 'placed' && c.role !== 'bound' && <span className="text-gray-400">({c.role})</span>}
                </>
              );
              const cls = 'nodrag flex h-6 items-center gap-1 rounded-full border border-gray-200 bg-gray-50 px-2 text-[11px] text-gray-700 dark:border-gray-600 dark:bg-gray-700/50 dark:text-gray-200';
              return c.ns ? (
                <Link key={c.resource + c.ns + c.name + (c.role ?? '')} to={detailPath(c.resource, c.ns, c.name)} title={c.title} className={cls + ' hover:border-blue-400'}>{body}</Link>
              ) : (
                <span key={c.resource + c.name} title={c.title} className={cls}>{body}</span>
              );
            })}
          </div>
        </div>
      )}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-0 !bg-gray-400" />
    </div>
  );
}

