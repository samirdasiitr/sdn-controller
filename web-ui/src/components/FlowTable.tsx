import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Pause, Play, RefreshCw } from 'lucide-react';
import { getNodeFlows, listResources, type OvsFlow } from '../api';
import { fmtBytes, fmtNum } from '../lib';
import { detailPath } from '../resources';
import { Badge, ErrorBanner } from './ui';

export interface VnicRef {
  key: string; // "ns/name"
  name: string;
  namespace: string;
  macs: string[];
  ips: string[];
  vm?: string; // spec.attachment.name
}

export function useVnicIndex() {
  const [vnics, setVnics] = useState<VnicRef[]>([]);
  useEffect(() => {
    listResources('vnics')
      .then((l) =>
        setVnics(
          l.map((v: any) => ({
            key: `${v.metadata.namespace}/${v.metadata.name}`,
            name: v.metadata.name,
            namespace: v.metadata.namespace,
            macs: [v.status?.macAddress, v.spec?.macAddress].filter(Boolean).map((s: string) => s.toLowerCase()),
            ips: [v.status?.ipAddress, v.spec?.ipAddress].filter(Boolean),
            vm: v.spec?.attachment?.name,
          })),
        ),
      )
      .catch(() => setVnics([]));
  }, []);
  return vnics;
}

/* ---------------- match / action chips ---------------- */

const MATCH_COLORS: Record<string, string> = {
  in_port: 'bg-sky-100 text-sky-800 dark:bg-sky-500/20 dark:text-sky-300',
  dl_src: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-300',
  dl_dst: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-300',
  nw_src: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-500/20 dark:text-indigo-300',
  nw_dst: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-500/20 dark:text-indigo-300',
  arp_spa: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-500/20 dark:text-indigo-300',
  arp_tpa: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-500/20 dark:text-indigo-300',
  arp_sha: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-300',
  tp_src: 'bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300',
  tp_dst: 'bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-300',
};
const PROTO = new Set(['ip', 'ip6', 'arp', 'tcp', 'udp', 'sctp', 'icmp', 'icmp6', 'tcp6', 'udp6', 'vlan', 'mpls', 'rarp']);

function splitTop(s: string, sep = ','): string[] {
  const out: string[] = [];
  let depth = 0, cur = '';
  for (const ch of s) {
    if (ch === '(' || ch === '[') depth++;
    if (ch === ')' || ch === ']') depth--;
    if (ch === sep && depth === 0) { out.push(cur); cur = ''; } else cur += ch;
  }
  if (cur) out.push(cur);
  return out;
}

const chip = 'inline-block rounded px-1.5 py-0.5 font-mono text-[11px] leading-tight';
const plain = 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300';

function MatchChips({ match, portNames, vnics }: { match: string; portNames?: Record<number, string>; vnics: VnicRef[] }) {
  const vnicLinks = new Map<string, VnicRef>();
  for (const v of vnics) {
    for (const m of v.macs) if (m) vnicLinks.set(m, v);
    for (const ip of v.ips) if (ip) vnicLinks.set(`ip:${ip}`, v);
  }
  return (
    <span className="flex max-w-xl flex-wrap items-center gap-1">
      {splitTop(match).map((tok, i) => {
        const [field, val] = tok.includes('=') ? [tok.slice(0, tok.indexOf('=')), tok.slice(tok.indexOf('=') + 1)] : [tok, ''];
        const isProto = PROTO.has(field);
        const cls = isProto
          ? 'bg-gray-200 font-semibold uppercase text-gray-600 dark:bg-gray-700 dark:text-gray-300'
          : MATCH_COLORS[field] ?? plain;
        let extra: React.ReactNode = null;
        if (field === 'in_port' && portNames) {
          const nm = portNames[Number(val)];
          if (nm) extra = <span className="ml-1 font-sans text-[10px] text-sky-600 dark:text-sky-400">({nm})</span>;
        }
        const hit = vnicLinks.get(val.toLowerCase()) ?? vnicLinks.get(`ip:${val}`);
        const body = (
          <span key={i} className={`${chip} ${cls}`} title={tok}>
            {tok}
            {extra}
          </span>
        );
        return hit ? (
          <Link key={i} to={detailPath('vnics', hit.namespace, hit.name)} title={`VNic ${hit.name}${hit.vm ? ` (${hit.vm})` : ''}`} className="nodrag">
            {body}
          </Link>
        ) : body;
      })}
    </span>
  );
}

const ACTION_COLORS: [RegExp, string][] = [
  [/^NORMAL$/, 'bg-green-100 text-green-800 dark:bg-green-500/20 dark:text-green-300'],
  [/drop/i, 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-300'],
  [/^resubmit/i, 'bg-blue-100 text-blue-800 dark:bg-blue-500/20 dark:text-blue-300'],
  [/^(output|in_port|group)/i, 'bg-purple-100 text-purple-800 dark:bg-purple-500/20 dark:text-purple-300'],
  [/^(ct|nat)/i, 'bg-orange-100 text-orange-800 dark:bg-orange-500/20 dark:text-orange-300'],
  [/^(load|set|move|mod_|push|pop|learn|clone|dec_ttl|set_field|conjunction|multipath)/i, 'bg-slate-100 text-slate-700 dark:bg-slate-500/20 dark:text-slate-300'],
];

function ActionChips({ actions, portNames }: { actions: string; portNames?: Record<number, string> }) {
  return (
    <span className="flex max-w-md flex-wrap items-center gap-1">
      {splitTop(actions).map((tok, i) => {
        const c = ACTION_COLORS.find(([re]) => re.test(tok));
        let disp = tok;
        const m = /^output:(\d+)$/.exec(tok);
        if (m && portNames?.[Number(m[1])]) disp = `${tok} (${portNames[Number(m[1])]})`;
        return <span key={i} className={`${chip} ${c ? c[1] : plain}`} title={tok}>{disp}</span>;
      })}
    </span>
  );
}

/* ---------------- main table ---------------- */

export interface FlowTableProps {
  node: string;
  bridge: string;
  portNames?: Record<number, string>; // ofport -> port name (from OVS topology)
  autoIntervalMs?: number;
}

export default function FlowTable({ node, bridge, portNames, autoIntervalMs = 5000 }: FlowTableProps) {
  const [flows, setFlows] = useState<OvsFlow[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [auto, setAuto] = useState(false);
  const [filter, setFilter] = useState('');
  const [hideZero, setHideZero] = useState(false);
  const [tick, setTick] = useState(0);
  const vnics = useVnicIndex();
  const prevRef = useRef<{ ts: number; pkts: Map<string, number> }>({ ts: 0, pkts: new Map() });
  const [pps, setPps] = useState<Map<string, number>>(new Map());

  const load = useCallback(async () => {
    try {
      const d = await getNodeFlows(node, bridge);
      const now = Date.now();
      const prev = prevRef.current;
      const next = new Map<string, number>();
      const cur = new Map<string, number>();
      for (const f of d.flows) {
        const k = `${f.table}|${f.priority}|${f.match}|${f.actions}`;
        cur.set(k, f.packets);
        const p = prev.pkts.get(k);
        if (p !== undefined && prev.ts > 0 && f.packets >= p) {
          next.set(k, Math.max(0, (f.packets - p) / ((now - prev.ts) / 1000)));
        }
      }
      prevRef.current = { ts: now, pkts: cur };
      setPps(next);
      setFlows(d.flows);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [node, bridge]);

  useEffect(() => {
    setLoading(true);
    prevRef.current = { ts: 0, pkts: new Map() };
    setPps(new Map());
    load();
  }, [load, tick]);

  useEffect(() => {
    if (!auto) return;
    const id = setInterval(load, autoIntervalMs);
    return () => clearInterval(id);
  }, [auto, load, autoIntervalMs]);

  const fkey = (f: OvsFlow) => `${f.table}|${f.priority}|${f.match}|${f.actions}`;

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return flows.filter((f) => {
      if (hideZero && f.packets === 0) return false;
      if (q && !`${f.table} ${f.priority} ${f.match} ${f.actions}`.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [flows, filter, hideZero]);

  const tables = useMemo(() => {
    const m = new Map<number, OvsFlow[]>();
    for (const f of shown) m.set(f.table, [...(m.get(f.table) ?? []), f]);
    return [...m.entries()].sort((a, b) => a[0] - b[0]);
  }, [shown]);

  const th = 'px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400';

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 border-b border-gray-100 px-4 py-2.5 dark:border-gray-800">
        <input
          className="w-64 rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm outline-none focus:border-blue-500 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
          placeholder="filter rules…" value={filter} onChange={(e) => setFilter(e.target.value)}
        />
        <label className="flex cursor-pointer items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
          <input type="checkbox" checked={hideZero} onChange={(e) => setHideZero(e.target.checked)} />
          hide zero-hit
        </label>
        <button onClick={() => setAuto((a) => !a)} className="flex items-center gap-1 rounded-lg border border-gray-300 px-2.5 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
          title="Re-fetch every 5s to compute live pps">
          {auto ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
          {auto ? 'auto 5s' : 'auto off'}
        </button>
        <button onClick={() => setTick((t) => t + 1)} className="flex items-center gap-1 rounded-lg border border-gray-300 px-2.5 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700">
          <RefreshCw className="h-3.5 w-3.5" /> Refresh
        </button>
        <span className="ml-auto text-xs text-gray-400">{shown.length} / {flows.length} rules</span>
      </div>
      <ErrorBanner message={error} />
      {loading && flows.length === 0 ? (
        <div className="px-4 py-6 text-center text-sm text-gray-400">Loading OpenFlow rules…</div>
      ) : shown.length === 0 && !error ? (
        <div className="px-4 py-6 text-center text-sm text-gray-400">{flows.length ? 'No rules match the filter' : 'No OpenFlow rules on this bridge'}</div>
      ) : (
        tables.map(([tbl, list]) => (
          <div key={tbl}>
            <div className="border-b border-gray-100 bg-gray-50/70 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:border-gray-800 dark:bg-gray-800/50 dark:text-gray-400">
              Table {tbl} · {list.length} rule{list.length === 1 ? '' : 's'}
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 dark:border-gray-800">
                    <th className={th}>Prio</th>
                    <th className={th}>Match</th>
                    <th className={th}>Actions</th>
                    <th className={`${th} text-right`}>Packets</th>
                    <th className={`${th} text-right`}>Bytes</th>
                    <th className={`${th} text-right`}>pps</th>
                    <th className={`${th} text-right`}>Idle</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50 dark:divide-gray-800/60">
                  {list.map((f) => {
                    const hot = f.packets > 0;
                    const rate = pps.get(fkey(f));
                    return (
                      <tr key={fkey(f)} className={hot ? 'bg-green-50/60 dark:bg-green-500/5' : 'text-gray-500 dark:text-gray-400'}>
                        <td className="px-3 py-1.5 font-mono text-xs">{f.priority}</td>
                        <td className="px-3 py-1.5"><MatchChips match={f.match} portNames={portNames} vnics={vnics} /></td>
                        <td className="px-3 py-1.5"><ActionChips actions={f.actions} portNames={portNames} /></td>
                        <td className={`px-3 py-1.5 text-right font-mono text-xs ${hot ? 'font-semibold text-green-700 dark:text-green-400' : ''}`}>{fmtNum(f.packets)}</td>
                        <td className="px-3 py-1.5 text-right font-mono text-xs">{fmtBytes(f.bytes)}</td>
                        <td className="px-3 py-1.5 text-right font-mono text-xs">{rate === undefined ? '–' : rate >= 0.05 ? fmtNum(rate) : '~0'}</td>
                        <td className="px-3 py-1.5 text-right font-mono text-xs">{f.idleAge}s</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        ))
      )}
    </div>
  );
}

/** Build ofport -> port-name map from a NodeOvsTopology. */
export function portNamesFor(bridges: { name: string; ports: { name: string; ofport: number }[] }[], bridge: string) {
  const b = bridges.find((x) => x.name === bridge);
  const m: Record<number, string> = {};
  for (const p of b?.ports ?? []) m[p.ofport] = p.name;
  return m;
}

export function Explainer() {
  return (
    <div className="flex items-start gap-2 px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
      <Badge tone="blue">pipeline</Badge>
      <span>
        Table 0 does <b>port security</b> (anti-spoof: in_port + dl_src + nw_src/arp_spa per VNic) and installs
        <b> routes</b> — higher tables handle routing, NAT and services. Highlighted rows are seeing traffic;
        counters are cumulative, “pps” is the delta between the last two refreshes.
      </span>
    </div>
  );
}
