import type { ReactNode } from 'react';
import { Loader2, Pencil, Plus, Trash2 } from 'lucide-react';
import { btnPrimary } from './ui';

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
  onEdit?: (item: any) => void;
  onClick?: (item: any) => void;
  onCreate?: () => void;
}

function getNestedValue(obj: any, path: string): string {
  const val = path.split('.').reduce((acc, part) => acc?.[part], obj);
  if (val === undefined || val === null) return '-';
  return String(val);
}

export default function ResourceTable({
  title,
  columns,
  data,
  loading,
  onDelete,
  onEdit,
  onClick,
  onCreate,
}: ResourceTableProps) {
  const th = 'px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400';
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-gray-700 dark:bg-gray-900">
      <div className="flex items-center justify-between border-b border-gray-200 px-5 py-3 dark:border-gray-700">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100">{title}</h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-500 dark:text-gray-400">{data.length} items</span>
          {onCreate && (
            <button onClick={onCreate} className={btnPrimary}>
              <Plus className="h-4 w-4" /> Create
            </button>
          )}
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
        </div>
      ) : data.length === 0 ? (
        <div className="py-12 text-center text-gray-400">No resources found</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800/60">
                {columns.map((col) => (
                  <th key={col.key} className={th}>{col.header}</th>
                ))}
                {(onDelete || onEdit) && <th className={`${th} text-right`}>Actions</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {data.map((item, idx) => (
                <tr
                  key={`${item.metadata?.namespace}/${item.metadata?.name ?? idx}`}
                  className={`transition-colors hover:bg-gray-50 dark:hover:bg-gray-800/60 ${onClick ? 'cursor-pointer' : ''}`}
                  onClick={() => onClick?.(item)}
                >
                  {columns.map((col) => (
                    <td key={col.key} className="whitespace-nowrap px-5 py-3 text-sm text-gray-700 dark:text-gray-300">
                      {col.render ? col.render(item) : getNestedValue(item, col.key)}
                    </td>
                  ))}
                  {(onDelete || onEdit) && (
                    <td className="whitespace-nowrap px-5 py-3 text-right">
                      {onEdit && (
                        <button
                          onClick={(e) => { e.stopPropagation(); onEdit(item); }}
                          className="rounded-lg p-2 text-gray-500 transition-colors hover:bg-blue-100 hover:text-blue-600 dark:hover:bg-blue-500/20 dark:hover:text-blue-400"
                          title="Edit"
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                      )}
                      {onDelete && (
                        <button
                          onClick={(e) => { e.stopPropagation(); onDelete(item); }}
                          className="rounded-lg p-2 text-gray-500 transition-colors hover:bg-red-100 hover:text-red-600 dark:hover:bg-red-500/20 dark:hover:text-red-400"
                          title="Delete"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      )}
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
