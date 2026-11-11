import { useState, useEffect, useCallback } from 'react';
import { listResources } from '../api';
import StatusBadge from '../components/StatusBadge';
import {
  Network,
  Layers,
  Globe,
  Shield,
  Server,
  GitBranch,
  Loader2,
  ArrowUpDown,
  Cpu,
  Cable,
  Router,
  Link2,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

interface ResourceTypeInfo {
  type: string;
  label: string;
  icon: LucideIcon;
  color: string;
}

const resourceTypes: ResourceTypeInfo[] = [
  { type: 'vpcs', label: 'VPCs', icon: Network, color: 'text-blue-400 bg-blue-500/20' },
  { type: 'subnets', label: 'Subnets', icon: Layers, color: 'text-purple-400 bg-purple-500/20' },
  { type: 'internetgateways', label: 'Internet GWs', icon: Globe, color: 'text-orange-400 bg-orange-500/20' },
  { type: 'natgateways', label: 'NAT GWs', icon: Router, color: 'text-amber-400 bg-amber-500/20' },
  { type: 'loadbalancers', label: 'Load Balancers', icon: ArrowUpDown, color: 'text-yellow-400 bg-yellow-500/20' },
  { type: 'firewalls', label: 'Firewalls', icon: Shield, color: 'text-red-400 bg-red-500/20' },
  { type: 'tunnels', label: 'Tunnels', icon: GitBranch, color: 'text-teal-400 bg-teal-500/20' },
  { type: 'bonds', label: 'Bonds', icon: Link2, color: 'text-cyan-400 bg-cyan-500/20' },
  { type: 'networkinterfaces', label: 'NICs', icon: Cable, color: 'text-indigo-400 bg-indigo-500/20' },
  { type: 'virtualnetworkfunctions', label: 'VNFs', icon: Server, color: 'text-green-400 bg-green-500/20' },
  { type: 'servicechains', label: 'Service Chains', icon: Cpu, color: 'text-pink-400 bg-pink-500/20' },
];

interface ResourceCount {
  type: string;
  label: string;
  icon: LucideIcon;
  color: string;
  count: number;
  ready: number;
  failed: number;
}

export default function Dashboard() {
  const [counts, setCounts] = useState<ResourceCount[]>([]);
  const [recentResources, setRecentResources] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchAll = useCallback(async () => {
    setLoading(true);
    try {
      const results = await Promise.allSettled(
        resourceTypes.map(async (rt) => {
          const items = await listResources(rt.type);
          return { ...rt, items };
        })
      );

      const allCounts: ResourceCount[] = [];
      const allRecent: any[] = [];

      for (const result of results) {
        if (result.status === 'fulfilled') {
          const { items, ...info } = result.value;
          const ready = items.filter(
            (i: any) =>
              i.status?.phase === 'Ready' ||
              i.status?.phase === 'Active' ||
              i.status?.phase === 'Running' ||
              i.status?.phase === 'Established'
          ).length;
          const failed = items.filter(
            (i: any) => i.status?.phase === 'Failed' || i.status?.phase === 'Error'
          ).length;
          allCounts.push({ ...info, count: items.length, ready, failed });
          for (const item of items) {
            allRecent.push({ ...item, _resourceType: info.type });
          }
        }
      }

      setCounts(allCounts);
      setRecentResources(
        allRecent
          .sort(
            (a, b) =>
              new Date(b.metadata?.creationTimestamp ?? 0).getTime() -
              new Date(a.metadata?.creationTimestamp ?? 0).getTime()
          )
          .slice(0, 10)
      );
    } catch {
      // silently handle
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const totalResources = counts.reduce((s, c) => s + c.count, 0);
  const totalReady = counts.reduce((s, c) => s + c.ready, 0);
  const totalFailed = counts.reduce((s, c) => s + c.failed, 0);

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-12 w-12 animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-100">Dashboard</h1>
        <p className="text-sm text-gray-400">SDN Controller overview</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-gray-700 bg-gray-800 p-6">
          <p className="text-sm font-medium text-gray-400">Total Resources</p>
          <p className="mt-2 text-3xl font-bold text-gray-100">{totalResources}</p>
        </div>
        <div className="rounded-xl border border-gray-700 bg-gray-800 p-6">
          <p className="text-sm font-medium text-gray-400">Healthy</p>
          <p className="mt-2 text-3xl font-bold text-green-400">{totalReady}</p>
        </div>
        <div className="rounded-xl border border-gray-700 bg-gray-800 p-6">
          <p className="text-sm font-medium text-gray-400">Failed</p>
          <p className="mt-2 text-3xl font-bold text-red-400">{totalFailed}</p>
        </div>
      </div>

      {/* Resource type cards */}
      <div>
        <h2 className="mb-4 text-lg font-semibold text-gray-100">Resources</h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
          {counts.map((rc) => (
            <div
              key={rc.type}
              className="rounded-xl border border-gray-700 bg-gray-800 p-4 transition-colors hover:border-gray-600"
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-lg p-2 ${rc.color}`}>
                  <rc.icon className="h-5 w-5" />
                </div>
                <div>
                  <p className="text-2xl font-bold text-gray-100">{rc.count}</p>
                  <p className="text-xs text-gray-400">{rc.label}</p>
                </div>
              </div>
              {rc.count > 0 && (
                <div className="mt-3 flex gap-2 text-xs">
                  <span className="text-green-400">{rc.ready} ready</span>
                  {rc.failed > 0 && (
                    <span className="text-red-400">{rc.failed} failed</span>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Recent resources */}
      <div>
        <h2 className="mb-4 text-lg font-semibold text-gray-100">
          Recent Resources
        </h2>
        <div className="overflow-hidden rounded-xl border border-gray-700 bg-gray-800">
          {recentResources.length === 0 ? (
            <div className="py-8 text-center text-gray-400">
              No resources found
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-gray-700 bg-gray-800/50">
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-400">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-400">
                      Type
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-400">
                      Namespace
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-400">
                      Status
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-700">
                  {recentResources.map((r, i) => (
                    <tr key={i} className="transition-colors hover:bg-gray-700/50">
                      <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-100">
                        {r.metadata?.name ?? '-'}
                      </td>
                      <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-300">
                        {r.kind ?? r._resourceType}
                      </td>
                      <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-300">
                        {r.metadata?.namespace ?? '-'}
                      </td>
                      <td className="whitespace-nowrap px-6 py-4 text-sm">
                        <StatusBadge phase={r.status?.phase} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
