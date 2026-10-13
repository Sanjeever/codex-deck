import {useCallback, useEffect, useRef, useState, type ReactNode} from 'react';
import {Button, Dropdown, Input, Label, Meter, Modal, Spinner, Switch, TextField} from '@heroui/react';
import {backend, type Account, type QuotaWindow, type ResetRequest, type RestorePreview, type Settings, type State} from './api';
import {accountName as displayAccountName, privateText, usageStale, visibleAccounts, workspaceName} from './accounts';
import {useKeyboardShortcuts} from './useKeyboardShortcuts';
import {useAutoRefresh} from './useAutoRefresh';
import {Notifications, notify} from './Notifications';

type Dialog = {kind: 'settings'} | {kind: 'overwrite'} | {kind: 'login'; id: string} | {kind: 'alias' | 'delete' | 'switch' | 'credits'; account: Account} | {kind: 'consume'; account: Account; request: ResetRequest} | {kind: 'restore'; preview: RestorePreview};

function errorText(error: unknown) {return error instanceof Error ? error.message : String(error)}
function dateTime(value: string | null | undefined) {return value ? new Date(value).toLocaleString('zh-CN', {hour12: false}) : '未知'}
function creditCount(a: Account) {
  if (a.usage?.reset_credits != null && (!a.credits || new Date(a.usage.queried_at) >= new Date(a.credits.queried_at))) return a.usage.reset_credits;
  return a.credits?.available_count ?? a.usage?.reset_credits ?? null;
}
function pendingConsumption(a: Account) {return !!a.pending_reset?.started && !a.pending_reset.result}

function Quota({label, value}: {label: string; value: QuotaWindow | null | undefined}) {
  if (!value) return <span className="text-muted">未知</span>;
  const percent = Math.round(value.remaining_percent * 10) / 10;
  return <div className="quota">
    <Meter aria-label={`${label} 剩余`} value={percent} minValue={0} maxValue={100} className="gap-2" size="sm">
      <Meter.Output className="tabular-nums text-sm">剩余 {percent}%</Meter.Output>
      <Meter.Track className="h-1"><Meter.Fill className={percent < 15 ? 'quota-low' : ''} /></Meter.Track>
    </Meter>
    <p className="mt-2 text-xs text-muted" title={dateTime(value.reset_at)}>{value.reset_at ? `${new Date(value.reset_at).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false})} 恢复` : '恢复时间未知'}</p>
  </div>;
}

function Field({label, value, onChange, type = 'text', disabled = false}: {label: string; value: string; onChange: (value: string) => void; type?: 'text' | 'password' | 'url'; disabled?: boolean}) {
  return <TextField value={value} onChange={onChange} isDisabled={disabled} className="w-full">
    <Label>{label}</Label><Input type={type} autoComplete="off" spellCheck={false} />
  </TextField>;
}

export default function App() {
  const [state, setState] = useState<State | null>(null);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [busy, setBusy] = useState('');
  const busyRef = useRef(false);
  const [loadError, setLoadError] = useState('');
  const [settings, setSettings] = useState<Settings>({webdav_url: '', webdav_username: '', webdav_password: ''});
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const privacy = state?.preferences.privacy ?? false;
  const accountName = (a: {alias: string; email: string; account_id: string}) => displayAccountName(a, privacy);
  const emailName = (value: string) => privacy ? '••••@••••' : value;
  const safeText = (value: string) => privateText(value, privacy, state?.accounts ?? []);
  const [alias, setAlias] = useState('');
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
      case 'alias': setAlias(account.alias); setDialog({kind: 'alias', account}); break;
      case 'delete': setDialog({kind: 'delete', account}); break;
      case 'login': void login(account.id); break;
      case 'refresh': void run('正在刷新', () => backend().RefreshAccount(account.id)); break;
      case 'credits': showCredits(account); break;
      case 'consume': useCredit(account); break;
    }
  };

  const openSettings = () => {if (state) {setSettings({...state.settings}); setDialog({kind: 'settings'})}};
  useKeyboardShortcuts(!!state && !busy && !dialog, {
    refresh: () => void run('正在刷新', () => backend().RefreshAll(), setState),
    settings: openSettings, login: () => void login(), search: () => searchRef.current?.focus(),
  });

  const activeAccount = dialog && 'account' in dialog ? state?.accounts.find(a => a.id === dialog.account.id) ?? dialog.account : null;
  const success = (text: string) => {setDialog(null); notify('success', text)};

  let title = '';
  let body: ReactNode = null;
  let actions: ReactNode = null;
  if (dialog) {
    switch (dialog.kind) {
      case 'login':
        title = '浏览器登录'; body = <div className="flex items-center gap-3 py-5"><Spinner size="sm" /><span>等待浏览器授权</span></div>; break;
      case 'settings':
        title = '设置';
        body = <div className="space-y-5">
          {state && <div className="flex items-center justify-between gap-4">
            <Switch isSelected={state.preferences.low_quota_notify} isDisabled={!!busy} onChange={value => void run('正在保存通知设置', () => backend().SavePreferences({...state.preferences, low_quota_notify: value}))}>
              <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>低额度通知</Label></Switch.Content>
            </Switch>
            <Dropdown><Dropdown.Trigger isDisabled={!!busy || !state.preferences.low_quota_notify} className="sort-trigger" aria-label="通知阈值">{state.preferences.low_quota_threshold || 10}% ▾</Dropdown.Trigger><Dropdown.Popover><Dropdown.Menu aria-label="通知阈值" onAction={key => void run('正在保存通知设置', () => backend().SavePreferences({...state.preferences, low_quota_threshold: Number(key)}))}>{[5, 10, 20].map(value => <Dropdown.Item key={value} id={String(value)}>{value}%</Dropdown.Item>)}</Dropdown.Menu></Dropdown.Popover></Dropdown>
          </div>}
          <Field label="WebDAV 目录地址" type="url" value={settings.webdav_url} onChange={value => setSettings({...settings, webdav_url: value})} disabled={!!busy} />
          <Field label="用户名" type={privacy ? 'password' : 'text'} value={settings.webdav_username} onChange={value => setSettings({...settings, webdav_username: value})} disabled={!!busy} />
          <Field label="密码" type="password" value={settings.webdav_password} onChange={value => setSettings({...settings, webdav_password: value})} disabled={!!busy} />
          {state?.backup.busy && <p role="status" className="text-xs text-muted">正在备份</p>}
          {state?.backup.error ? <p className="text-xs text-danger" title={safeText(state.backup.error)}>{state.backup.conflict ? '备份冲突' : '备份失败'}</p> : state?.backup.at && <p className="text-xs text-muted">上次备份 · {dateTime(state.backup.at)}</p>}
          <div className="flex gap-2 pt-1">
            {state?.backup.conflict && <Button variant="danger" size="sm" isDisabled={!!busy || JSON.stringify(settings) !== JSON.stringify(state.settings)} onPress={() => setDialog({kind: 'overwrite'})}>覆盖远端</Button>}
            <Button variant="secondary" size="sm" isDisabled={!!busy || !state?.settings.webdav_url || JSON.stringify(settings) !== JSON.stringify(state.settings)} onPress={() => void run('正在备份', () => backend().Backup(), () => notify('success', '备份成功'))}>备份</Button>
            <Button variant="secondary" size="sm" isDisabled={!!busy || !state?.settings.webdav_url || JSON.stringify(settings) !== JSON.stringify(state.settings)} onPress={() => void run('正在下载备份', () => backend().PrepareRestore(), preview => setDialog({kind: 'restore', preview}))}>恢复</Button>
          </div>
        </div>;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在保存设置', () => backend().SaveSettings(settings), () => notify('success', '设置已保存'))}>保存</Button>; break;
      case 'alias':
        title = '编辑别名'; body = <Field label="别名" value={alias} onChange={setAlias} disabled={!!busy} />;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在保存', () => backend().Rename(dialog.account.id, alias), () => success('别名已保存'))}>保存</Button>; break;
      case 'delete':
        title = '删除账号'; body = <p>删除 {accountName(dialog.account)}？</p>;
        actions = <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在删除', () => backend().Delete(dialog.account.id), () => success('账号已删除'))}>删除</Button>; break;
      case 'switch':
        title = '切换账号'; body = <p>CLI 当前使用 {state?.cli_store || '未知'} 存储。改为文件存储并切换至 {accountName(dialog.account)}？</p>;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在切换', () => backend().Switch(dialog.account.id, true), () => {setDialog(null); notify('info', '账号已切换', '重新启动 Codex CLI 后生效')})}>确认切换</Button>; break;
      case 'overwrite':
        title = '覆盖远端备份'; body = <p>用本地账号、认证和设置覆盖当前远端备份？其他设备的修改会被替换。</p>;
        actions = <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在覆盖备份', () => backend().OverwriteBackup(), () => success('备份已覆盖'))}>确认覆盖</Button>; break;
      case 'restore':
        title = '恢复备份'; body = <div className="space-y-3"><p>整体替换本地账号、认证和设置？</p><p className="text-sm text-muted">远端更新时间 · {dateTime(dialog.preview.modified_at)}</p>{dialog.preview.added.length > 0 && <div><p className="text-sm">新增账号</p>{dialog.preview.added.map(a => <p key={a.account_id + a.email} className="text-xs text-muted break-all">{accountName(a)} · {workspaceName(a.account_id, privacy)}</p>)}</div>}{dialog.preview.removed.length > 0 && <div><p className="text-sm">移除账号</p>{dialog.preview.removed.map(a => <p key={a.account_id + a.email} className="text-xs text-muted break-all">{accountName(a)} · {workspaceName(a.account_id, privacy)}</p>)}</div>}<p className="text-sm text-muted">备份包含 {dialog.preview.accounts} 个账号。CLI 当前账号保持不变。</p>{dialog.preview.webdav_url !== state?.settings.webdav_url && <p className="text-sm text-muted break-all">恢复后的 WebDAV 地址为 {dialog.preview.webdav_url || '未配置'}。</p>}</div>;
        actions = <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在恢复', () => backend().Restore(dialog.preview.id), () => success('备份已恢复'))}>确认替换</Button>; break;
      case 'consume':
        title = '使用重置卡'; body = <div className="space-y-2"><p>为 {accountName(dialog.account)} 消费一张重置卡？</p><p className="text-xs text-muted break-all">{emailName(dialog.account.email)} · 工作区 {workspaceName(dialog.account.account_id, privacy)}</p></div>;
        actions = <Button isDisabled={!!busy} onPress={() => {setDialog({...dialog, request: {...dialog.request, started: true}}); void run('正在使用重置卡', () => backend().Consume(dialog.account.id, dialog.request.id), result => {
          const messages: Record<string, string> = {reset: `重置成功 · ${result.windows_reset} 个窗口`, already_redeemed: '该请求已兑换', nothing_to_reset: '没有可重置窗口', no_credit: '没有可用重置卡'};
          if (result.refresh_error) {setDialog({kind: 'credits', account: dialog.account}); notify('error', messages[result.code], `刷新失败：${result.refresh_error}`)} else success(messages[result.code]);
        })}}>{dialog.request.started ? '重试原请求' : '确认使用'}</Button>; break;
      case 'credits':
        title = '重置卡';
        body = <div className="space-y-4">
          <p className="text-sm">{activeAccount && accountName(activeAccount)} <span className="ml-2 text-muted">{activeAccount && creditCount(activeAccount) != null ? `${creditCount(activeAccount)} 张可用` : '数量未知'}</span></p>
          {activeAccount?.credits_error && <p className="text-sm text-danger" title={safeText(activeAccount.credits_error)}>查询失败</p>}
          {activeAccount?.credits?.credits?.length ? <ul className="space-y-3">{activeAccount.credits.credits.map(credit => <li key={credit.id} className="credit-item">
            <div><p className="text-sm">{credit.title || credit.id}</p><p className="mt-1 text-xs text-muted">{({available: '可用', redeemed: '已兑换', expired: '已过期', redeeming: '兑换中'} as Record<string, string>)[credit.status] || credit.status} · {credit.expires_at ? `${dateTime(credit.expires_at)} 到期` : '未提供到期时间'}</p></div>
            {credit.status === 'available' && credit.reset_type === 'codex_rate_limits' && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => activeAccount && useCredit(activeAccount, credit.id)}>使用</Button>}
          </li>)}</ul> : <p className="text-sm text-muted">{busy ? '正在查询' : '暂无详情'}</p>}
          {activeAccount?.credits && <p className="text-xs text-muted">{activeAccount.credits_error ? '缓存' : '查询时间'} · {dateTime(activeAccount.credits.queried_at)}</p>}
        </div>;
        actions = <><Button variant="secondary" isDisabled={!!busy} onPress={() => activeAccount && void run('正在查询重置卡', () => backend().RefreshCredits(activeAccount.id))}>刷新</Button><Button isDisabled={!!busy || (activeAccount ? creditCount(activeAccount) === 0 && !pendingConsumption(activeAccount) : true)} onPress={() => activeAccount && useCredit(activeAccount)}>{activeAccount && pendingConsumption(activeAccount) ? '重试重置卡' : '使用重置卡'}</Button></>; break;
    }
  }

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
    {state && state.accounts.length > 0 && <div className="account-toolbar">
      <TextField value={query} onChange={setQuery} aria-label="搜索账号" className="max-w-64"><Input ref={searchRef} placeholder="搜索账号" autoComplete="off" /></TextField>
      <Dropdown><Dropdown.Trigger isDisabled={!!busy} className="sort-trigger">{({name: '账号名称', quota: '剩余额度'} as Record<string, string>)[state.preferences.sort] || '默认排序'} ▾</Dropdown.Trigger><Dropdown.Popover><Dropdown.Menu aria-label="账号排序" onAction={key => void run('正在保存', () => backend().SavePreferences({...state.preferences, sort: String(key) === 'default' ? '' : String(key)}))}><Dropdown.Item id="default">默认排序</Dropdown.Item><Dropdown.Item id="name">账号名称</Dropdown.Item><Dropdown.Item id="quota">剩余额度</Dropdown.Item></Dropdown.Menu></Dropdown.Popover></Dropdown>
    </div>}
    {state && state.accounts.length > 0 && visibleAccounts(state.accounts, query, state.preferences.sort).length === 0 && <p className="py-12 text-center text-muted">没有匹配账号</p>}
    {state && state.accounts.length === 0 ? <div className="empty-state"><Button variant="secondary" isDisabled={!!busy} onPress={() => void login()}>添加账号</Button></div> : state && <div className="table-scroll"><table aria-label="Codex 账号">
      <thead><tr><th>账号</th><th>套餐</th><th>5H 剩余</th><th>Weekly 剩余</th><th>重置卡</th><th className="text-right">操作</th></tr></thead>
      <tbody>{visibleAccounts(state.accounts, query, state.preferences.sort).map(account => <tr key={account.id} className={usageStale(account) ? 'usage-stale' : undefined}>
        <td className="account-cell"><div className="flex items-center gap-2"><span className="font-medium break-all">{accountName(account)}</span>{account.current && <span className="current-tag">当前</span>}</div>
          {account.alias && account.email && <p className="mt-1 text-xs text-muted break-all">{emailName(account.email)}</p>}
          <p className="mt-1 text-xs text-muted" title={workspaceName(account.account_id, privacy)}>工作区 {workspaceName(account.account_id, privacy).slice(0, 12)}</p>
          {account.auth_error && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => void login(account.id)}><span title={safeText(account.auth_error)}>需重新登录</span></Button>}
          {account.usage_error && <p className="row-error" title={safeText(account.usage_error)}>用量查询失败</p>}
          {account.usage && <p className="mt-2 text-[11px] text-muted" title={dateTime(account.usage.queried_at)}>{usageStale(account) ? '已过期' : account.usage_error ? '缓存' : '更新'} {new Date(account.usage.queried_at).toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit', hour12: false})}</p>}
        </td>
        <td><span className="plan-name">{account.plan || '未知'}</span></td>
        <td><Quota label="5H" value={account.usage?.five_hour} /></td><td><Quota label="Weekly" value={account.usage?.weekly} /></td>
        <td><Button size="sm" variant="ghost" className="px-0" isDisabled={!!busy} onPress={() => showCredits(account)}>{creditCount(account) == null ? '未知' : `${creditCount(account)} 张`}</Button></td>
        <td><div className="flex justify-end items-center gap-2">{!account.current && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => switchAccount(account)}>切换</Button>}
          <Dropdown><Dropdown.Trigger aria-label={`${accountName(account)} 操作`} isDisabled={!!busy} className="row-menu">···</Dropdown.Trigger><Dropdown.Popover><Dropdown.Menu aria-label="账号操作" onAction={key => menu(String(key), account)}>
            <Dropdown.Item id="alias">编辑别名</Dropdown.Item><Dropdown.Item id="login">重新登录</Dropdown.Item><Dropdown.Item id="refresh">刷新用量</Dropdown.Item><Dropdown.Item id="credits">重置卡详情</Dropdown.Item><Dropdown.Item id="consume" isDisabled={creditCount(account) === 0 && !pendingConsumption(account)}>{pendingConsumption(account) ? '重试重置卡' : '使用重置卡'}</Dropdown.Item><Dropdown.Item id="delete" variant="danger">删除</Dropdown.Item>
          </Dropdown.Menu></Dropdown.Popover></Dropdown>
        </div></td>
      </tr>)}</tbody>
    </table></div>}
    <Modal.Backdrop isOpen={!!dialog} onOpenChange={open => {if (!open) void close()}} isDismissable={!busy} isKeyboardDismissDisabled={!!busy && dialog?.kind !== 'login'}>
      <Modal.Container size={dialog?.kind === 'credits' ? 'lg' : 'sm'} placement="center"><Modal.Dialog>
        <Modal.Header><Modal.Heading>{title}</Modal.Heading></Modal.Header>
        <Modal.Body className="space-y-4">{body}{busy && dialog?.kind !== 'login' && <p role="status" className="flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}</Modal.Body>
        <Modal.Footer><Button variant="tertiary" isDisabled={!!busy && dialog?.kind !== 'login'} onPress={() => void close()}>{dialog?.kind === 'credits' || dialog?.kind === 'settings' ? '关闭' : '取消'}</Button>{actions}</Modal.Footer>
      </Modal.Dialog></Modal.Container>
    </Modal.Backdrop>
  </main>;
}
