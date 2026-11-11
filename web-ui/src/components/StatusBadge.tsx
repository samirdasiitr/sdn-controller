import { EntityLink, KindRefLink } from './ui';

const green = 'bg-green-100 text-green-700 border-green-300 dark:bg-green-500/20 dark:text-green-400 dark:border-green-500/30';
const red = 'bg-red-100 text-red-700 border-red-300 dark:bg-red-500/20 dark:text-red-400 dark:border-red-500/30';
const yellow = 'bg-yellow-100 text-yellow-700 border-yellow-300 dark:bg-yellow-500/20 dark:text-yellow-400 dark:border-yellow-500/30';
const blue = 'bg-blue-100 text-blue-700 border-blue-300 dark:bg-blue-500/20 dark:text-blue-400 dark:border-blue-500/30';
const orange = 'bg-orange-100 text-orange-700 border-orange-300 dark:bg-orange-500/20 dark:text-orange-400 dark:border-orange-500/30';

const statusColors: Record<string, string> = {
  Ready: green, Active: green, Running: green, Established: green, Bound: green, Configured: green, Up: green,
  Failed: red, Error: red, NotReady: red, Down: red,
  Pending: yellow, Degraded: yellow, Establishing: yellow,
  Provisioning: blue, Creating: blue, Allocated: blue,
  Terminating: orange, Deleting: orange,
};

export const phaseDot: Record<string, string> = {
  Ready: '#22c55e', Active: '#22c55e', Running: '#22c55e', Established: '#22c55e', Configured: '#22c55e',
  Failed: '#ef4444', Error: '#ef4444', NotReady: '#ef4444',
  Pending: '#eab308', Degraded: '#eab308', Establishing: '#eab308',
  Provisioning: '#3b82f6', Creating: '#3b82f6', Allocated: '#3b82f6', Bound: '#22c55e',
  Terminating: '#f97316', Deleting: '#f97316',
};

const defaultColor =
  'bg-gray-100 text-gray-600 border-gray-300 dark:bg-gray-500/20 dark:text-gray-400 dark:border-gray-500/30';

export default function StatusBadge({ phase }: { phase: string | undefined | null }) {
  const label = phase ?? 'Unknown';
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${statusColors[label] ?? defaultColor}`}
    >
      {label}
    </span>
  );
}

/** Phase badge that switches to "Terminating" + blocked-by links when deletionTimestamp is set. */
export function ResourceStatus({ item, showBlockers = true }: { item: any; showBlockers?: boolean }) {
  const terminating = !!item?.metadata?.deletionTimestamp;
  if (!terminating) return <StatusBadge phase={item?.status?.phase} />;
  const usedBy: string[] = item?.status?.usedBy ?? [];
  return (
    <div className="flex flex-col items-start gap-1">
      <StatusBadge phase="Terminating" />
      {showBlockers && usedBy.length > 0 && (
        <span className="text-xs text-orange-600 dark:text-orange-400">
          blocked by:{' '}
          {usedBy.map((u, i) => (
            <span key={u}>
              <KindRefLink value={u} ns={item.metadata.namespace} />
              {i < usedBy.length - 1 && ', '}
            </span>
          ))}
        </span>
      )}
    </div>
  );
}

export { EntityLink };
