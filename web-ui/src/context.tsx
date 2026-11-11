import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';

/* ---------------- Theme ---------------- */

type Theme = 'light' | 'dark';
const ThemeCtx = createContext<{ theme: Theme; toggle: () => void }>({
  theme: 'dark',
  toggle: () => {},
});

function initialTheme(): Theme {
  try {
    const t = localStorage.getItem('sdn-theme');
    if (t === 'light' || t === 'dark') return t;
  } catch {
    /* ignore */
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(initialTheme);
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
  }, [theme]);
  const toggle = useCallback(() => {
    setTheme((t) => {
      const n = t === 'dark' ? 'light' : 'dark';
      try {
        localStorage.setItem('sdn-theme', n);
      } catch {
        /* ignore */
      }
      return n;
    });
  }, []);
  return <ThemeCtx.Provider value={{ theme, toggle }}>{children}</ThemeCtx.Provider>;
}
export const useTheme = () => useContext(ThemeCtx);

/* ---------------- Refresh ---------------- */

interface RefreshState {
  tick: number;
  refresh: () => void;
  auto: boolean;
  setAuto: (v: boolean) => void;
  lastUpdated: Date | null;
  markUpdated: () => void;
  fetching: number;
  setFetching: (d: number) => void;
}

const RefreshCtx = createContext<RefreshState>({
  tick: 0,
  refresh: () => {},
  auto: false,
  setAuto: () => {},
  lastUpdated: null,
  markUpdated: () => {},
  fetching: 0,
  setFetching: () => {},
});

export function RefreshProvider({ children }: { children: ReactNode }) {
  const [tick, setTick] = useState(0);
  const [auto, setAutoState] = useState(() => localStorage.getItem('sdn-auto-refresh') === '1');
  const [lastUpdated, setLast] = useState<Date | null>(null);
  const [fetching, setFetchingState] = useState(0);
  const refresh = useCallback(() => setTick((t) => t + 1), []);
  const markUpdated = useCallback(() => setLast(new Date()), []);
  const setFetching = useCallback((d: number) => setFetchingState((f) => Math.max(0, f + d)), []);
  const setAuto = useCallback((v: boolean) => {
    localStorage.setItem('sdn-auto-refresh', v ? '1' : '0');
    setAutoState(v);
  }, []);
  useEffect(() => {
    if (!auto) return;
    const id = setInterval(refresh, 10000);
    return () => clearInterval(id);
  }, [auto, refresh]);
  return (
    <RefreshCtx.Provider
      value={{ tick, refresh, auto, setAuto, lastUpdated, markUpdated, fetching, setFetching }}
    >
      {children}
    </RefreshCtx.Provider>
  );
}
export const useRefresh = () => useContext(RefreshCtx);

/** Load data; reloads whenever the global refresh tick (button / auto-refresh) changes. */
export function useData<T>(fetcher: () => Promise<T>, deps: unknown[] = []) {
  const { tick, markUpdated, setFetching } = useRefresh();
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const fetchRef = useRef(fetcher);
  fetchRef.current = fetcher;
  useEffect(() => {
    let cancelled = false;
    setFetching(1);
    fetchRef.current()
      .then((d) => {
        if (cancelled) return;
        setData(d);
        setError(null);
        markUpdated();
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        setFetching(-1);
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, ...deps]);
  return { data, error, loading };
}
