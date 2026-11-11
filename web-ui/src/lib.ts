export function getIn(obj: any, path: string): any {
  if (!path) return obj;
  return path.split('.').reduce((a, k) => (a == null ? undefined : a[k]), obj);
}

function clone<T>(o: T): T {
  return o === undefined ? o : JSON.parse(JSON.stringify(o));
}

/** Immutable set; removes the key when value is undefined, pruning emptied parents. */
export function setIn(obj: any, path: string, value: any): any {
  const keys = path.split('.');
  const root = clone(obj ?? {});
  const parents: any[] = [root];
  let cur = root;
  for (let i = 0; i < keys.length - 1; i++) {
    if (cur[keys[i]] == null || typeof cur[keys[i]] !== 'object') {
      if (value === undefined) return root;
      cur[keys[i]] = {};
    }
    cur = cur[keys[i]];
    parents.push(cur);
  }
  const last = keys[keys.length - 1];
  if (value === undefined) delete cur[last];
  else cur[last] = value;
  // prune empty parent objects (never the root, never metadata/spec)
  for (let i = keys.length - 1; i >= 1; i--) {
    const p = parents[i];
    if (p && !Array.isArray(p) && Object.keys(p).length === 0 && keys[i - 1] !== 'spec' && keys[i - 1] !== 'metadata') {
      delete parents[i - 1][keys[i - 1]];
    } else break;
  }
  return root;
}

export function age(ts?: string): string {
  if (!ts) return '-';
  const s = Math.max(0, (Date.now() - new Date(ts).getTime()) / 1000);
  if (s < 60) return `${Math.floor(s)}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

export function fmtBytes(n: number): string {
  if (!isFinite(n)) return '-';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (Math.abs(n) >= 1024 && i < u.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n >= 100 || i === 0 ? 0 : 1)} ${u[i]}`;
}

export function fmtNum(n: number): string {
  if (!isFinite(n)) return '-';
  if (Math.abs(n) >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (Math.abs(n) >= 1e4) return `${(n / 1e3).toFixed(1)}k`;
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}
