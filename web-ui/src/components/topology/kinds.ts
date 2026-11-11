import {
  ArrowUpDown, Cable, Cpu, GitBranch, Globe, Layers, Link2, Network, Plug, Router, Server, Settings2, Shield, type LucideIcon,
} from 'lucide-react';

export const KIND_STYLE: Record<string, { color: string; icon: LucideIcon; label: string }> = {
  vpcs: { color: '#3b82f6', icon: Network, label: 'VPC' },
  subnets: { color: '#8b5cf6', icon: Layers, label: 'Subnet' },
  internetgateways: { color: '#f97316', icon: Globe, label: 'Internet GW' },
  natgateways: { color: '#f59e0b', icon: Router, label: 'NAT GW' },
  loadbalancers: { color: '#eab308', icon: ArrowUpDown, label: 'Load Balancer' },
  firewalls: { color: '#ef4444', icon: Shield, label: 'Firewall' },
  vnics: { color: '#d946ef', icon: Plug, label: 'VNIC' },
  servicechains: { color: '#ec4899', icon: Cpu, label: 'Service Chain' },
  virtualnetworkfunctions: { color: '#22c55e', icon: Server, label: 'VNF' },
  tunnels: { color: '#14b8a6', icon: GitBranch, label: 'Tunnel' },
  bonds: { color: '#06b6d4', icon: Link2, label: 'Bond' },
  networkinterfaces: { color: '#6366f1', icon: Cable, label: 'NIC' },
  nodenetworkconfigs: { color: '#64748b', icon: Settings2, label: 'Node Config' },
  nodes: { color: '#0ea5e9', icon: Server, label: 'Node' },
};

export const EDGE_COLORS: Record<string, string> = {
  belongs_to: '#3b82f6',
  attached_to: '#8b5cf6',
  uses_vnf: '#ec4899',
  tunnel_endpoint: '#14b8a6',
  active_on: '#f97316',
  attached_node: '#0ea5e9',
  runs_on: '#0ea5e9',
  standby_on: '#0ea5e9',
  protected_by: '#ef4444',
};

export function facts(n: { resource: string; spec?: any; status?: any }): string[] {
  const s = n.spec ?? {};
  const st = n.status ?? {};
  switch (n.resource) {
    case 'vpcs':
      return [s.cidr, [s.tunnelType, (s.vni ?? st.allocatedVNI) !== undefined ? `VNI ${s.vni ?? st.allocatedVNI}` : ''].filter(Boolean).join(' · ')].filter(Boolean);
    case 'subnets':
      return [s.cidr, s.gateway ? `gw ${s.gateway}` : ''].filter(Boolean);
    case 'internetgateways':
      return [(s.externalIPs ?? []).join(', '), st.activeNode ? `on ${st.activeNode}` : ''].filter(Boolean);
    case 'natgateways':
      return [`${s.externalIP ?? ''} ${s.type ?? ''}`.trim(), `${s.subnetRefs?.length ?? 0} subnet(s)`];
    case 'loadbalancers':
      return [`VIP ${s.frontend?.ip ?? '-'}${s.frontend?.ports?.[0] ? ':' + s.frontend.ports[0].port : ''}`, `${s.backends?.length ?? 0} backends · ${s.algorithm ?? ''}`];
    case 'firewalls':
      return [`${s.rules?.length ?? 0} rules`, `default ${s.defaultAction ?? '-'}`];
    case 'servicechains':
      return [`${s.chain?.length ?? 0} hops`, s.direction ?? ''];
    case 'virtualnetworkfunctions':
      return [s.type ?? '', `${st.readyReplicas ?? 0}/${s.replicas ?? 1} ready`];
    case 'vnics': {
      const att = s.attachment ? `${s.attachment.kind}/${s.attachment.name}` : '';
      return [st.ipAddress ?? s.ipAddress ?? '', att].filter(Boolean);
    }
    case 'nodes':
      return [s.internalIP ?? ''];
    default:
      return [s.nodeName ?? s.device ?? ''].filter(Boolean);
  }
}
