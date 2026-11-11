const API = '/api';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`API ${res.status}: ${text}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function listResourceTypes(): Promise<string[]> {
  return request<string[]>(`${API}/resources`);
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

export interface TopologyData {
  nodes: any[];
  edges: any[];
}

export async function getTopology(): Promise<TopologyData> {
  return request<TopologyData>(`${API}/topology`);
}

export async function listNamespaces(): Promise<string[]> {
  return request<string[]>(`${API}/namespaces`);
}
