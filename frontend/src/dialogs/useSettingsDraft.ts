import {useEffect, useState} from 'react';
import {backend, type Configuration, type State} from '../bridge/api';
import type {RunOperation} from '../app/useDeckController';
import {useCLIProxyDraft} from './useCLIProxyDraft';

export type SettingsTab = 'general' | 'network' | 'backup' | 'about';
export interface SettingsError {section: string; text: string}
const errorText = (error: unknown) => error instanceof Error ? error.message : String(error);
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export function useSettingsDraft(state: Pick<State, 'network' | 'settings' | 'preferences'>, run: RunOperation, initialTab: SettingsTab = 'general', busy = '') {
  const [draft, setDraft] = useState<Configuration>(() => ({
    network: {...state.network}, settings: {...state.settings},
    preferences: {low_quota_notify: state.preferences.low_quota_notify, low_quota_threshold: state.preferences.low_quota_threshold, disable_update_check: !!state.preferences.disable_update_check},
  }));
  const [saved, setSaved] = useState(draft);
  const [tab, setTab] = useState<SettingsTab>(initialTab);
  const [autoStart, setAutoStart] = useState<boolean | null>(null);
  const [savedAutoStart, setSavedAutoStart] = useState<boolean | null>(null);
  const [error, setError] = useState<SettingsError | null>(null);
  const cliProxy = useCLIProxyDraft(tab === 'network', busy, run);
  const localDirty = !same(draft, saved);
  const autoStartDirty = autoStart !== savedAutoStart;
  const dirty = {
    general: autoStartDirty || draft.preferences.low_quota_notify !== saved.preferences.low_quota_notify || draft.preferences.low_quota_threshold !== saved.preferences.low_quota_threshold,
    network: !same(draft.network, saved.network) || cliProxy.dirty,
    backup: !same(draft.settings, saved.settings),
    about: draft.preferences.disable_update_check !== saved.preferences.disable_update_check,
  };
  const readAutoStart = () => void run('正在读取自启动设置', () => backend().GetAutoStart(), enabled => {setAutoStart(enabled); setSavedAutoStart(enabled); setError(null)}, text => setError({section: 'general', text}));
  useEffect(readAutoStart, [run]);

  const save = () => void run('正在保存设置', async () => {
    setError(null);
    let localSaved = false;
    if (localDirty) {
      let result;
      try {result = await backend().SaveConfiguration(draft)}
      catch (error) {setError({section: '', text: errorText(error)}); throw error}
      if (result.error) {
        setError({section: result.section, text: result.error});
        if (result.section) setTab(result.section as SettingsTab);
        throw new Error(result.error);
      }
      if (!result.configuration) throw new Error('保存设置未返回配置');
      setDraft(result.configuration); setSaved(result.configuration);
      localSaved = true;
    }
    if (cliProxy.dirty) {
      try {await cliProxy.save(); localSaved = true}
      catch (error) {
        const text = `${localSaved ? 'Deck 设置已保存，' : ''}CLI 代理保存失败：${errorText(error)}`;
        setError({section: 'network', text}); setTab('network'); throw new Error(text);
      }
    }
    if (autoStartDirty && autoStart !== null) {
      try {
        const actual = await backend().SetAutoStart(autoStart);
        setSavedAutoStart(actual);
        if (actual !== autoStart) throw new Error('系统未应用开机自启动设置');
      } catch (error) {
        const text = `${localSaved ? '其他设置已保存，' : ''}开机自启动设置失败：${errorText(error)}`;
        setError({section: 'general', text}); setTab('general');
        throw new Error(text);
      }
    }
  }, undefined, text => setError(current => ({section: current?.section ?? '', text})));

  return {draft, setDraft, tab, setTab, autoStart, setAutoStart, readAutoStart, error, dirty, cliProxy, unsaved: localDirty || autoStartDirty || cliProxy.dirty, save};
}
