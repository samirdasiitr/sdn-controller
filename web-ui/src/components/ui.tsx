import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, RefreshCw } from 'lucide-react';
import { KIND_TO_RESOURCE, RESOURCES, detailPath, nodePath } from '../resources';
import { useRefresh } from '../context';
import { placementOf } from '../placement';

export const btnPrimary =
  'inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-500 disabled:opacity-50';
export const btnSecondary =
  'inline-flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700';
export const inputCls =
  'w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-50 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 dark:placeholder-gray-500';
export const linkCls = 'text-blue-600 hover:underline dark:text-blue-400';

export function Card({
  title,
  actions,
  children,
  className = '',
  bodyClass = '',
}: {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClass?: string;
}) {
  return (
    <section
      className={`overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-gray-700 dark:bg-gray-900 ${className}`}
    >
      {title && (
        <div className="flex items-center justify-between border-b border-gray-200 px-5 py-3 dark:border-gray-700">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">{title}</h2>
          {actions}
        </div>
      )}
      <div className={bodyClass}>{children}</div>
    </section>
  );
}

export function RefreshButton({ compact = false }: { compact?: boolean }) {
  const { refresh, fetching } = useRefresh();
  return (
    <button onClick={refresh} className={btnSecondary} title="Refresh data">
      <RefreshCw className={`h-4 w-4 ${fetching > 0 ? 'animate-spin' : ''}`} />
      {!compact && 'Refresh'}
    </button>
  );
}

export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">{title}</h1>
        {subtitle && <p className="text-sm text-gray-500 dark:text-gray-400">{subtitle}</p>}
      </div>
      <div className="flex items-center gap-2">
        {actions}
        <RefreshButton />
      </div>
    </div>
  );
}

export function Breadcrumb({ items }: { items: { label: string; to?: string }[] }) {
  return (
    <nav className="mb-3 flex flex-wrap items-center gap-1 text-sm text-gray-500 dark:text-gray-400">
      {items.map((it, i) => (
        <span key={i} className="flex items-center gap-1">
          {i > 0 && <ChevronRight className="h-3.5 w-3.5" />}
          {it.to ? (
            <Link to={it.to} className={linkCls}>
              {it.label}
            </Link>
          ) : (
            <span className="font-medium text-gray-900 dark:text-gray-100">{it.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}

/** Link to a resource detail page. */
export function EntityLink({
  resource,
  ns,
  name,
  children,
}: {
  resource: string;
  ns: string;
  name?: string | null;
  children?: ReactNode;
}) {
  if (!name) return <span className="text-gray-400">-</span>;
  return (
    <Link
      to={detailPath(resource, ns, name)}
      onClick={(e) => e.stopPropagation()}
      className={linkCls}
    >
      {children ?? name}
    </Link>
  );
}

/** Links for lists of refs (subnetRefs). */
export function EntityLinks({ resource, ns, names }: { resource: string; ns: string; names?: string[] }) {
  if (!names || names.length === 0) return <span className="text-gray-400">-</span>;
  return (
    <span className="flex flex-wrap gap-x-2">
      {names.map((n, i) => (
        <span key={n}>
          <EntityLink resource={resource} ns={ns} name={n} />
          {i < names.length - 1 && ','}
        </span>
      ))}
    </span>
  );
}

/** "Kind/name" reference (status.usedBy). */
export function KindRefLink({ value, ns }: { value: string; ns: string }) {
  const [kind, ...rest] = value.split('/');
  const name = rest.join('/');
  const res = KIND_TO_RESOURCE[kind];
  if (!res || !name) return <span>{value}</span>;
  return (
    <EntityLink resource={res} ns={ns} name={name}>
      <span className="text-gray-500 dark:text-gray-400">{RESOURCES[res].label}/</span>
      {name}
    </EntityLink>
  );
}

export function NodeLink({ name }: { name?: string | null }) {
  if (!name) return <span className="text-gray-400">-</span>;
  return (
    <Link to={nodePath(name)} onClick={(e) => e.stopPropagation()} className={linkCls}>
      {name}
    </Link>
  );
}

export function Props({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-1 gap-x-6 gap-y-3 p-5 sm:grid-cols-2 lg:grid-cols-3">
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {k}
          </dt>
          <dd className="mt-0.5 break-words text-sm text-gray-900 dark:text-gray-100">
            {v === undefined || v === null || v === '' ? <span className="text-gray-400">-</span> : v}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export interface Col<T> {
  header: string;
  render: (row: T, i: number) => ReactNode;
  className?: string;
}

export function MiniTable({
  cols,
  rows,
  empty = 'None',
}: {
  cols: Col<any>[];
  rows: any[];
  empty?: string;
}) {
  if (!rows || rows.length === 0)
    return <div className="px-5 py-6 text-center text-sm text-gray-400">{empty}</div>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800/60">
            {cols.map((c) => (
              <th
                key={c.header}
                className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
              >
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
          {rows.map((r, i) => (
            <tr key={i} className="text-gray-700 dark:text-gray-300">
              {cols.map((c) => (
                <td key={c.header} className={`px-4 py-2 ${c.className ?? ''}`}>
                  {c.render(r, i)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function ProgressBar({ value, max, label }: { value: number; max: number; label?: string }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  const color = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-blue-500';
  return (
    <div>
      <div className="mb-1 flex justify-between text-xs text-gray-500 dark:text-gray-400">
        <span>{label}</span>
        <span>
          {value} / {max} ({pct.toFixed(1)}%)
        </span>
      </div>
      <div className="h-2.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export function ErrorBanner({ message }: { message: string | null | undefined }) {
  if (!message) return null;
  return (
    <div className="mb-4 rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-300">
      {message}
    </div>
  );
}

const badgeBase = 'inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium';
export function Badge({ tone, children, title }: { tone: 'green' | 'blue' | 'yellow' | 'gray'; children: ReactNode; title?: string }) {
  const c = {
    green: 'bg-green-100 text-green-700 border-green-300 dark:bg-green-500/20 dark:text-green-400 dark:border-green-500/30',
    blue: 'bg-blue-100 text-blue-700 border-blue-300 dark:bg-blue-500/20 dark:text-blue-400 dark:border-blue-500/30',
    yellow: 'bg-yellow-100 text-yellow-700 border-yellow-300 dark:bg-yellow-500/20 dark:text-yellow-400 dark:border-yellow-500/30',
    gray: 'bg-gray-100 text-gray-600 border-gray-300 dark:bg-gray-500/20 dark:text-gray-400 dark:border-gray-500/30',
  }[tone];
  return <span title={title} className={`${badgeBase} ${c}`}>{children}</span>;
}

/** "Runs on" cell: node links (with active/standby badges for gateways) or a "Not placed" warning. */
export function RunsOn({ resource, item }: { resource: string; item: any }) {
  const p = placementOf(resource, item);
  if (p.length === 0) {
    const cond = (item?.status?.conditions ?? []).find((c: any) => c.type === 'Placed');
    return <Badge tone="yellow" title={cond?.message ?? cond?.reason}>Not placed</Badge>;
  }
  return (
    <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
      {p.map((x) => (
        <span key={x.node + x.role} className="inline-flex items-center gap-1">
          <NodeLink name={x.node} />
          {x.role === 'active' && <Badge tone="green">active</Badge>}
          {x.role === 'standby' && <Badge tone="blue">standby</Badge>}
        </span>
      ))}
    </span>
  );
}
