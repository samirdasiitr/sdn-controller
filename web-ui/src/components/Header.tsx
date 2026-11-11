import { Moon, Sun } from 'lucide-react';
import { useRefresh, useTheme } from '../context';
import { RefreshButton } from './ui';

export default function Header() {
  const { theme, toggle } = useTheme();
  const { auto, setAuto, lastUpdated } = useRefresh();
  return (
    <header className="flex h-14 shrink-0 items-center justify-end gap-4 border-b border-gray-200 bg-white px-6 dark:border-gray-800 dark:bg-gray-900">
      <span className="text-xs text-gray-500 dark:text-gray-400" title="Time of the last successful data load">
        Last updated: {lastUpdated ? lastUpdated.toLocaleTimeString() : '—'}
      </span>
      <label className="flex cursor-pointer items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
        <input type="checkbox" checked={auto} onChange={(e) => setAuto(e.target.checked)} className="h-4 w-4" />
        Auto-refresh (10s)
      </label>
      <RefreshButton />
      <button
        onClick={toggle}
        className="rounded-lg border border-gray-300 p-2 text-gray-600 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
        title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        aria-label="Toggle theme"
      >
        {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
      </button>
    </header>
  );
}
