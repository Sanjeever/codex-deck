import {useRef, useState} from 'react';
import {Button, Dropdown, Spinner} from '@heroui/react';
import {privateText} from '../accounts/accounts';
import {useKeyboardShortcuts} from './useKeyboardShortcuts';
import {useDeckController} from './useDeckController';
import {Notifications} from '../notifications/Notifications';
import {DialogHost} from '../dialogs/DialogHost';
import {AccountTable} from '../accounts/AccountTable';

export default function App() {
  const {
    state, dialog, setDialog, busy, loadError, automaticError,
    run, load, close, login, importAuth, useCredit, menu, openSettings, refresh, savePreferences,
  } = useDeckController();
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const privacy = state?.preferences.privacy ?? false;
  const safeText = (value: string) => privateText(value, privacy, state?.accounts ?? []);
  useKeyboardShortcuts(!!state && !busy && !dialog, {
    refresh,
    settings: openSettings, login: () => void login(), search: () => searchRef.current?.focus(),
  });

  return <main className="app-shell">
    <header className="flex items-center justify-between gap-4 pb-8">
      <div className="flex items-baseline gap-2"><h1 className="text-xl font-semibold tracking-tight">Codex Deck</h1>{state && <span className="text-xs text-muted">v{state.build_info.version}</span>}</div>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" aria-pressed={privacy} isDisabled={!!busy || !state} onPress={() => state && savePreferences({...state.preferences, privacy: !privacy})}>隐私</Button>
        <Button variant="secondary" size="sm" aria-label={state?.auto_refreshing ? '正在自动刷新' : '刷新'} isDisabled={!!busy || !state} onPress={refresh}>{state?.auto_refreshing && <Spinner size="sm" />}刷新</Button>
        <Button variant="secondary" size="sm" isDisabled={!!busy || !state} onPress={openSettings}>设置</Button>
        <div className="flex items-center gap-1">
          <Button size="sm" isDisabled={!!busy || !state} onPress={() => void login()}>添加账号</Button>
          <Dropdown>
            <Dropdown.Trigger aria-label="添加账号方式" isDisabled={!!busy || !state} className="row-menu">▾</Dropdown.Trigger>
            <Dropdown.Popover><Dropdown.Menu aria-label="添加账号方式" onAction={key => {if (key === 'import') importAuth(); else void login()}}>
              <Dropdown.Item id="login">浏览器登录</Dropdown.Item>
              <Dropdown.Item id="import">从 auth.json 导入</Dropdown.Item>
            </Dropdown.Menu></Dropdown.Popover>
          </Dropdown>
        </div>
      </div>
    </header>
    <Notifications state={state} automaticError={automaticError} />
    {state && (automaticError || state.desktop_error || state.cli_error || state.backup.error) && <div className="mb-4 flex flex-wrap items-center gap-3 text-xs text-danger">
      {automaticError && <span title={safeText(automaticError)}>自动刷新异常</span>}
      {state.desktop_error && <span title={safeText(state.desktop_error)}>桌面功能异常</span>}
      {state.cli_error && <Button variant="ghost" size="sm" isDisabled={!!busy} onPress={refresh}><span title={safeText(state.cli_error)}>CLI 同步失败 · 重试</span></Button>}
      {state.backup.error && <Button variant="ghost" size="sm" isDisabled={!!busy} onPress={openSettings}><span title={safeText(state.backup.error)}>{state.backup.conflict ? '备份冲突 · 处理' : '备份失败 · 查看'}</span></Button>}
    </div>}
    {!state && !busy && (loadError || automaticError) && <div className="empty-state flex-col gap-4"><p role="alert" className="max-w-lg text-sm text-danger break-all">{safeText(loadError || automaticError)}</p><Button variant="secondary" onPress={() => void load()}>重试加载</Button></div>}
    {busy && !dialog && <p role="status" className="mb-5 flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
    {state && <AccountTable
      accounts={state.accounts} privacy={privacy} busy={busy}
      query={query} sort={state.preferences.sort} searchRef={searchRef} onQuery={setQuery}
      onSort={sort => savePreferences({...state.preferences, sort})}
      onAction={menu} onAdd={() => void login()}
    />}
    {state && <DialogHost
      state={state} dialog={dialog} busy={busy} run={run}
      onClose={close} setDialog={setDialog} useCredit={useCredit}
    />}
  </main>;
}
