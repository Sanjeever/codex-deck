import {useCallback, useEffect, useRef, useState} from 'react';
import {Button, Spinner} from '@heroui/react';
import {backend, type Account, type State} from '../bridge/api';
import {pendingConsumption, privateText} from '../accounts/accounts';
import {useKeyboardShortcuts} from './useKeyboardShortcuts';
import {useAutoRefresh} from './useAutoRefresh';
import {Notifications, notify} from '../notifications/Notifications';
import {DialogHost, type Dialog} from '../dialogs/DialogHost';
import {AccountTable} from '../accounts/AccountTable';


function errorText(error: unknown) {return error instanceof Error ? error.message : String(error)}
export default function App() {
  const [state, setState] = useState<State | null>(null);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [busy, setBusy] = useState('');
  const busyRef = useRef(false);
  const [loadError, setLoadError] = useState('');
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const privacy = state?.preferences.privacy ?? false;
  const safeText = (value: string) => privateText(value, privacy, state?.accounts ?? []);
  const canceledLogin = useRef('');
  const automatic = useAutoRefresh(state !== null, !!busy || dialog !== null);

  const run = useCallback(async <T,>(label: string, action: () => Promise<T>, done?: (result: T) => void) => {
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

  const close = async () => {
    if (busyRef.current && dialog?.kind !== 'login') return;
    if (dialog?.kind === 'login') {
      canceledLogin.current = dialog.id;
      try {await backend().CancelLogin(dialog.id)} catch (error) {notify('error', '取消登录失败', errorText(error)); return}
    }
    if (dialog?.kind === 'restore') {
      try {await backend().CancelRestore()} catch (error) {notify('error', '取消恢复失败', errorText(error)); return}
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
    }
  };

  const openSettings = () => {if (state) setDialog({kind: 'settings'})};
  useKeyboardShortcuts(!!state && !busy && !dialog, {
    refresh: () => void run('正在刷新', () => backend().RefreshAll(), setState),
    settings: openSettings, login: () => void login(), search: () => searchRef.current?.focus(),
  });

  return <main className="app-shell">
    <header className="flex items-center justify-between gap-4 pb-8">
      <h1 className="text-xl font-semibold tracking-tight">Codex Deck</h1>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" aria-pressed={privacy} isDisabled={!!busy || !state} onPress={() => state && void run('正在保存', () => backend().SavePreferences({...state.preferences, privacy: !privacy}))}>隐私</Button>
        <Button variant="secondary" size="sm" aria-label={state?.auto_refreshing ? '正在自动刷新' : '刷新'} isDisabled={!!busy || !state} onPress={() => void run('正在刷新', () => backend().RefreshAll(), setState)}>{state?.auto_refreshing && <Spinner size="sm" />}刷新</Button>
        <Button variant="secondary" size="sm" isDisabled={!!busy || !state} onPress={openSettings}>设置</Button>
        <Button size="sm" isDisabled={!!busy || !state} onPress={() => void login()}>添加账号</Button>
      </div>
    </header>
    <Notifications state={state} automaticError={automatic.error} />
    {state && (automatic.error || state.desktop_error || state.cli_error || state.backup.error) && <div className="mb-4 flex flex-wrap items-center gap-3 text-xs text-danger">
      {automatic.error && <span title={safeText(automatic.error)}>自动刷新异常</span>}
      {state.desktop_error && <span title={safeText(state.desktop_error)}>桌面功能异常</span>}
      {state.cli_error && <Button variant="ghost" size="sm" isDisabled={!!busy} onPress={() => void run('正在刷新', () => backend().RefreshAll(), setState)}><span title={safeText(state.cli_error)}>CLI 同步失败 · 重试</span></Button>}
      {state.backup.error && <Button variant="ghost" size="sm" isDisabled={!!busy} onPress={openSettings}><span title={safeText(state.backup.error)}>{state.backup.conflict ? '备份冲突 · 处理' : '备份失败 · 查看'}</span></Button>}
    </div>}
    {!state && !busy && (loadError || automatic.error) && <div className="empty-state flex-col gap-4"><p role="alert" className="max-w-lg text-sm text-danger break-all">{safeText(loadError || automatic.error)}</p><Button variant="secondary" onPress={() => void load()}>重试加载</Button></div>}
    {busy && !dialog && <p role="status" className="mb-5 flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
    {state && <AccountTable accounts={state.accounts} privacy={privacy} busy={busy} query={query} sort={state.preferences.sort} searchRef={searchRef} onQuery={setQuery} onSort={sort => void run('正在保存', () => backend().SavePreferences({...state.preferences, sort}))} onAction={menu} onAdd={() => void login()} />}
    {state && <DialogHost state={state} dialog={dialog} busy={busy} run={run} onClose={close} setDialog={setDialog} useCredit={useCredit} />}
  </main>;
}
