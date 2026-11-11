import { NavLink } from 'react-router-dom';
import {
  Activity, BarChart3, Cable, HardDrive, ExternalLink, GitBranch, Globe, Layers, LayoutDashboard, Map, Network, Server, Shield,
} from 'lucide-react';
import { GRAFANA_URL, nodePath } from '../resources';
import { getTopology, listNodes } from '../api';
import { useData } from '../context';
import { hostedByNode } from '../placement';

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/vpcs', label: 'VPCs', icon: Network },
  { to: '/subnets', label: 'Subnets', icon: Layers },
  { to: '/vnics', label: 'VNICs', icon: Cable },
  { to: '/gateways', label: 'Gateways', icon: Globe },
  { to: '/services', label: 'Services', icon: Shield },
  { to: '/nfv', label: 'NFV', icon: Server },
  { to: '/infra', label: 'Infrastructure', icon: GitBranch },
  { to: '/nodes', label: 'Nodes', icon: HardDrive },
  { to: '/topology', label: 'Topology', icon: Map },
  { to: '/metrics', label: 'Metrics', icon: Activity },
];

function NodeSubList() {
  const nodes = useData(() => listNodes().catch(() => []));
  const topo = useData(() => getTopology().catch(() => undefined));
  const hosted = hostedByNode(topo.data);
  if (!nodes.data?.length) return null;
  return (
    <ul className="mb-1 ml-5 mt-1 space-y-0.5 border-l border-gray-200 pl-3 dark:border-gray-700">
      {nodes.data.map((n) => (
        <li key={n.name}>
          <NavLink
            to={nodePath(n.name)}
            className={({ isActive }) =>
              `flex items-center gap-2 rounded px-2 py-1 text-xs transition-colors ${isActive ? 'text-blue-700 dark:text-blue-400' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'}`
            }
          >
            <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${n.ready ? 'bg-green-500' : 'bg-yellow-500'}`} />
            <span className="truncate">{n.name}</span>
            <span className="ml-auto rounded-full bg-gray-100 px-1.5 text-[10px] dark:bg-gray-800" title="hosted services & VNICs">{hosted.get(n.name)?.length ?? 0}</span>
          </NavLink>
        </li>
      ))}
    </ul>
  );
}

export default function Sidebar() {
  return (
    <aside className="flex h-screen w-60 shrink-0 flex-col border-r border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
      <div className="flex items-center gap-3 border-b border-gray-200 px-5 py-4 dark:border-gray-800">
        <Network className="h-8 w-8 text-blue-500" />
        <div>
          <h1 className="text-lg font-bold text-gray-900 dark:text-gray-100">SDN Controller</h1>
          <p className="text-xs text-gray-500 dark:text-gray-400">Management Console</p>
        </div>
      </div>

      <nav className="flex-1 overflow-y-auto px-3 py-4">
        <ul className="space-y-1">
          {navItems.map((item) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400'
                      : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100'
                  }`
                }
              >
                <item.icon className="h-5 w-5 shrink-0" />
                {item.label}
              </NavLink>
              {item.to === '/nodes' && <NodeSubList />}
            </li>
          ))}
          <li>
            <a
              href={GRAFANA_URL}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
            >
              <BarChart3 className="h-5 w-5 shrink-0" />
              Grafana
              <ExternalLink className="ml-auto h-3.5 w-3.5" />
            </a>
          </li>
        </ul>
      </nav>

      <div className="border-t border-gray-200 px-5 py-3 dark:border-gray-800">
        <p className="text-xs text-gray-400">SDN Controller v1.0</p>
      </div>
    </aside>
  );
}
