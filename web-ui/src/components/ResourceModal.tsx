import { useEffect, useMemo, useState } from 'react';
import * as yaml from 'js-yaml';
import { Plus, Trash2, X } from 'lucide-react';
import {
  createResource,
  friendlyError,
  getResource,
  listNamespaces,
  listNodes,
  listResources,
  updateResource,
} from '../api';
import { FORMS, type FieldDef } from '../forms';
import { RESOURCES } from '../resources';
import { getIn, setIn } from '../lib';
import { withTags } from '../tags';
import { TagEditor } from './tags';
import { btnPrimary, btnSecondary, inputCls } from './ui';

interface Props {
  resource: string;
  /** when set, edit this object (name/namespace); otherwise create */
  target?: { namespace: string; name: string } | null;
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

type Ctx = { vpcs: string[]; subnets: string[]; vnfs: string[]; nodes: string[]; firewalls: string[]; subnetsByVpc?: Record<string, string[]> };

/* ---------- small controlled inputs with local text state ---------- */

function StringsInput({ value, onChange, placeholder }: { value?: string[]; onChange: (v: string[] | undefined) => void; placeholder?: string }) {
  const [text, setText] = useState((value ?? []).join(', '));
  return (
    <input
      className={inputCls}
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        setText(e.target.value);
        const arr = e.target.value.split(',').map((s) => s.trim()).filter(Boolean);
        onChange(arr.length ? arr : undefined);
      }}
    />
  );
}

function SelectorInput({ value, onChange, placeholder }: { value?: any; onChange: (v: any) => void; placeholder?: string }) {
  const toText = (v: any) =>
    Object.entries(v?.matchLabels ?? {}).map(([k, x]) => `${k}=${x}`).join(', ');
  const [text, setText] = useState(toText(value));
  return (
    <input
      className={inputCls}
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        setText(e.target.value);
        const ml: Record<string, string> = {};
        for (const part of e.target.value.split(',')) {
          const [k, ...rest] = part.split('=');
          if (k.trim() && rest.length) ml[k.trim()] = rest.join('=').trim();
        }
        const next = { ...(value ?? {}) };
        if (Object.keys(ml).length) next.matchLabels = ml;
        else delete next.matchLabels;
        onChange(Object.keys(next).length ? next : undefined);
      }}
    />
  );
}

function FieldInput({
  field,
  value,
  onChange,
  ctx,
  obj,
  disabled,
  rowCtx,
}: {
  field: FieldDef;
  value: any;
  onChange: (v: any) => void;
  ctx: Ctx;
  obj: any;
  disabled?: boolean;
  rowCtx?: any;
}) {
  const scope = rowCtx ?? obj;
  const required = typeof field.required === 'function' ? field.required(scope) : !!field.required;
  let options = field.options ?? [];
  if (field.optionsFrom) {
    options = ctx[field.optionsFrom];
    // subnets are filtered to the chosen VPC when known
    if (field.optionsFrom === 'subnets' && obj?.spec?.vpcRef) options = ctx.subnetsByVpc?.[obj.spec.vpcRef] ?? options;
  }
  switch (field.type) {
    case 'text':
      return (
        <input className={inputCls} value={value ?? ''} required={required} disabled={disabled} placeholder={field.placeholder}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)} />
      );
    case 'number':
      return (
        <input type="number" className={inputCls} value={value ?? ''} required={required} disabled={disabled} placeholder={field.placeholder}
          onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))} />
      );
    case 'select': {
      const opts = value && !options.includes(value) ? [value, ...options] : options;
      return (
        <select className={inputCls} value={value ?? ''} required={required} disabled={disabled}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}>
          {(!required || value === undefined) && <option value="">{required ? 'Select…' : '(none)'}</option>}
          {opts.map((o) => <option key={o} value={o}>{o}</option>)}
        </select>
      );
    }
    case 'multiselect': {
      const sel: string[] = value ?? [];
      const all = Array.from(new Set([...options, ...sel]));
      const toggle = (o: string) => {
        const next = sel.includes(o) ? sel.filter((x) => x !== o) : [...sel, o];
        onChange(next.length ? next : undefined);
      };
      return (
        <div className="flex flex-wrap gap-2">
          {all.length === 0 && <span className="text-xs text-gray-400">No options available</span>}
          {all.map((o) => (
            <label key={o} className={`cursor-pointer rounded-full border px-3 py-1 text-xs ${sel.includes(o) ? 'border-blue-500 bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-300' : 'border-gray-300 text-gray-600 dark:border-gray-600 dark:text-gray-400'}`}>
              <input type="checkbox" className="hidden" checked={sel.includes(o)} onChange={() => toggle(o)} />
              {o}
            </label>
          ))}
          {required && sel.length === 0 && <span className="text-xs text-red-500">select at least one</span>}
        </div>
      );
    }
    case 'checkbox':
      return (
        <input type="checkbox" className="h-4 w-4" checked={field.boolDefault !== undefined ? (value ?? field.boolDefault) : !!value} disabled={disabled}
          onChange={(e) => onChange(field.boolDefault !== undefined ? e.target.checked : e.target.checked ? true : undefined)} />
      );
    case 'strings':
      return <StringsInput value={value} onChange={onChange} placeholder={field.placeholder} />;
    case 'selector':
      return <SelectorInput value={value} onChange={onChange} placeholder={field.placeholder} />;
    case 'rows': {
      const rows: any[] = value ?? [];
      return (
        <div className="space-y-2">
          {rows.map((row, i) => (
            <div key={i} className="rounded-lg border border-gray-200 p-3 dark:border-gray-700">
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {field.rows!.filter((f) => !f.showIf || f.showIf(row)).map((f) => (
                  <div key={f.path}>
                    <div className="mb-0.5 text-[11px] text-gray-500 dark:text-gray-400">{f.label}</div>
                    <FieldInput field={f} value={getIn(row, f.path)} ctx={ctx} obj={obj} rowCtx={row}
                      onChange={(v) => {
                        const next = rows.slice();
                        next[i] = setIn(row, f.path, v);
                        onChange(next);
                      }} />
                  </div>
                ))}
              </div>
              <button type="button" className="mt-2 inline-flex items-center gap-1 text-xs text-red-500 hover:underline"
                onClick={() => { const next = rows.filter((_, j) => j !== i); onChange(next.length ? next : undefined); }}>
                <Trash2 className="h-3 w-3" /> Remove
              </button>
            </div>
          ))}
          <button type="button" className={btnSecondary}
            onClick={() => onChange([...rows, field.newRow ? field.newRow() : {}])}>
            <Plus className="h-4 w-4" /> Add
          </button>
        </div>
      );
    }
  }
}


function stripForYaml(obj: any) {
  const o = JSON.parse(JSON.stringify(obj));
  delete o.status;
  if (o.metadata) {
    delete o.metadata.managedFields;
  }
  return o;
}

export default function ResourceModal({ resource, target, open, onClose, onSaved }: Props) {
  const meta = RESOURCES[resource];
  const form = FORMS[resource] ?? { fields: [] };
  const edit = !!target;
  const [obj, setObj] = useState<any>(null);
  const [fetched, setFetched] = useState<any>(null);
  const [tab, setTab] = useState<'form' | 'yaml'>('form');
  const [yamlText, setYamlText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [lists, setLists] = useState<{ vpcs: any[]; subnets: any[]; vnfs: any[]; firewalls: any[]; nodes: string[] }>({ vpcs: [], subnets: [], vnfs: [], firewalls: [], nodes: [] });
  const [resetKey, setResetKey] = useState(0);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setTab('form');
    setResetKey((k) => k + 1);
    listNamespaces().then(setNamespaces).catch(() => setNamespaces(['default']));
    Promise.all([
      listResources('vpcs').catch(() => []),
      listResources('subnets').catch(() => []),
      listResources('virtualnetworkfunctions').catch(() => []),
      listResources('firewalls').catch(() => []),
      listNodes().then((l) => l.map((n) => n.name)).catch(() => [] as string[]),
    ]).then(([vpcs, subnets, vnfs, firewalls, nodes]) => setLists({ vpcs, subnets, vnfs, firewalls, nodes }));
    if (target) {
      setObj(null);
      getResource(resource, target.namespace, target.name)
        .then((o) => {
          setFetched(o);
          setObj(o);
        })
        .catch((e) => setError(friendlyError(e)));
    } else {
      setFetched(null);
      setObj({
        apiVersion: meta.apiVersion,
        kind: meta.kind,
        metadata: { name: '', namespace: 'sdn-demo' },
        spec: JSON.parse(JSON.stringify(form.defaults?.spec ?? {})),
      });
    }
  }, [open, resource, target?.namespace, target?.name]); // eslint-disable-line react-hooks/exhaustive-deps

  // default namespace to something that exists
  useEffect(() => {
    if (!open || edit || !obj || namespaces.length === 0) return;
    if (!namespaces.includes(obj.metadata.namespace)) {
      setObj((o: any) => setIn(o, 'metadata.namespace', namespaces.includes('default') ? 'default' : namespaces[0]));
    }
  }, [namespaces, open, edit, obj?.metadata?.namespace]); // eslint-disable-line react-hooks/exhaustive-deps

  const ns = obj?.metadata?.namespace;
  const ctx = useMemo(() => {
    const inNs = (x: any) => !ns || x.metadata?.namespace === ns;
    const names = (l: any[]) => l.filter(inNs).map((x) => x.metadata.name).sort();
    const subnetsByVpc: Record<string, string[]> = {};
    for (const s of lists.subnets.filter(inNs)) {
      (subnetsByVpc[s.spec?.vpcRef] ??= []).push(s.metadata.name);
    }
    return { vpcs: names(lists.vpcs), subnets: names(lists.subnets), vnfs: names(lists.vnfs), firewalls: names(lists.firewalls), nodes: [...lists.nodes].sort(), subnetsByVpc };
  }, [lists, ns]) as Ctx & { subnetsByVpc: Record<string, string[]> };

  if (!open) return null;

  const switchTab = (t: 'form' | 'yaml') => {
    if (t === tab) return;
    setError(null);
    if (t === 'yaml') {
      setYamlText(yaml.dump(stripForYaml(obj), { noRefs: true, lineWidth: 120 }));
      setTab('yaml');
    } else {
      try {
        const parsed = yaml.load(yamlText);
        if (!parsed || typeof parsed !== 'object') throw new Error('Spec must be a YAML/JSON object');
        setObj(parsed);
        setResetKey((k) => k + 1);
        setTab('form');
      } catch (e) {
        setError(`Invalid YAML: ${(e as Error).message}`);
      }
    }
  };

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    let out: any;
    if (tab === 'yaml') {
      try {
        out = yaml.load(yamlText);
        if (!out || typeof out !== 'object') throw new Error('document must be an object');
      } catch (err) {
        setError(`Invalid YAML: ${(err as Error).message}`);
        return;
      }
    } else {
      out = obj;
      for (const f of form.fields) {
        if (f.stripWhenHidden && f.showIf && !f.showIf(out)) out = setIn(out, f.path, undefined);
      }
    }
    out = { ...out, apiVersion: out.apiVersion ?? meta.apiVersion, kind: out.kind ?? meta.kind };
    if (edit && fetched) {
      // keep server-owned parts (status) that the YAML view hides
      if (fetched.status && !out.status) out.status = fetched.status;
      out.metadata = { ...(out.metadata ?? {}), name: target!.name, namespace: target!.namespace };
      if (!out.metadata.resourceVersion && fetched.metadata?.resourceVersion)
        out.metadata.resourceVersion = fetched.metadata.resourceVersion;
    }
    const name = out.metadata?.name;
    const namespace = out.metadata?.namespace;
    if (!name) {
      setError('metadata.name is required');
      return;
    }
    setSaving(true);
    try {
      if (edit) await updateResource(resource, target!.namespace, target!.name, out);
      else await createResource(resource, namespace || 'default', out);
      onSaved();
      onClose();
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setSaving(false);
    }
  };

  const tabCls = (t: string) =>
    `px-4 py-2 text-sm font-medium border-b-2 ${tab === t ? 'border-blue-500 text-blue-600 dark:text-blue-400' : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'}`;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onClose} />
      <div className="relative flex max-h-[92vh] w-full max-w-3xl flex-col rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900">
        <div className="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {edit ? 'Edit' : 'Create'} {meta.label}
            {edit && <span className="ml-2 text-sm font-normal text-gray-500">{target!.name}</span>}
          </h3>
          <button onClick={onClose} className="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="flex border-b border-gray-200 px-4 dark:border-gray-700">
          <button type="button" className={tabCls('form')} onClick={() => switchTab('form')}>Form</button>
          <button type="button" className={tabCls('yaml')} onClick={() => switchTab('yaml')}>YAML / JSON spec</button>
        </div>

        <form onSubmit={save} className="flex min-h-0 flex-1 flex-col">
          <div className="flex-1 space-y-4 overflow-y-auto px-6 py-5">
            {error && (
              <div className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-300">
                {error}
              </div>
            )}
            {!obj ? (
              <div className="py-8 text-center text-sm text-gray-400">{error ? '' : 'Loading…'}</div>
            ) : tab === 'yaml' ? (
              <textarea
                value={yamlText}
                onChange={(e) => setYamlText(e.target.value)}
                spellCheck={false}
                className="h-96 w-full rounded-lg border border-gray-300 bg-gray-50 p-3 font-mono text-xs text-gray-900 outline-none focus:border-blue-500 dark:border-gray-600 dark:bg-gray-950 dark:text-gray-100"
              />
            ) : (
              <div key={resetKey} className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <Labeled label="Name" required>
                    <input className={inputCls} required disabled={edit} value={obj.metadata?.name ?? ''}
                      pattern="[a-z0-9]([-a-z0-9.]*[a-z0-9])?" title="lowercase alphanumerics, '-' or '.'"
                      onChange={(e) => setObj(setIn(obj, 'metadata.name', e.target.value))} placeholder={`my-${meta.kind.toLowerCase()}`} />
                  </Labeled>
                  <Labeled label="Namespace" required>
                    <select className={inputCls} disabled={edit} value={obj.metadata?.namespace ?? ''}
                      onChange={(e) => setObj(setIn(obj, 'metadata.namespace', e.target.value))}>
                      {(namespaces.length ? namespaces : [obj.metadata?.namespace ?? 'default']).map((n) => <option key={n}>{n}</option>)}
                    </select>
                  </Labeled>
                </div>
                {form.fields.filter((f) => !f.showIf || f.showIf(obj)).map((f) => {
                  const isReq = typeof f.required === 'function' ? f.required(obj) : f.required;
                  return (
                    <Labeled key={f.path} label={f.label} required={!!isReq} help={f.help}>
                      <FieldInput field={f} value={getIn(obj, f.path)} ctx={ctx} obj={obj}
                        onChange={(v) => setObj(setIn(obj, f.path, v))} />
                    </Labeled>
                  );
                })}
                <Labeled label="Tags" help="key=value tags, stored as tags.sdncontroller.io/* labels">
                  <TagEditor obj={obj} onChange={(tags) => setObj(withTags(obj, tags))} />
                </Labeled>
                <p className="text-xs text-gray-400">
                  Fields not shown here (e.g. health checks) are preserved; use the YAML tab to edit them.
                </p>
              </div>
            )}
          </div>
          <div className="flex justify-end gap-3 border-t border-gray-200 px-6 py-4 dark:border-gray-700">
            <button type="button" onClick={onClose} className={btnSecondary}>Cancel</button>
            <button type="submit" disabled={saving || !obj} className={btnPrimary}>
              {saving ? 'Saving…' : edit ? 'Save changes' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function Labeled({ label, required, help, children }: { label: string; required?: boolean; help?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {label}
        {required && <span className="ml-1 text-red-500">*</span>}
      </label>
      {children}
      {help && <p className="mt-1 text-xs text-gray-400">{help}</p>}
    </div>
  );
}
