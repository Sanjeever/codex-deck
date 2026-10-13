import {useEffect, useEffectEvent} from 'react';

export function useKeyboardShortcuts(enabled: boolean, actions: {refresh: () => void; settings: () => void; login: () => void; search: () => void}) {
  const keydown = useEffectEvent((event: KeyboardEvent) => {
    if (!enabled || event.repeat || event.altKey || !(event.metaKey || event.ctrlKey)) return;
    const target = event.target as HTMLElement;
    const editing = target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
    const key = event.key.toLowerCase();
    if (key === 'r') {event.preventDefault(); actions.refresh()}
    else if (key === ',') {event.preventDefault(); actions.settings()}
    else if (key === 'n' && !editing) {event.preventDefault(); actions.login()}
    else if (key === 'f') {event.preventDefault(); actions.search()}
  });
  useEffect(() => {
    window.addEventListener('keydown', keydown);
    return () => window.removeEventListener('keydown', keydown);
  }, []);
}
