import {useCallback, useEffect, useRef, useState} from 'react';
import {backend, type Account, type Preferences, type State} from '../bridge/api';
import {pendingConsumption} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import type {Dialog} from '../dialogs/DialogHost';
import type {SettingsTab} from '../dialogs/useSettingsDraft';
import {useAutoRefresh} from './useAutoRefresh';

export type RunOperation = <T>(label: string, action: () => Promise<T>, done?: (result: T) => void) => Promise<void>;

function errorText(error: unknown) {return error instanceof Error ? error.message : String(error)}

export function useDeckController() {
  const [state, setState] = useState<State | null>(null);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [busy, setBusy] = useState('');
  const busyRef = useRef(false);
  const [loadError, setLoadError] = useState('');
  const canceledLogin = useRef('');
  const remindedVersion = useRef('');
  const automatic = useAutoRefresh(state !== null, !!busy || dialog !== null);

  const run: RunOperation = useCallback(async <T,>(label: string, action: () => Promise<T>, done?: (result: T) => void) => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(label); setLoadError('');
    try {
      await automatic.pause();
      const result = await action();
      setState(await backend().GetState());
      done?.(result);
    } catch (error) {
      let text = errorText(error);
      try {setState(await backend().GetState())} catch (stateError) {if (errorText(stateError) !== text) text += `；${errorText(stateError)}`}
      setLoadError(text);
      notify('error', '操作失败', text);
    }
    finally {busyRef.current = false; setBusy('')}
  }, [automatic.pause]);

  const load = useCallback(() => run('正在加载', async () => {
    const initial = await backend().GetState(); setState(initial);
  }), [run]);
  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => window.runtime?.EventsOnMultiple('deck:state', setState, -1), []);

  useEffect(() => {
    const showUpdate = () => {
      if (state?.update.version && !state.build_info.development && !state.preferences.disable_update_check && !busy && !dialog && document.visibilityState === 'visible' && remindedVersion.current !== state.update.version) {
        remindedVersion.current = state.update.version;
        setDialog({kind: 'update'});
      }
    };
    showUpdate(); document.addEventListener('visibilitychange', showUpdate);
    return () => document.removeEventListener('visibilitychange', showUpdate);
  }, [state, busy, dialog]);

  const close = async () => {
    if (dialog?.kind === 'update' && state?.update.phase) return;
    if (busyRef.current && dialog?.kind !== 'login') return;
    if (dialog?.kind === 'login') {
      canceledLogin.current = dialog.id;
      try {await backend().CancelLogin(dialog.id)} catch (error) {notify('error', '取消登录失败', errorText(error)); return}
    }
    if (dialog?.kind === 'restore') {
      try {await backend().CancelRestore()} catch (error) {notify('error', '取消恢复失败', errorText(error)); return}
    }
    if (dialog?.kind === 'import') {
      try {await backend().CancelAuthImport()} catch (error) {notify('error', '取消导入失败', errorText(error)); return}
    }
    setDialog(null);
  };

  const login = async (target = '') => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy('正在打开浏览器');
    let loginID = '';
    try {
      await automatic.pause();
      const session = await backend().BeginLogin(target); loginID = session.id;
      setDialog({kind: 'login', id: session.id}); setBusy('等待浏览器登录');
      await backend().WaitLogin(session.id);
      setDialog(null); setState(await backend().RefreshAll()); notify('success', '账号已保存');
    } catch (error) {
      setDialog(null);
      if (canceledLogin.current !== loginID || !loginID) notify('error', '登录失败', errorText(error));
    } finally {busyRef.current = false; setBusy('')}
  };

  const useCredit = (account: Account, creditID = '') => void run('正在准备消费', () => backend().PrepareConsume(account.id, creditID || (pendingConsumption(account) ? account.pending_reset!.credit_id : '')), request => setDialog({kind: 'consume', account, request}));
  const showCredits = (account: Account) => {setDialog({kind: 'credits', account}); void run('正在查询重置卡', () => backend().RefreshCredits(account.id))};
  const switchAccount = (account: Account) => {
    if (state?.cli_store !== 'file') {setDialog({kind: 'switch', account}); return}
    void run('正在切换', () => backend().Switch(account.id, false), () => notify('info', '账号已切换', '重新启动 Codex CLI 后生效'));
  };

  const menu = (key: string, account: Account) => {
    switch (key) {
      case 'alias': setDialog({kind: 'alias', account}); break;
      case 'delete': if (!account.current) setDialog({kind: 'delete', account}); break;
      case 'login': void login(account.id); break;
      case 'refresh': void run('正在刷新', () => backend().RefreshAccount(account.id)); break;
      case 'credits': showCredits(account); break;
      case 'switch': switchAccount(account); break;
      case 'consume': useCredit(account); break;
      case 'export': setDialog({kind: 'export', account}); break;
    }
  };

  const importAuth = () => void run('正在导入账号', () => backend().ImportAuth(), preview => {
    if (!preview) return;
    if (preview.existing) setDialog({kind: 'import', preview});
    else notify('success', '账号已导入');
  });

  const openSettings = (tab: SettingsTab = 'general') => {if (state) setDialog({kind: 'settings', tab})};
  const refresh = () => void run('正在刷新', () => backend().RefreshAll(), setState);
  const savePreferences = (preferences: Preferences) => void run('正在保存', () => backend().SavePreferences(preferences));

  return {state, dialog, setDialog, busy, loadError, automaticError: automatic.error, run, load, close, login, importAuth, useCredit, menu, openSettings, refresh, savePreferences};
}
