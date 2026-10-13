import {useCallback, useEffect, useRef, useState} from 'react';
import {backend} from '../bridge/api';

export function useAutoRefresh(enabled: boolean, paused: boolean) {
  const [error, setError] = useState('');
  const pending = useRef(Promise.resolve());
  // 操作前的暂停与渲染后的恢复按顺序送到后端。
  const setPaused = useCallback((value: boolean) => {
    const request = pending.current.then(() => backend().SetAutomaticPaused(value));
    pending.current = request.then(() => setError(''), cause => setError(cause instanceof Error ? cause.message : String(cause)));
    return request;
  }, []);
  const pause = useCallback(() => setPaused(true), [setPaused]);

  useEffect(() => {void setPaused(!enabled || paused)}, [enabled, paused, setPaused]);
  useEffect(() => {
    const wake = () => {void backend().WakeAutomatic().catch(cause => setError(cause instanceof Error ? cause.message : String(cause)))};
    window.addEventListener('online', wake);
    return () => window.removeEventListener('online', wake);
  }, []);
  return {pause, error};
}
