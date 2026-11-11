import { getResource, updateResource } from './api';
import { RESOURCES } from './resources';

export const TAG_PREFIX = 'tags.sdncontroller.io/';

export interface Tag {
  key: string;
  value: string;
}

export const keyRe = /^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$/;
export const valueRe = /^[A-Za-z0-9_.-]*$/;

export function validateTagKey(k: string): string | null {
  if (!k) return 'key is required';
  if (k.length > 63) return 'key must be ≤ 63 chars';
  if (!keyRe.test(k)) return 'key must match [A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?';
  return null;
}

export function validateTagValue(v: string): string | null {
  if (v.length > 63) return 'value must be ≤ 63 chars';
  if (!valueRe.test(v)) return 'value may only contain [A-Za-z0-9_.-]';
  return null;
}

/** Tags stored on an object as labels under tags.sdncontroller.io/. */
export function tagsOf(obj: any): Tag[] {
  const labels = obj?.metadata?.labels ?? {};
  return Object.entries(labels)
    .filter(([k]) => k.startsWith(TAG_PREFIX))
    .map(([k, v]) => ({ key: k.slice(TAG_PREFIX.length), value: v as string }))
    .sort((a, b) => a.key.localeCompare(b.key) || a.value.localeCompare(b.value));
}

/** Return a new object with tag labels replaced; all non-tag labels preserved. */
export function withTags(obj: any, tags: Tag[]): any {
  const out = JSON.parse(JSON.stringify(obj ?? {}));
  const labels: Record<string, string> = { ...(out.metadata?.labels ?? {}) };
  for (const k of Object.keys(labels)) if (k.startsWith(TAG_PREFIX)) delete labels[k];
  for (const t of tags) labels[TAG_PREFIX + t.key] = t.value;
  out.metadata = { ...(out.metadata ?? {}), labels };
  if (Object.keys(labels).length === 0) delete out.metadata.labels;
  return out;
}

export const tagStr = (t: Tag) => `${t.key}=${t.value}`;

export function parseTagParam(s: string): Tag | null {
  const i = s.indexOf('=');
  if (i <= 0) return null;
  const key = s.slice(0, i);
  const value = s.slice(i + 1);
  if (validateTagKey(key) || validateTagValue(value)) return null;
  return { key, value };
}

export function hasTag(obj: any, t: Tag): boolean {
  return obj?.metadata?.labels?.[TAG_PREFIX + t.key] === t.value;
}

/** Merge tag labels into an object (adds/updates given tags, removes tag keys absent from `tags`). */
export function mergeTagLabels(existing: Record<string, string> | undefined, tags: Tag[]): Record<string, string> {
  const labels = { ...(existing ?? {}) };
  for (const k of Object.keys(labels)) if (k.startsWith(TAG_PREFIX)) delete labels[k];
  for (const t of tags) labels[TAG_PREFIX + t.key] = t.value;
  return labels;
}

/**
 * Update only metadata.labels on the server, preserving everything else.
 * Retries once on conflict by refetching.
 */
export async function updateTags(resource: string, ns: string, name: string, tags: Tag[]): Promise<any> {
  for (let attempt = 0; attempt < 2; attempt++) {
    const cur = await getResource(resource, ns, name);
    const next = JSON.parse(JSON.stringify(cur));
    next.metadata.labels = mergeTagLabels(cur.metadata?.labels, tags);
    try {
      return await updateResource(resource, ns, name, next);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (attempt === 0 && /conflict|modified/i.test(msg)) continue; // refetch & retry once
      throw e;
    }
  }
}

/* ---------------- /api/tags ---------------- */

export interface TagResourceRef {
  resource: string;
  kind: string;
  namespace: string;
  name: string;
}
export interface TagEntry {
  key: string;
  value: string;
  count: number;
  resources: TagResourceRef[];
}

export async function listTags(): Promise<TagEntry[]> {
  const res = await fetch('/api/tags');
  if (!res.ok) return [];
  return res.json();
}

/** suggestions: unique key=value pairs from the tags API (fallback: scanning resources). */
export async function tagSuggestions(): Promise<Tag[]> {
  try {
    const entries = await listTags();
    return entries.map((e) => ({ key: e.key, value: e.value }));
  } catch {
    return [];
  }
}

export const resourceLabel = (r: string) => RESOURCES[r]?.label ?? r;
