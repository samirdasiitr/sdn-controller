import type { TopologyData } from './api';

/** Resource types that get a node placement from their controller. */
export const PLACED = ['loadbalancers', 'firewalls', 'virtualnetworkfunctions', 'servicechains'];
export const GATEWAYS = ['internetgateways', 'natgateways'];
export const SERVICE_KINDS = [...PLACED, ...GATEWAYS];

export interface Hosted {
  resource: string;
  kind: string;
  name: string;
  namespace: string;
  role: string;
}

/** Node names (and roles) a resource runs on. */
export function placementOf(resource: string, item: { spec?: any; status?: any }): { node: string; role: string }[] {
  const st = item.status ?? {};
  const out: { node: string; role: string }[] = [];
  if (GATEWAYS.includes(resource)) {
    if (st.activeNode) out.push({ node: st.activeNode, role: 'active' });
    if (st.standbyNode) out.push({ node: st.standbyNode, role: 'standby' });
  } else if (resource === 'vnics') {
    const n = st.boundNode || item.spec?.nodeName;
    if (n) out.push({ node: n, role: 'bound' });
  } else {
    for (const n of st.nodes ?? []) out.push({ node: n, role: 'placed' });
  }
  return out;
}

export const isPlaceable = (resource: string) => PLACED.includes(resource) || GATEWAYS.includes(resource);

/** Placed=False/NoEligibleNodes (or simply no nodes) message. */
export function placedCondition(item: any): any | undefined {
  return (item?.status?.conditions ?? []).find((c: any) => c.type === 'Placed');
}

/** node name -> hosted SDN resources (client-side mirror of GET /api/nodes/{name}.hosted for services & vnics). */
export function hostedByNode(topo: TopologyData | undefined): Map<string, Hosted[]> {
  const m = new Map<string, Hosted[]>();
  for (const n of topo?.nodes ?? []) {
    if (n.resource !== 'vnics' && !isPlaceable(n.resource)) continue;
    for (const p of placementOf(n.resource, n)) {
      const l = m.get(p.node) ?? [];
      l.push({ resource: n.resource, kind: n.kind, name: n.name, namespace: n.namespace, role: p.role });
      m.set(p.node, l);
    }
  }
  return m;
}

export function countByResource(h: Hosted[] | undefined): Record<string, number> {
  const c: Record<string, number> = {};
  for (const x of h ?? []) c[x.resource] = (c[x.resource] ?? 0) + 1;
  return c;
}
