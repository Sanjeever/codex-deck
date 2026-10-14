import {useEffect, useState} from 'react';
import {backend, type CLIProxyState} from '../bridge/api';
import type {RunOperation} from '../app/useDeckController';

export interface CLIProxyDraft {url: string; remove: boolean}
const initialDraft = (state: CLIProxyState): CLIProxyDraft => {
  const values = [...new Set(state.variables.map(variable => variable.value))];
  return {url: values.length === 1 ? values[0] : '', remove: false};
};

export function useCLIProxyDraft(active: boolean, busy: string, run: RunOperation) {
  const [state, setState] = useState<CLIProxyState | null>(null);
  const [draft, setDraft] = useState<CLIProxyDraft>({url: '', remove: false});
  const [saved, setSaved] = useState(draft);
  const [error, setError] = useState('');
  const [applied, setApplied] = useState(false);
  const dirty = draft.url !== saved.url || draft.remove !== saved.remove;
  const accept = (value: CLIProxyState) => {setState(value); const next = initialDraft(value); setDraft(next); setSaved(next); setError('')};
  const read = () => void run('正在读取 CLI 代理', () => backend().GetCLIProxy(), accept, setError);
  useEffect(() => {if (active && !busy && state === null && !error) read()}, [active, busy, state, error, run]);
  const save = async () => {
    if (!state) throw new Error('请先读取 CLI 代理配置');
    const value = await backend().SaveCLIProxy({url: draft.url, remove: draft.remove, expected: state.variables});
    accept(value); setApplied(true);
  };
  const change = (value: CLIProxyDraft) => {setDraft(value); setApplied(false)};
  const undo = () => {setDraft(saved)};
  return {state, draft, change, dirty, error, applied, read, save, undo};
}
