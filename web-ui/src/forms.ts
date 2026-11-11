export type FieldType =
  | 'text'
  | 'number'
  | 'select'
  | 'multiselect'
  | 'checkbox'
  | 'strings'
  | 'selector'
  | 'rows';

export interface FieldDef {
  path: string;
  label: string;
  type: FieldType;
  options?: string[];
  optionsFrom?: 'vpcs' | 'subnets' | 'vnfs';
  required?: boolean | ((o: any) => boolean);
  showIf?: (o: any) => boolean;
  stripWhenHidden?: boolean;
  placeholder?: string;
  help?: string;
  rows?: FieldDef[];
  newRow?: () => any;
  readOnlyOnEdit?: boolean;
}

const nodeSelector: FieldDef = {
  path: 'spec.nodeSelector',
  label: 'Node selector (key=value, comma separated)',
  type: 'selector',
  placeholder: 'role=gateway, zone=a',
};
const vpcRef: FieldDef = { path: 'spec.vpcRef', label: 'VPC', type: 'select', optionsFrom: 'vpcs', required: true };
const TUNNELS = ['VXLAN', 'IPIP', 'GRE'];
const needsVNI = (o: any) => (o?.spec?.tunnelType ?? 'VXLAN') !== 'IPIP';

export const FORMS: Record<string, { fields: FieldDef[]; defaults?: any }> = {
  vpcs: {
    defaults: { spec: { cidr: '10.0.0.0/16', tunnelType: 'VXLAN', mtu: 1450 } },
    fields: [
      { path: 'spec.cidr', label: 'CIDR', type: 'text', required: true, placeholder: '10.0.0.0/16' },
      { path: 'spec.secondaryCIDRs', label: 'Secondary CIDRs', type: 'strings', placeholder: '10.1.0.0/16, ...' },
      { path: 'spec.tunnelType', label: 'Tunnel type', type: 'select', options: TUNNELS, required: true },
      {
        path: 'spec.vni',
        label: 'VNI',
        type: 'number',
        required: needsVNI,
        showIf: needsVNI,
        stripWhenHidden: true,
        help: 'Required for VXLAN and GRE; not used by IPIP.',
      },
      { path: 'spec.mtu', label: 'MTU', type: 'number', placeholder: '1450' },
      { path: 'spec.enableDNS', label: 'Enable DNS', type: 'checkbox' },
      nodeSelector,
    ],
  },
  subnets: {
    defaults: { spec: { type: 'Private' } },
    fields: [
      vpcRef,
      { path: 'spec.cidr', label: 'CIDR', type: 'text', required: true, placeholder: '10.0.1.0/24' },
      { path: 'spec.gateway', label: 'Gateway', type: 'text', placeholder: '10.0.1.1' },
      { path: 'spec.type', label: 'Type', type: 'select', options: ['Private', 'Public'] },
      { path: 'spec.vlanID', label: 'VLAN ID', type: 'number' },
      { path: 'spec.mtu', label: 'MTU', type: 'number' },
      { path: 'spec.dhcp.enabled', label: 'DHCP enabled', type: 'checkbox' },
      nodeSelector,
    ],
  },
  internetgateways: {
    fields: [
      vpcRef,
      { path: 'spec.externalInterface', label: 'External interface', type: 'text', required: true, placeholder: 'eth1' },
      { path: 'spec.externalIPs', label: 'External IPs', type: 'strings', required: true, placeholder: '203.0.113.10, ...' },
      { path: 'spec.ha.enabled', label: 'HA enabled', type: 'checkbox' },
      { path: 'spec.ha.mode', label: 'HA mode', type: 'select', options: ['ActiveStandby', 'ActiveActive'], showIf: (o) => !!o?.spec?.ha?.enabled },
      nodeSelector,
    ],
  },
  natgateways: {
    defaults: { spec: { type: 'SNAT' } },
    fields: [
      vpcRef,
      { path: 'spec.subnetRefs', label: 'Subnets', type: 'multiselect', optionsFrom: 'subnets', required: true },
      { path: 'spec.externalIP', label: 'External IP', type: 'text', required: true, placeholder: '203.0.113.20' },
      { path: 'spec.type', label: 'NAT type', type: 'select', options: ['SNAT', 'DNAT', 'FullNAT'] },
      { path: 'spec.portRange.start', label: 'Port range start', type: 'number' },
      { path: 'spec.portRange.end', label: 'Port range end', type: 'number' },
      { path: 'spec.ha.enabled', label: 'HA enabled', type: 'checkbox' },
      { path: 'spec.ha.mode', label: 'HA mode', type: 'select', options: ['ActiveStandby', 'ActiveActive'], showIf: (o) => !!o?.spec?.ha?.enabled },
      nodeSelector,
    ],
  },
  loadbalancers: {
    defaults: {
      spec: {
        type: 'L4',
        algorithm: 'RoundRobin',
        frontend: { ip: '', ports: [{ protocol: 'TCP', port: 80, targetPort: 8080 }] },
        backends: [{ ip: '', weight: 100 }],
      },
    },
    fields: [
      vpcRef,
      { path: 'spec.type', label: 'Type', type: 'select', options: ['L4', 'L7'] },
      { path: 'spec.algorithm', label: 'Algorithm', type: 'select', options: ['RoundRobin', 'LeastConn', 'IPHash', 'Maglev'] },
      { path: 'spec.frontend.ip', label: 'Frontend VIP', type: 'text', required: true, placeholder: '10.0.1.100' },
      {
        path: 'spec.frontend.ports',
        label: 'Frontend ports',
        type: 'rows',
        newRow: () => ({ protocol: 'TCP', port: 80, targetPort: 80 }),
        rows: [
          { path: 'protocol', label: 'Protocol', type: 'select', options: ['TCP', 'UDP'], required: true },
          { path: 'port', label: 'Port', type: 'number', required: true },
          { path: 'targetPort', label: 'Target port', type: 'number', required: true },
        ],
      },
      {
        path: 'spec.backends',
        label: 'Backends',
        type: 'rows',
        newRow: () => ({ ip: '', weight: 100 }),
        rows: [
          { path: 'ip', label: 'IP', type: 'text', required: true, placeholder: '10.0.1.11' },
          { path: 'weight', label: 'Weight', type: 'number' },
        ],
      },
      nodeSelector,
    ],
  },
  firewalls: {
    defaults: { spec: { defaultAction: 'Deny', stateful: true, rules: [] } },
    fields: [
      vpcRef,
      { path: 'spec.subnetRefs', label: 'Subnets', type: 'multiselect', optionsFrom: 'subnets' },
      { path: 'spec.defaultAction', label: 'Default action', type: 'select', options: ['Allow', 'Deny', 'Log'] },
      { path: 'spec.stateful', label: 'Stateful', type: 'checkbox' },
      {
        path: 'spec.rules',
        label: 'Rules',
        type: 'rows',
        newRow: () => ({ name: '', priority: 100, direction: 'Ingress', action: 'Allow', match: { protocol: 'TCP' } }),
        rows: [
          { path: 'name', label: 'Name', type: 'text', required: true },
          { path: 'priority', label: 'Priority', type: 'number', required: true },
          { path: 'direction', label: 'Direction', type: 'select', options: ['Ingress', 'Egress'], required: true },
          { path: 'action', label: 'Action', type: 'select', options: ['Allow', 'Deny', 'Log'], required: true },
          { path: 'match.protocol', label: 'Protocol', type: 'text', placeholder: 'TCP' },
          { path: 'match.srcCIDR', label: 'Src CIDR', type: 'text' },
          { path: 'match.dstCIDR', label: 'Dst CIDR', type: 'text' },
          { path: 'match.srcPort', label: 'Src port', type: 'text' },
          { path: 'match.dstPort', label: 'Dst port', type: 'text', placeholder: '80 or 8000-9000' },
          { path: 'logging', label: 'Logging', type: 'checkbox' },
        ],
      },
      nodeSelector,
    ],
  },
  servicechains: {
    defaults: { spec: { direction: 'Ingress', chain: [] } },
    fields: [
      vpcRef,
      { path: 'spec.subnetRef', label: 'Subnet', type: 'select', optionsFrom: 'subnets' },
      { path: 'spec.direction', label: 'Direction', type: 'select', options: ['Ingress', 'Egress', 'Both'] },
      {
        path: 'spec.chain',
        label: 'Chain (ordered hops)',
        type: 'rows',
        newRow: () => ({ name: '', vnfRef: '', portPair: { ingress: 'eth0', egress: 'eth1' } }),
        rows: [
          { path: 'name', label: 'Hop name', type: 'text', required: true },
          { path: 'vnfRef', label: 'VNF', type: 'select', optionsFrom: 'vnfs', required: true },
          { path: 'portPair.ingress', label: 'Ingress port', type: 'text', required: true },
          { path: 'portPair.egress', label: 'Egress port', type: 'text', required: true },
        ],
      },
      { path: 'spec.match.protocol', label: 'Match protocol', type: 'text' },
      { path: 'spec.match.srcCIDR', label: 'Match src CIDR', type: 'text' },
      { path: 'spec.match.dstCIDR', label: 'Match dst CIDR', type: 'text' },
      { path: 'spec.offload.enabled', label: 'Offload enabled', type: 'checkbox' },
      nodeSelector,
    ],
  },
  virtualnetworkfunctions: {
    defaults: { spec: { type: 'Container', replicas: 1, interfaces: [{ name: 'eth0', type: 'OVSPort' }] } },
    fields: [
      { path: 'spec.type', label: 'Type', type: 'select', options: ['Container', 'VM'] },
      { path: 'spec.image', label: 'Image', type: 'text', showIf: (o) => o?.spec?.type !== 'VM' },
      { path: 'spec.vmConfig.image', label: 'VM image', type: 'text', showIf: (o) => o?.spec?.type === 'VM' },
      { path: 'spec.vmConfig.vcpus', label: 'VM vCPUs', type: 'number', showIf: (o) => o?.spec?.type === 'VM' },
      { path: 'spec.vmConfig.memoryMB', label: 'VM memory (MB)', type: 'number', showIf: (o) => o?.spec?.type === 'VM' },
      { path: 'spec.replicas', label: 'Replicas', type: 'number' },
      {
        path: 'spec.interfaces',
        label: 'Interfaces',
        type: 'rows',
        newRow: () => ({ name: '', type: 'OVSPort' }),
        rows: [
          { path: 'name', label: 'Name', type: 'text', required: true },
          { path: 'type', label: 'Type', type: 'select', options: ['OVSPort', 'SRIOV', 'Tap'] },
        ],
      },
      { path: 'spec.resources.cpu', label: 'CPU', type: 'text', placeholder: '500m' },
      { path: 'spec.resources.memory', label: 'Memory', type: 'text', placeholder: '256Mi' },
      nodeSelector,
    ],
  },
  tunnels: {
    defaults: { spec: { type: 'VXLAN', localEndpoint: { nodeName: '', ip: '' }, remoteEndpoint: { nodeName: '', ip: '' } } },
    fields: [
      { path: 'spec.type', label: 'Type', type: 'select', options: TUNNELS },
      { path: 'spec.vni', label: 'VNI', type: 'number', showIf: (o) => (o?.spec?.type ?? 'VXLAN') !== 'IPIP', stripWhenHidden: true },
      { path: 'spec.localEndpoint.nodeName', label: 'Local node', type: 'text', required: true },
      { path: 'spec.localEndpoint.ip', label: 'Local IP', type: 'text', required: true },
      { path: 'spec.remoteEndpoint.nodeName', label: 'Remote node', type: 'text', required: true },
      { path: 'spec.remoteEndpoint.ip', label: 'Remote IP', type: 'text', required: true },
      { path: 'spec.mtu', label: 'MTU', type: 'number' },
      { path: 'spec.checksumEnabled', label: 'Checksum enabled', type: 'checkbox' },
    ],
  },
  bonds: {
    defaults: { spec: { mode: 'active-backup', interfaces: [] } },
    fields: [
      { path: 'spec.nodeName', label: 'Node name', type: 'text', required: true },
      { path: 'spec.mode', label: 'Mode', type: 'select', options: ['balance-rr', 'active-backup', '802.3ad', 'balance-xor', 'balance-tlb', 'balance-alb'] },
      { path: 'spec.interfaces', label: 'Member interfaces', type: 'strings', required: true, placeholder: 'eth1, eth2' },
      { path: 'spec.mtu', label: 'MTU', type: 'number' },
      { path: 'spec.lacpRate', label: 'LACP rate', type: 'select', options: ['slow', 'fast'], showIf: (o) => o?.spec?.mode === '802.3ad' },
      { path: 'spec.hashPolicy', label: 'Hash policy', type: 'select', options: ['layer2', 'layer2+3', 'layer3+4'] },
      { path: 'spec.addresses', label: 'Addresses', type: 'strings' },
      { path: 'spec.subnetRef', label: 'Subnet', type: 'select', optionsFrom: 'subnets' },
    ],
  },
  networkinterfaces: {
    defaults: { spec: { type: 'Physical' } },
    fields: [
      { path: 'spec.nodeName', label: 'Node name', type: 'text', required: true },
      { path: 'spec.type', label: 'Type', type: 'select', options: ['Physical', 'Virtual', 'VF', 'Bond'] },
      { path: 'spec.device', label: 'Device', type: 'text', required: true },
      { path: 'spec.mtu', label: 'MTU', type: 'number' },
      { path: 'spec.addresses', label: 'Addresses', type: 'strings' },
      { path: 'spec.vpcRef', label: 'VPC', type: 'select', optionsFrom: 'vpcs' },
      { path: 'spec.subnetRef', label: 'Subnet', type: 'select', optionsFrom: 'subnets' },
      { path: 'spec.dpuManaged', label: 'DPU managed', type: 'checkbox' },
    ],
  },
  nodenetworkconfigs: {
    defaults: { spec: { nodeType: 'BareMetal', computeType: 'CPU', tunnelProtocol: 'VXLAN' } },
    fields: [
      { path: 'spec.nodeName', label: 'Node name', type: 'text', required: true },
      { path: 'spec.nodeType', label: 'Node type', type: 'select', options: ['BareMetal', 'VM'] },
      { path: 'spec.computeType', label: 'Compute type', type: 'select', options: ['CPU', 'GPU'] },
      { path: 'spec.tunnelProtocol', label: 'Tunnel protocol', type: 'select', options: TUNNELS },
      { path: 'spec.tunnelSourceIP', label: 'Tunnel source IP', type: 'text' },
      { path: 'spec.defaultMTU', label: 'Default MTU', type: 'number' },
      { path: 'spec.dpuEnabled', label: 'DPU enabled', type: 'checkbox' },
      {
        path: 'spec.ovsBridges',
        label: 'OVS bridges',
        type: 'rows',
        newRow: () => ({ name: '', type: 'integration' }),
        rows: [
          { path: 'name', label: 'Name', type: 'text', required: true },
          { path: 'type', label: 'Type', type: 'select', options: ['integration', 'external', 'tunnel'], required: true },
          { path: 'datapathType', label: 'Datapath', type: 'select', options: ['system', 'netdev'] },
          { path: 'uplinks', label: 'Uplinks', type: 'strings' },
        ],
      },
    ],
  },
};
