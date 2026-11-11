import type { ReactNode } from 'react';
import { Trash2, Plus, Loader2 } from 'lucide-react';

export interface Column {
  key: string;
  header: string;
  render?: (item: any) => ReactNode;
}

interface ResourceTableProps {
  title: string;
  columns: Column[];
  data: any[];
  loading?: boolean;
  onDelete?: (item: any) => void;
  onClick?: (item: any) => void;
  onCreate?: () => void;
}

export default function ResourceTable({
  title,
  columns,
  data,
  loading,
  onDelete,
  onClick,
  onCreate,
}: ResourceTableProps) {
  return (
    <div className="rounded-xl border border-gray-700 bg-gray-800 overflow-hidden">
      <div className="flex items-center justify-between border-b border-gray-700 px-6 py-4">
        <h2 className="text-lg font-semibold text-gray-100">{title}</h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-400">{data.length} items</span>
          {onCreate && (
            <button
              onClick={onCreate}
              className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-500"
            >
              <Plus className="h-4 w-4" />
              Create
            </button>
          )}
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
        </div>
      ) : data.length === 0 ? (
        <div className="py-12 text-center text-gray-400">
          No resources found
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-700 bg-gray-800/50">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-400"
                  >
                    {col.header}
                  </th>
                ))}
                {onDelete && (
                  <th className="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-400">
                    Actions
                  </th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-700">
              {data.map((item, idx) => (
                <tr
                  key={item.metadata?.name ?? idx}
                  className={`transition-colors hover:bg-gray-700/50 ${
                    onClick ? 'cursor-pointer' : ''
                  }`}
                  onClick={() => onClick?.(item)}
                >
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className="whitespace-nowrap px-6 py-4 text-sm text-gray-300"
                    >
                      {col.render
                        ? col.render(item)
                        : getNestedValue(item, col.key)}
                    </td>
                  ))}
                  {onDelete && (
                    <td className="whitespace-nowrap px-6 py-4 text-right">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onDelete(item);
                        }}
                        className="rounded-lg p-2 text-gray-400 transition-colors hover:bg-red-500/20 hover:text-red-400"
                        title="Delete"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function getNestedValue(obj: any, path: string): string {
  const val = path.split('.').reduce((acc, part) => acc?.[part], obj);
  if (val === undefined || val === null) return '-';
  return String(val);
}
