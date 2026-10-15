import {useRef, useState} from 'react';
import {Button, Dropdown, Spinner} from '@heroui/react';
import {privateText} from '../accounts/accounts';
import {useKeyboardShortcuts} from './useKeyboardShortcuts';
import {useDeckController} from './useDeckController';
import {Notifications} from '../notifications/Notifications';
import {DialogHost} from '../dialogs/DialogHost';
import {AccountTable} from '../accounts/AccountTable';
import {SessionBrowser} from '../sessions/SessionBrowser';
import {useSessionBrowser} from '../sessions/useSessionBrowser';
import {ErrorDetails} from './ErrorDetails';

export default function App() {
  const {
    state, dialog, setDialog, busy, loadError, automaticError,
    run, load, close, login, importAuth, useCredit, menu, openSettings, refresh, savePreferences,
  } = useDeckController();
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const [workspace, setWorkspace] = useState('accounts');
  const sessions = useSessionBrowser(workspace === 'sessions');
  const privacy = state?.preferences.privacy ?? false;
  const safeText = (value: string) => privateText(value, privacy, state?.accounts ?? []);
  useKeyboardShortcuts((!!state || workspace === 'sessions') && !busy && !dialog, {
    refresh: workspace === 'sessions' ? sessions.refresh : refresh,
    settings: openSettings, login: workspace === 'accounts' ? () => void login() : undefined,
    search: workspace === 'sessions' ? sessions.focusSearch : () => searchRef.current?.focus(),
    sessionSearch: workspace === 'sessions' ? sessions.focusSearch : undefined,
  });

  return <main className="app-shell">
    <header className="deck-header">
      <div className="deck-heading"><h1 className="text-xl font-semibold tracking-tight">Codex Deck</h1><nav aria-label="工作区" className="deck-workspaces"><Button size="sm" variant={workspace === 'accounts' ? 'secondary' : 'ghost'} aria-pressed={workspace === 'accounts'} onPress={() => setWorkspace('accounts')}>账号</Button><Button size="sm" variant={workspace === 'sessions' ? 'secondary' : 'ghost'} aria-pressed={workspace === 'sessions'} onPress={() => setWorkspace('sessions')}>会话</Button></nav></div>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" aria-pressed={privacy} isDisabled={!!busy || !state} onPress={() => state && savePreferences({...state.preferences, privacy: !privacy})}>隐私</Button>
        <Button variant="secondary" size="sm" aria-keyshortcuts="Control+r Meta+r" aria-label={workspace === 'sessions' ? '刷新会话' : state?.auto_refreshing ? '正在自动刷新' : '刷新'} isDisabled={workspace === 'sessions' ? sessions.listLoading : !!busy || !state} onPress={workspace === 'sessions' ? sessions.refresh : refresh}>{workspace === 'accounts' && state?.auto_refreshing && <Spinner size="sm" />}<span title="刷新（Ctrl / ⌘ + R）">刷新</span></Button>
        <Button variant="secondary" size="sm" aria-label="设置" aria-keyshortcuts="Control+, Meta+," isDisabled={!!busy || !state} onPress={() => openSettings(state?.update.version ? 'about' : 'general')}><span title="设置（Ctrl / ⌘ + ,）">设置</span>{state?.update.version && <span aria-label="有可用更新" title="有可用更新" className="size-1.5 rounded-full bg-accent" />}</Button>
        {workspace === 'accounts' && <div className="flex items-center gap-1">
          {!!state?.accounts.length && <Button size="sm" aria-keyshortcuts="Control+n Meta+n" isDisabled={!!busy} onPress={() => void login()}><span title="添加账号（Ctrl / ⌘ + N）">添加账号</span></Button>}
          <Dropdown>
            <Dropdown.Trigger aria-label="添加账号方式" isDisabled={!!busy || !state} className="row-menu">{!state?.accounts.length && '导入 '}▾</Dropdown.Trigger>
            <Dropdown.Popover><Dropdown.Menu aria-label="添加账号方式" onAction={key => {if (key === 'import') importAuth(); else void login()}}>
              <Dropdown.Item id="login">浏览器登录</Dropdown.Item>
              <Dropdown.Item id="import">从 auth.json 导入</Dropdown.Item>
            </Dropdown.Menu></Dropdown.Popover>
          </Dropdown>
        </div>}
      </div>
    </header>
    <Notifications state={state} />
    {state && !dialog && (automaticError || state.desktop_error || state.cli_error || state.backup.error) && <div className="mb-4 flex flex-wrap items-center gap-3 text-xs text-danger">
      {automaticError && <ErrorDetails label="自动刷新异常" message={safeText(automaticError)} busy={busy} onAction={refresh} />}
      {state.desktop_error && <ErrorDetails label="桌面功能异常" message={safeText(state.desktop_error)} busy={busy} />}
      {state.cli_error && <ErrorDetails label="CLI 同步失败" message={safeText(state.cli_error)} busy={busy} onAction={refresh} />}
      {state.backup.error && <Button variant="ghost" size="sm" isDisabled={!!busy} onPress={() => openSettings('backup')}>备份失败 · 查看</Button>}
    </div>}
    {workspace === 'accounts' && !state && !busy && (loadError || automaticError) && <div className="empty-state flex-col gap-4"><p role="alert" className="max-w-lg text-sm text-danger break-all">{safeText(loadError || automaticError)}</p><Button variant="secondary" onPress={() => void load()}>重试加载</Button></div>}
    {busy && !dialog && <p role="status" className="mb-5 flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
    {state && <div hidden={workspace !== 'accounts'}><AccountTable
      accounts={state.accounts} privacy={privacy} lowQuotaThreshold={state.preferences.low_quota_threshold} busy={busy}
      query={query} sort={state.preferences.sort} searchRef={searchRef} onQuery={setQuery}
      onSort={sort => savePreferences({...state.preferences, sort})}
      onAction={menu} onAdd={() => void login()}
    /></div>}
    {(workspace === 'sessions' || sessions.list !== null) && <SessionBrowser browser={sessions} active={workspace === 'sessions'} safeText={safeText} keyboardEnabled={!busy && !dialog} />}
    {state && <DialogHost
      state={state} dialog={dialog} busy={busy} run={run}
      onClose={close} setDialog={setDialog} useCredit={useCredit}
    />}
  </main>;
}
