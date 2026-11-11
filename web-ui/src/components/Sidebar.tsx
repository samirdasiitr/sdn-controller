import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Network,
  Globe,
  Shield,
  Server,
  GitBranch,
  Map,
  Layers,
} from 'lucide-react';

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/vpcs', label: 'VPCs', icon: Network },
  { to: '/subnets', label: 'Subnets', icon: Layers },
  { to: '/gateways', label: 'Gateways', icon: Globe },
  { to: '/services', label: 'Services', icon: Shield },
  { to: '/nfv', label: 'NFV', icon: Server },
  { to: '/infra', label: 'Infrastructure', icon: GitBranch },
  { to: '/topology', label: 'Topology', icon: Map },
];

export default function Sidebar() {
  return (
    <aside className="flex h-screen w-64 flex-col bg-gray-900 border-r border-gray-700">
      <div className="flex items-center gap-3 px-6 py-5 border-b border-gray-700">
        <Network className="h-8 w-8 text-blue-500" />
        <div>
          <h1 className="text-lg font-bold text-gray-100">SDN Controller</h1>
          <p className="text-xs text-gray-400">Management Console</p>
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
                      ? 'bg-blue-500/20 text-blue-400'
                      : 'text-gray-400 hover:bg-gray-800 hover:text-gray-100'
                  }`
                }
              >
                <item.icon className="h-5 w-5 shrink-0" />
                {item.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>

      <div className="border-t border-gray-700 px-6 py-4">
        <p className="text-xs text-gray-500">SDN Controller v1.0</p>
      </div>
    </aside>
  );
}
