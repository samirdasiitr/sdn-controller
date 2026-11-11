const API = '/api';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

/** Turn raw k8s/API errors into something a human can act on. */
export function friendlyError(e: unknown): string {
  const msg = e instanceof Error ? e.message : String(e);
  if (/already exists/i.test(msg)) return `${msg} — a resource with that name already exists in this namespace.`;
  if (/in use|used by|usedBy|dependent/i.test(msg)) return `${msg} — it is still referenced by other resources; remove the dependents first.`;
  if (/denied the request|admission webhook/i.test(msg)) return `Rejected by validation: ${msg.replace(/^admission webhook "[^"]*" denied the request:\s*/i, '')}`;
  if (/conflict|the object has been modified/i.test(msg)) return `${msg} — the resource changed since it was loaded; refresh and retry.`;
  if (/Failed to fetch|NetworkError/i.test(msg)) return 'Cannot reach the API server.';
  return msg;
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    let msg = text;
    try {
      const j = JSON.parse(text);
      if (j?.error) msg = j.error;
    } catch {
      /* not json */
    }
    throw new ApiError(res.status, msg || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function listResources(type: string, namespace?: string): Promise<any[]> {
  const url = namespace
    ? `${API}/resources/${type}/${namespace}`
    : `${API}/resources/${type}`;
  const data = await request<any>(url);
  return Array.isArray(data) ? data : data?.items ?? [];
}

export async function getResource(type: string, namespace: string, name: string): Promise<any> {
  return request<any>(`${API}/resources/${type}/${namespace}/${name}`);
}

export async function createResource(type: string, namespace: string, obj: any): Promise<any> {
  return request<any>(`${API}/resources/${type}/${namespace}`, {
    method: 'POST',
    body: JSON.stringify(obj),
  });
}

export async function updateResource(type: string, namespace: string, name: string, obj: any): Promise<any> {
  return request<any>(`${API}/resources/${type}/${namespace}/${name}`, {
    method: 'PUT',
    body: JSON.stringify(obj),
  });
}

export async function deleteResource(type: string, namespace: string, name: string): Promise<void> {
  await request<void>(`${API}/resources/${type}/${namespace}/${name}`, {
    method: 'DELETE',
  });
}

export interface TopologyNode {
  id: string;
  name: string;
  namespace: string;
  kind: string;
  resource: string;
  spec?: any;
  status?: any;
  labels?: Record<string, string>;
  external?: boolean;
  deletionTimestamp?: string | null;
}
export interface TopologyEdge {
  id: string;
  source: string;
  target: string;
  type: string;
  label?: string;
}
export interface TopologyData {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
}

export async function getTopology(): Promise<TopologyData> {
  return request<TopologyData>(`${API}/topology`);
}

export async function listNamespaces(): Promise<string[]> {
  return request<string[]>(`${API}/namespaces`);
}

export interface K8sNode {
  name: string;
  internalIP: string;
  labels: Record<string, string>;
  ready: boolean;
  readyReason?: string;
  kubeletVersion?: string;
}

export async function listNodes(): Promise<K8sNode[]> {
  return request<K8sNode[]>(`${API}/nodes`);
}

/* ---------- Prometheus proxy ---------- */

export interface PromSample {
  metric: Record<string, string>;
  value?: [number, string];
  values?: [number, string][];
}

async function prom(path: string, params: Record<string, string>): Promise<PromSample[]> {
  const qs = new URLSearchParams(params).toString();
  const r = await request<{ status: string; data: { result: PromSample[] }; error?: string }>(
    `${API}/metrics/${path}?${qs}`,
  );
  if (r.status !== 'success') throw new Error(r.error ?? 'prometheus error');
  return r.data.result;
}

export function queryMetric(query: string): Promise<PromSample[]> {
  return prom('query', { query });
}

export function queryRange(query: string, rangeSec = 3600, step = 30): Promise<PromSample[]> {
  const end = Math.floor(Date.now() / 1000);
  return prom('query_range', {
    query,
    start: String(end - rangeSec),
    end: String(end),
    step: String(step),
  });
}

export interface HostedRef {
  kind: string;
  name: string;
  namespace: string;
  resource: string;
  role: string;
}
export interface K8sNodeDetail extends K8sNode {
  creationTimestamp?: string;
  hosted: HostedRef[];
}

export async function getNode(name: string): Promise<K8sNodeDetail> {
  return request<K8sNodeDetail>(`${API}/nodes/${encodeURIComponent(name)}`);
}

/* ---------- Node agent OVS proxies ---------- */

export interface OvsPortStats {
  rxBytes: number; txBytes: number; rxPackets: number; txPackets: number;
  rxDropped: number; txDropped: number; rxErrors: number; txErrors: number;
}
export interface OvsPort {
  name: string;
  ofport: number;
  type: string;
  role: 'internal' | 'tunnel' | 'workload' | 'uplink' | 'patch' | string;
  mac?: string;
  linkState?: string;
  mtu?: number;
  options?: Record<string, string>;
  remoteIP?: string;
  vni?: string;
  peer?: string;
  vnic?: string; // "ns/name"
  stats?: OvsPortStats;
}
export interface OvsBridgeInfo {
  name: string;
  datapathType?: string;
  externalIDs?: Record<string, string>;
  addresses?: string[];
  flows?: number;
  ports: OvsPort[];
}
export interface OvsUplink {
  name: string;
  mac?: string;
  mtu?: number;
  state?: string;
  addresses?: string[];
  defaultRoute?: boolean;
  physical?: boolean;
}
export interface OvsNat {
  vpc: string;
  source: string;
  uplink: string;
}
export interface NodeOvsTopology {
  node: string;
  time?: string;
  ovsVersion?: string;
  hwOffload?: boolean;
  uplinks: OvsUplink[];
  bridges: OvsBridgeInfo[];
  nat: OvsNat[];
}

export async function getNodeOvs(node: string): Promise<NodeOvsTopology> {
  return request<NodeOvsTopology>(`${API}/nodes/${encodeURIComponent(node)}/ovs`);
}

export interface OvsFlow {
  table: number;
  priority: number;
  cookie: string;
  match: string;
  actions: string;
  packets: number;
  bytes: number;
  duration: number;
  idleAge: number;
}
export interface OvsFlowDump {
  bridge: string;
  node?: string;
  flows: OvsFlow[];
}

export async function getNodeFlows(node: string, bridge: string): Promise<OvsFlowDump> {
  return request<OvsFlowDump>(
    `${API}/nodes/${encodeURIComponent(node)}/ovs/flows?bridge=${encodeURIComponent(bridge)}`,
  );
}
