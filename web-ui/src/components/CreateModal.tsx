import { useState, useEffect } from 'react';
import { X } from 'lucide-react';

export interface FieldDef {
  name: string;
  label: string;
  type: 'text' | 'number' | 'select';
  options?: string[];
  defaultValue?: string | number;
  required?: boolean;
  placeholder?: string;
}

interface CreateModalProps {
  open: boolean;
  onClose: () => void;
  onSave: (values: Record<string, any>) => void;
  title: string;
  fields: FieldDef[];
}

export default function CreateModal({
  open,
  onClose,
  onSave,
  title,
  fields,
}: CreateModalProps) {
  const [values, setValues] = useState<Record<string, any>>({});

  useEffect(() => {
    if (open) {
      const defaults: Record<string, any> = {};
      for (const f of fields) {
        if (f.defaultValue !== undefined) {
          defaults[f.name] = f.defaultValue;
        } else if (f.type === 'number') {
          defaults[f.name] = 0;
        } else if (f.type === 'select' && f.options && f.options.length > 0) {
          defaults[f.name] = f.options[0];
        } else {
          defaults[f.name] = '';
        }
      }
      setValues(defaults);
    }
  }, [open, fields]);

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave(values);
  };

  const setValue = (name: string, value: any) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative w-full max-w-lg rounded-xl border border-gray-700 bg-gray-800 shadow-2xl">
        <div className="flex items-center justify-between border-b border-gray-700 px-6 py-4">
          <h3 className="text-lg font-semibold text-gray-100">{title}</h3>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-700 hover:text-gray-200"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 px-6 py-5">
          {fields.map((field) => (
            <div key={field.name}>
              <label className="mb-1.5 block text-sm font-medium text-gray-300">
                {field.label}
                {field.required && <span className="ml-1 text-red-400">*</span>}
              </label>
              {field.type === 'select' ? (
                <select
                  value={values[field.name] ?? ''}
                  onChange={(e) => setValue(field.name, e.target.value)}
                  required={field.required}
                  className="w-full rounded-lg border border-gray-600 bg-gray-700 px-3 py-2 text-sm text-gray-100 outline-none transition-colors focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                >
                  {field.options?.map((opt) => (
                    <option key={opt} value={opt}>
                      {opt}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type={field.type}
                  value={values[field.name] ?? ''}
                  onChange={(e) =>
                    setValue(
                      field.name,
                      field.type === 'number'
                        ? Number(e.target.value)
                        : e.target.value
                    )
                  }
                  required={field.required}
                  placeholder={field.placeholder}
                  className="w-full rounded-lg border border-gray-600 bg-gray-700 px-3 py-2 text-sm text-gray-100 placeholder-gray-500 outline-none transition-colors focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                />
              )}
            </div>
          ))}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-600 px-4 py-2 text-sm font-medium text-gray-300 transition-colors hover:bg-gray-700"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-500"
            >
              Create
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
