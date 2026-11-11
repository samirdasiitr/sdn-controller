import { useMemo, useState, type ReactNode } from 'react';
import { queryMetric, queryRange, type PromSample } from '../api';
import { useData } from '../context';
import { Card } from './ui';
import { fmtBytes, fmtNum } from '../lib';

export const SERIES_COLORS = ['#3b82f6', '#22c55e', '#f59e0b', '#ef4444', '#a855f7', '#14b8a6', '#ec4899', '#84cc16'];

export type Point = [number, number];

export function toPoints(s: PromSample): Point[] {
  return (s.values ?? []).map(([t, v]) => [t, Number(v)] as Point).filter((p) => isFinite(p[1]));
}

export function Sparkline({
  points,
  width = 120,
  height = 32,
  color = '#3b82f6',
}: {
  points: Point[];
  width?: number;
  height?: number;
  color?: string;
}) {
  if (points.length < 2)
    return <div style={{ width, height }} className="flex items-center text-xs text-gray-400">no data</div>;
  const xs = points.map((p) => p[0]);
  const ys = points.map((p) => p[1]);
  const x0 = Math.min(...xs), x1 = Math.max(...xs);
  const y0 = Math.min(...ys), y1 = Math.max(...ys);
  const sx = (x: number) => ((x - x0) / (x1 - x0 || 1)) * (width - 2) + 1;
  const sy = (y: number) => height - 2 - ((y - y0) / (y1 - y0 || 1)) * (height - 4);
  const d = points.map((p, i) => `${i ? 'L' : 'M'}${sx(p[0]).toFixed(1)},${sy(p[1]).toFixed(1)}`).join(' ');
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
      <path d={`${d} L${sx(x1)},${height} L${sx(x0)},${height} Z`} fill={color} opacity={0.12} />
      <path d={d} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  );
}

export interface Series {
  label: string;
  points: Point[];
}

export function LineChart({
  series,
  height = 160,
  fmt = fmtNum,
}: {
  series: Series[];
  height?: number;
  fmt?: (n: number) => string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const W = 600;
  const padL = 48, padR = 8, padT = 8, padB = 18;
  const all = series.flatMap((s) => s.points);
  const geom = useMemo(() => {
    if (all.length === 0) return null;
    const xs = all.map((p) => p[0]);
    const ys = all.map((p) => p[1]);
    const x0 = Math.min(...xs), x1 = Math.max(...xs);
    let y0 = Math.min(0, ...ys), y1 = Math.max(...ys);
    if (y1 === y0) y1 = y0 + 1;
    return { x0, x1, y0, y1 };
  }, [all.length, series]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!geom)
    return <div style={{ height }} className="flex items-center justify-center text-sm text-gray-400">No data</div>;
  const { x0, x1, y0, y1 } = geom;
  const sx = (x: number) => padL + ((x - x0) / (x1 - x0 || 1)) * (W - padL - padR);
  const sy = (y: number) => padT + (1 - (y - y0) / (y1 - y0)) * (height - padT - padB);
  const ticks = [0, 0.5, 1].map((f) => y0 + f * (y1 - y0));
  const fmtT = (t: number) => new Date(t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const hoverT = hover !== null ? x0 + hover * (x1 - x0) : null;
  const valueAt = (s: Series, t: number) => {
    let best: Point | null = null;
    for (const p of s.points) if (!best || Math.abs(p[0] - t) < Math.abs(best[0] - t)) best = p;
    return best?.[1];
  };
  return (
    <div>
      <svg
        viewBox={`0 0 ${W} ${height}`}
        className="w-full text-gray-500 dark:text-gray-400"
        onMouseMove={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          const f = ((e.clientX - r.left) / r.width) * W;
          setHover(Math.min(1, Math.max(0, (f - padL) / (W - padL - padR))));
        }}
        onMouseLeave={() => setHover(null)}
      >
        {ticks.map((t, i) => (
          <g key={i}>
            <line x1={padL} x2={W - padR} y1={sy(t)} y2={sy(t)} stroke="currentColor" opacity={0.15} />
            <text x={padL - 4} y={sy(t) + 3} textAnchor="end" fontSize={10} fill="currentColor">{fmt(t)}</text>
          </g>
        ))}
        <text x={padL} y={height - 4} fontSize={10} fill="currentColor">{fmtT(x0)}</text>
        <text x={W - padR} y={height - 4} fontSize={10} fill="currentColor" textAnchor="end">{fmtT(x1)}</text>
        {series.map((s, i) => (
          <path
            key={s.label}
            d={s.points.map((p, j) => `${j ? 'L' : 'M'}${sx(p[0]).toFixed(1)},${sy(p[1]).toFixed(1)}`).join(' ')}
            fill="none"
            stroke={SERIES_COLORS[i % SERIES_COLORS.length]}
            strokeWidth={1.5}
            strokeLinejoin="round"
          />
        ))}
        {hoverT !== null && <line x1={sx(hoverT)} x2={sx(hoverT)} y1={padT} y2={height - padB} stroke="currentColor" opacity={0.4} />}
      </svg>
      <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600 dark:text-gray-400">
        {series.map((s, i) => (
          <span key={s.label} className="flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-full" style={{ background: SERIES_COLORS[i % SERIES_COLORS.length] }} />
            {s.label}
            {hoverT !== null && <b className="text-gray-900 dark:text-gray-100">{fmt(valueAt(s, hoverT) ?? NaN)}</b>}
          </span>
        ))}
      </div>
    </div>
  );
}

export function Stat({
  label,
  value,
  sub,
  spark,
  color,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  spark?: Point[];
  color?: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-900">
      <div className="min-w-0">
        <div className="truncate text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{label}</div>
        <div className="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">{value}</div>
        {sub && <div className="text-xs text-gray-500 dark:text-gray-400">{sub}</div>}
      </div>
      {spark && <Sparkline points={spark} color={color} />}
    </div>
  );
}

export const FORMATS = { num: fmtNum, bytes: fmtBytes, bps: (n: number) => `${fmtBytes(n)}/s`, pct: (n: number) => `${n.toFixed(0)}%` };

/** Range query rendered as a line chart. */
export function MetricChart({
  title,
  query,
  labelKey,
  labelFn,
  fmt = fmtNum,
  rangeSec = 3600,
  step = 30,
  height = 160,
  className,
}: {
  title: string;
  query: string;
  labelKey?: string | string[];
  labelFn?: (m: Record<string, string>) => string;
  fmt?: (n: number) => string;
  rangeSec?: number;
  step?: number;
  height?: number;
  className?: string;
}) {
  const { data, error, loading } = useData(() => queryRange(query, rangeSec, step), [query, rangeSec]);
  const series: Series[] = (data ?? []).map((s, i) => ({
    label: labelFn
      ? labelFn(s.metric)
      : labelKey
        ? [labelKey].flat().map((k) => s.metric[k]).filter(Boolean).join('/') || `series ${i + 1}`
        : s.metric.__name__ ?? 'value',
    points: toPoints(s),
  }));
  return (
    <Card title={title} className={className} bodyClass="p-4">
      {error ? (
        <div className="py-6 text-center text-sm text-red-500">Metrics unavailable: {error}</div>
      ) : loading ? (
        <div className="py-6 text-center text-sm text-gray-400">Loading…</div>
      ) : (
        <LineChart series={series} fmt={fmt} height={height} />
      )}
    </Card>
  );
}

/** Instant query helper hook. */
export function useInstant(query: string) {
  return useData(() => queryMetric(query), [query]);
}

export function scalar(samples?: PromSample[]): number | undefined {
  if (!samples || samples.length === 0) return undefined;
  return samples.reduce((a, s) => a + Number(s.value?.[1] ?? 0), 0);
}
