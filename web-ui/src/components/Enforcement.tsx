import type { ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import { fmtBytes, fmtNum } from '../lib';
import { Badge, Card, EntityLink, MiniTable, NodeLink, Props } from './ui';

const num = (n: any) => (n === undefined || n === null ? '-' : fmtNum(Number(n)));
const bytes = (n: any) => (n === undefined || n === null ? '-' : fmtBytes(Number(n)));

export function Chip({ children, tone = 'amber' }: { children: ReactNode; tone?: 'amber' | 'red' }) {
  const c = tone === 'amber'
    ? 'border-amber-300 bg-amber-100 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/15 dark:text-amber-300'
    : 'border-red-300 bg-red-100 text-red-700 dark:border-red-500/30 dark:bg-red-500/15 dark:text-red-300';
  return <span className={`inline-block rounded-full border px-2 py-0.5 text-[11px] font-medium ${c}`}>{children}</span>;
}

type Kind = 'nat' | 'lb' | 'fw';

/** Prominent banner of all distinct enforcement warnings/messages across nodes. */
export function EnforcementWarnings({ item }: { item: any }) {
  const enf: Record<string, any> = item.status?.enforcement ?? {};
  const warns = new Map<string, string[]>();
  const errs: [string, string][] = [];
  for (const [node, e] of Object.entries(enf)) {
    for (const w of e?.warnings ?? []) warns.set(w, [...(warns.get(w) ?? []), node]);
    if (e?.message) errs.push([node, e.message]);
  }
  if (warns.size === 0 && errs.length === 0) return null;
  return (
    <div className="space-y-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
      {[...warns].map(([w, nodes]) => (
        <div key={w} className="flex items-start gap-2">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span><b>{w}</b> <span className="text-xs opacity-70">({nodes.join(', ')})</span></span>
        </div>
      ))}
      {errs.map(([n, m]) => (
        <div key={n} className="flex items-start gap-2 text-red-700 dark:text-red-300">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{m} <span className="text-xs opacity-70">({n})</span></span>
        </div>
      ))}
    </div>
  );
}

function sumEnf(item: any, f: string): number {
  return Object.values<any>(item.status?.enforcement ?? {}).reduce((a, e) => a + (Number(e?.[f]) || 0), 0);
}

export function CountersPanel({ kind, item }: { kind: Kind; item: any }) {
  const st = item.status ?? {};
  const hasEnf = !!st.enforcement && Object.keys(st.enforcement).length > 0;
  const packets = hasEnf ? sumEnf(item, 'packets') : undefined;
  const by = hasEnf ? sumEnf(item, 'bytes') : undefined;
  const items: [string, ReactNode][] =
    kind === 'nat'
      ? [
          ['Active translations', num(st.translationCount ?? 0)],
          ['Total translations (cumulative)', num(st.totalTranslations ?? 0)],
          ['Flows installed', hasEnf ? num(sumEnf(item, 'flows')) : undefined],
          ['Packets', num(packets)],
          ['Bytes', bytes(by)],
        ]
      : kind === 'lb'
      ? [
          ['Active connections', num(st.activeConnections ?? 0)],
          ['Total connections (cumulative)', num(st.totalConnections ?? 0)],
          ['Flows installed', hasEnf ? num(sumEnf(item, 'flows')) : undefined],
          ['Packets', num(packets)],
          ['Bytes', bytes(by)],
        ]
      : [
          ['Matched packets', num(st.matchedPackets ?? 0)],
          ['Active rules', st.activeRules],
          ['Flows installed', hasEnf ? num(sumEnf(item, 'flows')) : undefined],
          ['Packets', num(packets)],
          ['Bytes', bytes(by)],
        ];
  return (
    <Card title="Counters">
      <Props items={items} />
    </Card>
  );
}

export function PerNodeEnforcement({ kind, item }: { kind: Kind; item: any }) {
  const rows = Object.entries<any>(item.status?.enforcement ?? {}).map(([node, e]) => ({ node, ...e }));
  const counters = (e: any) =>
    kind === 'nat'
      ? `${num(e.translations)} active · ${num(e.newTranslations)} new`
      : kind === 'lb'
      ? `${num(e.connections)} active · ${num(e.newConnections)} new`
      : `${num(e.matchedPackets)} matched`;
  return (
    <Card title={`Per-node enforcement (${rows.length})`}>
      <MiniTable rows={rows} empty="No node has reported enforcement status yet" cols={[
        { header: 'Node', render: (r) => <NodeLink name={r.node} /> },
        { header: 'Programmed', render: (r) => <Badge tone={r.programmed ? 'green' : 'yellow'}>{r.programmed ? 'programmed' : 'not programmed'}</Badge> },
        { header: 'Gen', render: (r) => r.observedGeneration ?? '-' },
        { header: 'Flows', render: (r) => num(r.flows) },
        { header: kind === 'nat' ? 'Translations' : kind === 'lb' ? 'Connections' : 'Matches', render: counters },
        { header: 'Packets', render: (r) => num(r.packets) },
        { header: 'Bytes', render: (r) => bytes(r.bytes) },
        { header: 'Warnings', render: (r) => (r.warnings?.length ? <span className="flex flex-wrap gap-1">{r.warnings.map((w: string) => <Chip key={w}>{w}</Chip>)}</span> : '') },
        { header: 'Message', render: (r) => (r.message ? <span className="text-red-600 dark:text-red-400">{r.message}</span> : '') },
      ]} />
    </Card>
  );
}

export function RuleCounters({ item }: { item: any }) {
  const rows = [...(item.status?.ruleCounters ?? [])].sort((a: any, b: any) => (b.packets ?? 0) - (a.packets ?? 0));
  const color = (a: string) => (a === 'Allow' ? 'text-green-600 dark:text-green-400' : a === 'Deny' ? 'text-red-600 dark:text-red-400' : 'text-yellow-600 dark:text-yellow-400');
  return (
    <Card title={`Per-rule counters (${rows.length})`}>
      <MiniTable rows={rows} empty="No rule counters reported yet" cols={[
        { header: 'Rule', render: (r) => r.name },
        { header: 'Action', render: (r) => <b className={color(r.action)}>{r.action}</b> },
        { header: 'Packets', render: (r) => <span title={String(r.packets ?? 0)}>{num(r.packets ?? 0)}</span> },
        { header: 'Bytes', render: (r) => bytes(r.bytes ?? 0) },
      ]} />
    </Card>
  );
}

export function BackendHealth({ item, vnics }: { item: any; vnics: any[] }) {
  const rows: any[] = item.status?.backends ?? [];
  const ns = item.metadata.namespace;
  const byIP = new Map(vnics.filter((v) => v.status?.ipAddress).map((v) => [v.status.ipAddress, v]));
  const healthy = rows.filter((b) => b.healthy).length;
  return (
    <Card title={`Backend health (${healthy} / ${rows.length} healthy)`}>
      <MiniTable rows={rows} empty="No backend health reported yet" cols={[
        { header: 'IP', render: (b) => {
          const v = byIP.get(b.ip);
          return v ? <VnicLink ns={ns} v={v} ip={b.ip} /> : b.ip;
        } },
        { header: 'Healthy', render: (b) => <Badge tone={b.healthy ? 'green' : 'yellow'}>{b.healthy ? 'healthy' : 'unhealthy'}</Badge> },
        { header: 'MAC', render: (b) => <code className="text-xs">{b.mac || '-'}</code> },
        { header: 'Message', render: (b) => (b.message ? <span className="text-red-600 dark:text-red-400">{b.message}</span> : '') },
      ]} />
    </Card>
  );
}

function VnicLink({ ns, v, ip }: { ns: string; v: any; ip: string }) {
  return (
    <EntityLink resource="vnics" ns={v.metadata?.namespace ?? ns} name={v.metadata.name}>
      {ip} <span className="text-xs text-gray-500">({v.metadata.name})</span>
    </EntityLink>
  );
}
