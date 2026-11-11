import { useEffect, useMemo, useRef, useState } from 'react';
import { Plus, X } from 'lucide-react';
import {
  tagSuggestions, tagStr, tagsOf, validateTagKey, validateTagValue, type Tag,
} from '../tags';
import { inputCls } from './ui';

const chipCls =
  'inline-flex items-center gap-1 rounded-full border border-purple-300 bg-purple-100 px-2 py-0.5 text-[11px] font-medium text-purple-700 dark:border-purple-500/30 dark:bg-purple-500/15 dark:text-purple-300';

/** Read-only tag chip; clickable when onClick given. */
export function TagChip({ tag, onClick, title }: { tag: Tag; onClick?: () => void; title?: string }) {
  const body = (
    <>
      <span className="font-semibold">{tag.key}</span>
      <span className="opacity-60">=</span>
      <span>{tag.value === '' ? '∅' : tag.value}</span>
    </>
  );
  if (onClick)
    return (
      <button type="button" onClick={(e) => { e.stopPropagation(); onClick(); }} title={title ?? `filter by ${tagStr(tag)}`}
        className={chipCls + ' cursor-pointer hover:border-purple-500 hover:bg-purple-200 dark:hover:bg-purple-500/25'}>
        {body}
      </button>
    );
  return <span title={title ?? tagStr(tag)} className={chipCls}>{body}</span>;
}

/** Compact tag list for table cells: first `max` chips + "+N" for the rest. */
export function TagChips({ item, max = 3, onClickTag }: { item: any; max?: number; onClickTag?: (t: Tag) => void }) {
  const tags = tagsOf(item);
  if (!tags.length) return <span className="text-gray-400">-</span>;
  const shown = tags.slice(0, max);
  const rest = tags.length - shown.length;
  return (
    <span className="flex max-w-[220px] flex-wrap gap-1" onClick={(e) => e.stopPropagation()}>
      {shown.map((t) => <TagChip key={t.key} tag={t} onClick={onClickTag ? () => onClickTag(t) : undefined} />)}
      {rest > 0 && (
        <span className={chipCls} title={tags.slice(max).map(tagStr).join(', ')}>+{rest}</span>
      )}
    </span>
  );
}

/* ---------------- TagEditor ---------------- */

export function TagEditor({ obj, onChange }: { obj: any; onChange: (tags: Tag[]) => void }) {
  const tags: Tag[] = tagsOf(obj);
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [suggestions, setSuggestions] = useState<Tag[]>([]);
  const [showSug, setShowSug] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    tagSuggestions().then(setSuggestions).catch(() => {});
  }, []);

  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setShowSug(false);
    };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  const available = useMemo(
    () => suggestions.filter((s) => !tags.some((t) => t.key === s.key && t.value === s.value)),
    [suggestions, tags],
  );
  const filtered = available.filter((s) =>
    (!key || s.key.toLowerCase().includes(key.toLowerCase())) &&
    (!value || s.value.toLowerCase().includes(value.toLowerCase())),
  );

  const add = (k = key.trim(), v = value.trim()) => {
    const ke = validateTagKey(k);
    if (ke) return setErr(`key "${k}": ${ke}`);
    const ve = validateTagValue(v);
    if (ve) return setErr(`value "${v}": ${ve}`);
    if (tags.some((t) => t.key === k)) return setErr(`duplicate key "${k}"`);
    setErr(null);
    onChange([...tags, { key: k, value: v }].sort((a, b) => a.key.localeCompare(b.key)));
    setKey('');
    setValue('');
  };

  const remove = (k: string) => onChange(tags.filter((t) => t.key !== k));

  return (
    <div ref={boxRef}>
      <div className="flex flex-wrap items-center gap-1.5">
        {tags.map((t) => (
          <span key={t.key} className={chipCls}>
            <span className="font-semibold">{t.key}</span>
            <span className="opacity-60">=</span>
            <span>{t.value === '' ? '∅' : t.value}</span>
            <button type="button" onClick={() => remove(t.key)} className="ml-0.5 rounded-full p-0.5 hover:bg-purple-200 dark:hover:bg-purple-500/30" title="remove tag">
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        {tags.length === 0 && <span className="text-xs text-gray-400">No tags</span>}
      </div>
      <div className="relative mt-2 flex items-center gap-2">
        <input
          className={inputCls + ' !w-40'} placeholder="key" value={key}
          onChange={(e) => { setKey(e.target.value); setErr(null); setShowSug(true); }}
          onFocus={() => setShowSug(true)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add(); } }}
        />
        <span className="text-gray-400">=</span>
        <input
          className={inputCls + ' !w-40'} placeholder="value (may be empty)" value={value}
          onChange={(e) => { setValue(e.target.value); setErr(null); setShowSug(true); }}
          onFocus={() => setShowSug(true)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add(); } }}
        />
        <button type="button" className="inline-flex items-center gap-1 rounded-lg border border-gray-300 px-2.5 py-2 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
          onClick={() => add()}>
          <Plus className="h-3.5 w-3.5" /> Add
        </button>
        {showSug && filtered.length > 0 && (
          <div className="absolute left-0 top-full z-10 mt-1 max-h-44 w-96 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-600 dark:bg-gray-800">
            <div className="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wide text-gray-400">Existing tags</div>
            {filtered.slice(0, 30).map((s) => (
              <button key={s.key + '=' + s.value} type="button"
                className="flex w-full items-center px-3 py-1.5 text-left text-xs hover:bg-purple-50 dark:hover:bg-purple-500/10"
                onClick={() => add(s.key, s.value)}>
                <TagChip tag={s} />
              </button>
            ))}
          </div>
        )}
      </div>
      {err && <p className="mt-1 text-xs text-red-500">{err}</p>}
      <p className="mt-1 text-[11px] text-gray-400">
        Stored as labels <code>tags.sdncontroller.io/&lt;key&gt;</code>. Other labels are left untouched.
      </p>
    </div>
  );
}

/* ---------------- TagFilterBar ---------------- */

export function TagFilterBar({
  selected,
  onChange,
  extra,
}: {
  selected: Tag[];
  onChange: (tags: Tag[]) => void;
  extra?: React.ReactNode;
}) {
  const [text, setText] = useState('');
  const [suggestions, setSuggestions] = useState<Tag[]>([]);
  const [open, setOpen] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    tagSuggestions().then(setSuggestions).catch(() => {});
  }, []);
  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  const opts = suggestions.filter(
    (s) =>
      !selected.some((t) => t.key === s.key && t.value === s.value) &&
      tagStr(s).toLowerCase().includes(text.toLowerCase()),
  );

  const addTag = (t: Tag) => {
    if (!selected.some((x) => x.key === t.key && x.value === t.value)) onChange([...selected, t]);
    setText('');
  };
  const submit = () => {
    // accept "key=value" or a bare "key=value"-ish text
    const i = text.indexOf('=');
    if (i > 0) {
      const t = { key: text.slice(0, i).trim(), value: text.slice(i + 1).trim() };
      if (!validateTagKey(t.key) && !validateTagValue(t.value)) { addTag(t); return; }
    }
    const exact = opts.find((s) => tagStr(s) === text.trim());
    if (exact) addTag(exact);
  };

  return (
    <div ref={boxRef} className="flex flex-wrap items-center gap-2">
      {extra}
      <span className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Tags</span>
      {selected.map((t) => (
        <span key={t.key + '=' + t.value} className={chipCls}>
          <span className="font-semibold">{t.key}</span><span className="opacity-60">=</span><span>{t.value === '' ? '∅' : t.value}</span>
          <button type="button" className="ml-0.5 rounded-full p-0.5 hover:bg-purple-200 dark:hover:bg-purple-500/30"
            onClick={() => onChange(selected.filter((x) => !(x.key === t.key && x.value === t.value)))}>
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <div className="relative">
        <input
          className={inputCls + ' !w-56 !py-1.5 text-xs'} placeholder="filter key=value…"
          value={text}
          onChange={(e) => { setText(e.target.value); setOpen(true); }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit(); } }}
        />
        {open && opts.length > 0 && (
          <div className="absolute left-0 top-full z-10 mt-1 max-h-52 w-72 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-600 dark:bg-gray-800">
            {opts.slice(0, 40).map((s) => (
              <button key={s.key + '=' + s.value} type="button"
                className="flex w-full px-3 py-1.5 text-left text-xs hover:bg-purple-50 dark:hover:bg-purple-500/10"
                onClick={() => addTag(s)}>
                <TagChip tag={s} />
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
