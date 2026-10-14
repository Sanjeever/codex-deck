import {useEffect, useState} from 'react';
import {backend, type Configuration, type State} from '../bridge/api';
import {notify} from '../notifications/Notifications';
import type {RunOperation} from '../app/useDeckController';

export type SettingsTab = 'general' | 'network' | 'backup' | 'about';
export interface SettingsError {section: string; text: string}
const errorText = (error: unknown) => error instanceof Error ? error.message : String(error);
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export function useSettingsDraft(state: Pick<State, 'network' | 'settings' | 'preferences'>, run: RunOperation, initialTab: SettingsTab = 'general') {
  const [draft, setDraft] = useState<Configuration>(() => ({
    network: {...state.network}, settings: {...state.settings},
    preferences: {low_quota_notify: state.preferences.low_quota_notify, low_quota_threshold: state.preferences.low_quota_threshold, disable_update_check: !!state.preferences.disable_update_check},
  }));
  const [saved, setSaved] = useState(draft);
  const [tab, setTab] = useState<SettingsTab>(initialTab);
  const [autoStart, setAutoStart] = useState<boolean | null>(null);
  const [savedAutoStart, setSavedAutoStart] = useState<boolean | null>(null);
  const [error, setError] = useState<SettingsError | null>(null);
  const localDirty = !same(draft, saved);
  const autoStartDirty = autoStart !== savedAutoStart;
  const dirty = {
    general: autoStartDirty || draft.preferences.low_quota_notify !== saved.preferences.low_quota_notify || draft.preferences.low_quota_threshold !== saved.preferences.low_quota_threshold,
    network: !same(draft.network, saved.network),
    backup: !same(draft.settings, saved.settings),
    about: draft.preferences.disable_update_check !== saved.preferences.disable_update_check,
  };
  const readAutoStart = () => void run('正在读取自启动设置', async () => {
    try {return await backend().GetAutoStart()}
    catch (error) {setError({section: 'general', text: errorText(error)}); throw error}
  }, enabled => {setAutoStart(enabled); setSavedAutoStart(enabled); setError(null)});
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
  }, () => notify('success', '设置已保存'));

  return {draft, setDraft, tab, setTab, autoStart, setAutoStart, readAutoStart, error, dirty, unsaved: localDirty || autoStartDirty, save};
}
