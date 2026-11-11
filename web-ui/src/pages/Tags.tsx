import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Tags as TagsIcon } from 'lucide-react';
import { listTags, resourceLabel, type TagEntry } from '../tags';
import { useData } from '../context';
import { detailPath } from '../resources';
import { Card, ErrorBanner, MiniTable, PageHeader, inputCls } from '../components/ui';

export default function Tags() {
  const { data, error, loading } = useData(listTags);
  const [q, setQ] = useState('');
  const entries: TagEntry[] = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return data ?? [];
    return (data ?? []).filter(
      (e) =>
        `${e.key}=${e.value}`.toLowerCase().includes(needle) ||
        e.resources.some((r) => r.name.toLowerCase().includes(needle)),
    );
  }, [data, q]);

  return (
    <div>
      <PageHeader
        title={<span className="flex items-center gap-3"><TagsIcon className="h-6 w-6 text-purple-500" />Tags</span>}
        subtitle="All tags (tags.sdncontroller.io/* labels) across every SDN resource"
      />
      <ErrorBanner message={error} />
      <div className="mb-4">
        <input className={inputCls + ' !w-80'} placeholder="filter by tag or resource…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {loading && !data ? (
        <div className="text-gray-400">Loading…</div>
      ) : (
        <Card title={`Tags (${entries.length})`}>
          <MiniTable
            rows={entries}
            empty="No tags found — tag resources from their detail page or create/edit dialog."
            cols={[
              {
                header: 'Tag',
                render: (e: TagEntry) => (
                  <span className="rounded-full border border-purple-300 bg-purple-100 px-2.5 py-0.5 text-xs font-medium text-purple-700 dark:border-purple-500/30 dark:bg-purple-500/15 dark:text-purple-300">
                    <span className="font-semibold">{e.key}</span>=<span>{e.value === '' ? '∅' : e.value}</span>
                  </span>
                ),
              },
              { header: 'Resources', render: (e: TagEntry) => e.count },
              {
                header: 'Carried by',
                render: (e: TagEntry) => (
                  <span className="flex flex-wrap gap-x-3 gap-y-1">
                    {e.resources.map((r) => (
                      <Link key={`${r.resource}/${r.namespace}/${r.name}`} to={detailPath(r.resource, r.namespace, r.name)}
                        className="text-blue-600 hover:underline dark:text-blue-400">
                        <span className="text-gray-500 dark:text-gray-400">{resourceLabel(r.resource)}/</span>
                        {r.name}
                        <span className="ml-1 text-xs text-gray-400">({r.namespace})</span>
                      </Link>
                    ))}
                  </span>
                ),
              },
            ]}
          />
        </Card>
      )}
    </div>
  );
}
