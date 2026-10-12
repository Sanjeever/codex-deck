import {useEffect, useRef, useState} from 'react';

const INTERVAL = 5 * 60_000;
const EVENT_DELAY = 1000;

export function useAutoRefresh<T>(request: () => Promise<T>, onResult: (result: T) => void, enabled: boolean, paused: boolean, nextAt: number | null) {
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const flight = useRef(false);
  const lastStart = useRef(-Infinity);
  const retryAfter = useRef(0);
  const mounted = useRef(false);
  const resume = useRef<(() => void) | null>(null);

  useEffect(() => {mounted.current = true; return () => {mounted.current = false}}, []);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const active = () => enabled && !paused && document.visibilityState === 'visible' && document.hasFocus() && navigator.onLine;
    const stop = () => {clearTimeout(timer)};
    const schedule = () => {
      stop();
      if (disposed || !active() || flight.current) return;
      const due = nextAt ?? Date.now() + INTERVAL;
      timer = setTimeout(() => void start(), Math.max(0, due - Date.now(), lastStart.current + EVENT_DELAY - Date.now(), retryAfter.current - Date.now()));
    };
    const start = async () => {
      if (disposed || !active() || flight.current) return;
      flight.current = true; lastStart.current = Date.now(); setRefreshing(true);
      try {
        const result = await request();
        if (!disposed && mounted.current) {retryAfter.current = 0; onResult(result); setError('')}
      } catch (cause) {
        retryAfter.current = Date.now() + INTERVAL;
        if (!disposed && mounted.current) setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        flight.current = false;
        if (mounted.current) {setRefreshing(false); resume.current?.()}
      }
    };
    const wake = () => {
      stop();
      if (!active() || flight.current) return;
      if (retryAfter.current > Date.now()) {schedule(); return}
      const delay = lastStart.current + EVENT_DELAY - Date.now();
      if (delay > 0) {timer = setTimeout(() => void start(), delay)} else {void start()}
    };
    resume.current = schedule;
    const reconnect = () => {retryAfter.current = 0; wake()};
    window.addEventListener('focus', wake);
    window.addEventListener('blur', stop);
    window.addEventListener('online', reconnect);
    window.addEventListener('offline', stop);
    document.addEventListener('visibilitychange', wake);
    schedule();
    return () => {
      disposed = true; stop();
      resume.current = null;
      window.removeEventListener('focus', wake);
      window.removeEventListener('blur', stop);
      window.removeEventListener('online', reconnect);
      window.removeEventListener('offline', stop);
      document.removeEventListener('visibilitychange', wake);
    };
  }, [request, onResult, enabled, paused, nextAt]);
  return {refreshing, error};
}
