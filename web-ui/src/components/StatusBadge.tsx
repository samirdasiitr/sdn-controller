interface StatusBadgeProps {
  phase: string | undefined | null;
}

const statusColors: Record<string, string> = {
  Ready: 'bg-green-500/20 text-green-400 border-green-500/30',
  Active: 'bg-green-500/20 text-green-400 border-green-500/30',
  Running: 'bg-green-500/20 text-green-400 border-green-500/30',
  Established: 'bg-green-500/20 text-green-400 border-green-500/30',
  Bound: 'bg-green-500/20 text-green-400 border-green-500/30',
  Failed: 'bg-red-500/20 text-red-400 border-red-500/30',
  Error: 'bg-red-500/20 text-red-400 border-red-500/30',
  Pending: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/30',
  Provisioning: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  Creating: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  Terminating: 'bg-orange-500/20 text-orange-400 border-orange-500/30',
};

const defaultColor = 'bg-gray-500/20 text-gray-400 border-gray-500/30';

export default function StatusBadge({ phase }: StatusBadgeProps) {
  const label = phase ?? 'Unknown';
  const color = statusColors[label] ?? defaultColor;

  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${color}`}
    >
      {label}
    </span>
  );
}
