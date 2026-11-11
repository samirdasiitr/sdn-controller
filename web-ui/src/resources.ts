export interface ResourceMeta {
  resource: string; // plural, used in API paths
  kind: string;
  apiVersion: string;
  label: string;
}

const g = (group: string) => `${group}.sdncontroller.io/v1alpha1`;

export const RESOURCES: Record<string, ResourceMeta> = {
  vpcs: { resource: 'vpcs', kind: 'VPC', apiVersion: g('vpc'), label: 'VPC' },
  subnets: { resource: 'subnets', kind: 'Subnet', apiVersion: g('vpc'), label: 'Subnet' },
  internetgateways: { resource: 'internetgateways', kind: 'InternetGateway', apiVersion: g('gateway'), label: 'Internet Gateway' },
  natgateways: { resource: 'natgateways', kind: 'NATGateway', apiVersion: g('gateway'), label: 'NAT Gateway' },
  loadbalancers: { resource: 'loadbalancers', kind: 'LoadBalancer', apiVersion: g('service'), label: 'Load Balancer' },
  firewalls: { resource: 'firewalls', kind: 'Firewall', apiVersion: g('service'), label: 'Firewall' },
  vnics: { resource: 'vnics', kind: 'VNic', apiVersion: g('vpc'), label: 'VNIC' },
  servicechains: { resource: 'servicechains', kind: 'ServiceChain', apiVersion: g('nfv'), label: 'Service Chain' },
  virtualnetworkfunctions: { resource: 'virtualnetworkfunctions', kind: 'VirtualNetworkFunction', apiVersion: g('nfv'), label: 'VNF' },
  tunnels: { resource: 'tunnels', kind: 'Tunnel', apiVersion: g('infra'), label: 'Tunnel' },
  bonds: { resource: 'bonds', kind: 'Bond', apiVersion: g('infra'), label: 'Bond' },
  networkinterfaces: { resource: 'networkinterfaces', kind: 'NetworkInterface', apiVersion: g('infra'), label: 'Network Interface' },
  nodenetworkconfigs: { resource: 'nodenetworkconfigs', kind: 'NodeNetworkConfig', apiVersion: g('infra'), label: 'Node Network Config' },
};

export const KIND_TO_RESOURCE: Record<string, string> = Object.fromEntries(
  Object.values(RESOURCES).map((r) => [r.kind, r.resource]),
);

export const detailPath = (resource: string, ns: string, name: string) =>
  `/${resource}/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`;

export const nodePath = (name: string) => `/nodes/${encodeURIComponent(name)}`;

export const GRAFANA_URL = 'http://localhost:3000';
