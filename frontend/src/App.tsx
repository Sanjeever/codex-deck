import {useCallback, useEffect, useRef, useState, type ReactNode} from 'react';
import {Button, Dropdown, Input, Label, Meter, Modal, Spinner, TextField} from '@heroui/react';
import {backend, type Account, type QuotaWindow, type ResetRequest, type RestorePreview, type Settings, type State} from './api';
import {useAutoRefresh} from './useAutoRefresh';

type Dialog = {kind: 'settings'} | {kind: 'login'; id: string} | {kind: 'alias' | 'delete' | 'switch' | 'credits'; account: Account} | {kind: 'consume'; account: Account; request: ResetRequest} | {kind: 'restore'; preview: RestorePreview};
type Notice = {kind: 'error' | 'success'; text: string};

function errorText(error: unknown) {return error instanceof Error ? error.message : String(error)}
function dateTime(value: string | null | undefined) {return value ? new Date(value).toLocaleString('zh-CN', {hour12: false}) : '未知'}
function accountName(a: Account) {return a.alias || a.email || a.account_id}
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
  const [notice, setNotice] = useState<Notice | null>(null);
  const [settings, setSettings] = useState<Settings>({webdav_url: '', webdav_username: '', webdav_password: ''});
  const [alias, setAlias] = useState('');
  const canceledLogin = useRef('');
  const stateRevision = useRef(0);
  const requestAutomatic = useCallback(async () => {
    const revision = stateRevision.current;
    const response = await backend().RefreshAutomatic();
    return {revision, response};
  }, []);
  const applyAutomatic = useCallback((result: {revision: number; response: State}) => {
    if (result.revision === stateRevision.current) setState(result.response);
  }, []);
  const automatic = useAutoRefresh(requestAutomatic, applyAutomatic, state !== null, !!busy || dialog !== null, state?.next_refresh_at ? Date.parse(state.next_refresh_at) : null);

  const run = useCallback(async <T,>(label: string, action: () => Promise<T>, done?: (result: T) => void) => {
    if (busyRef.current) return;
    stateRevision.current++;
    busyRef.current = true; setBusy(label); setNotice(null);
    try {
      const result = await action();
      setState(await backend().GetState());
      done?.(result);
    } catch (error) {
      let text = errorText(error);
      try {setState(await backend().GetState())} catch (stateError) {if (errorText(stateError) !== text) text += `；${errorText(stateError)}`}
      setNotice({kind: 'error', text});
    }
    finally {busyRef.current = false; setBusy('')}
  }, []);

  useEffect(() => {
    void run('正在加载', async () => {
      const initial = await backend().GetState(); setState(initial);
    });
  }, [run]);

  const close = async () => {
    if (busyRef.current && dialog?.kind !== 'login') return;
    if (dialog?.kind === 'login') {
      canceledLogin.current = dialog.id;
      try {await backend().CancelLogin(dialog.id)} catch (error) {setNotice({kind: 'error', text: errorText(error)}); return}
    }
    if (dialog?.kind === 'restore') {
      try {await backend().CancelRestore()} catch (error) {setNotice({kind: 'error', text: errorText(error)}); return}
    }
    setDialog(null); setNotice(null);
  };

  const login = async (target = '') => {
    if (busyRef.current) return;
    stateRevision.current++;
    busyRef.current = true; setBusy('正在打开浏览器'); setNotice(null);
    let loginID = '';
    try {
      const session = await backend().BeginLogin(target); loginID = session.id;
      setDialog({kind: 'login', id: session.id}); setBusy('等待浏览器登录');
      await backend().WaitLogin(session.id);
      setDialog(null); setState(await backend().RefreshAll()); setNotice({kind: 'success', text: '账号已保存'});
    } catch (error) {
      setDialog(null);
      if (canceledLogin.current !== loginID || !loginID) setNotice({kind: 'error', text: errorText(error)});
    } finally {busyRef.current = false; setBusy('')}
  };

  const useCredit = (account: Account, creditID = '') => void run('正在准备消费', () => backend().PrepareConsume(account.id, creditID || (pendingConsumption(account) ? account.pending_reset!.credit_id : '')), request => setDialog({kind: 'consume', account, request}));
  const showCredits = (account: Account) => {setDialog({kind: 'credits', account}); void run('正在查询重置卡', () => backend().RefreshCredits(account.id))};
  const switchAccount = (account: Account) => {
    if (state?.cli_store !== 'file') {setDialog({kind: 'switch', account}); setNotice(null); return}
    void run('正在切换', () => backend().Switch(account.id, false), () => setNotice({kind: 'success', text: '已切换，重新启动 Codex CLI 后生效'}));
  };

  const menu = (key: string, account: Account) => {
    setNotice(null);
    switch (key) {
      case 'alias': setAlias(account.alias); setDialog({kind: 'alias', account}); break;
      case 'delete': setDialog({kind: 'delete', account}); break;
      case 'login': void login(account.id); break;
      case 'refresh': void run('正在刷新', () => backend().RefreshAccount(account.id)); break;
      case 'credits': showCredits(account); break;
      case 'consume': useCredit(account); break;
    }
  };

  const activeAccount = dialog && 'account' in dialog ? state?.accounts.find(a => a.id === dialog.account.id) ?? dialog.account : null;
  const success = (text: string) => {setDialog(null); setNotice({kind: 'success', text})};
  const feedback = notice ? <p role={notice.kind === 'error' ? 'alert' : 'status'} className={`feedback ${notice.kind}`}>{notice.text}</p> : null;

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
          <Field label="WebDAV 目录地址" type="url" value={settings.webdav_url} onChange={value => setSettings({...settings, webdav_url: value})} disabled={!!busy} />
          <Field label="用户名" value={settings.webdav_username} onChange={value => setSettings({...settings, webdav_username: value})} disabled={!!busy} />
          <Field label="密码" type="password" value={settings.webdav_password} onChange={value => setSettings({...settings, webdav_password: value})} disabled={!!busy} />
          {state?.backup.at && <p className={`text-xs ${state.backup.error ? 'text-danger' : 'text-muted'}`}>{state.backup.error || `备份成功 · ${dateTime(state.backup.at)}`}</p>}
          <div className="flex gap-2 pt-1">
            <Button variant="secondary" size="sm" isDisabled={!!busy || !state?.settings.webdav_url || JSON.stringify(settings) !== JSON.stringify(state.settings)} onPress={() => void run('正在备份', () => backend().Backup(), () => setNotice({kind: 'success', text: '备份成功'}))}>备份</Button>
            <Button variant="secondary" size="sm" isDisabled={!!busy || !state?.settings.webdav_url || JSON.stringify(settings) !== JSON.stringify(state.settings)} onPress={() => void run('正在下载备份', () => backend().PrepareRestore(), preview => setDialog({kind: 'restore', preview}))}>恢复</Button>
          </div>
        </div>;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在保存设置', () => backend().SaveSettings(settings), () => setNotice({kind: 'success', text: '设置已保存'}))}>保存</Button>; break;
      case 'alias':
        title = '编辑别名'; body = <Field label="别名" value={alias} onChange={setAlias} disabled={!!busy} />;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在保存', () => backend().Rename(dialog.account.id, alias), () => success('别名已保存'))}>保存</Button>; break;
      case 'delete':
        title = '删除账号'; body = <p>删除 {accountName(dialog.account)}？</p>;
        actions = <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在删除', () => backend().Delete(dialog.account.id), () => success('账号已删除'))}>删除</Button>; break;
      case 'switch':
        title = '切换账号'; body = <p>CLI 当前使用 {state?.cli_store || '未知'} 存储。改为文件存储并切换至 {accountName(dialog.account)}？</p>;
        actions = <Button isDisabled={!!busy} onPress={() => void run('正在切换', () => backend().Switch(dialog.account.id, true), () => success('已切换，重新启动 Codex CLI 后生效'))}>确认切换</Button>; break;
      case 'restore':
        title = '恢复备份'; body = <div className="space-y-3"><p>整体替换本地账号、认证和设置？</p><p className="text-sm text-muted">备份包含 {dialog.preview.accounts} 个账号。CLI 当前账号保持不变。</p>{dialog.preview.webdav_url !== state?.settings.webdav_url && <p className="text-sm text-muted break-all">恢复后的 WebDAV 地址为 {dialog.preview.webdav_url || '未配置'}。</p>}</div>;
        actions = <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在恢复', () => backend().Restore(dialog.preview.id), () => success('备份已恢复'))}>确认替换</Button>; break;
      case 'consume':
        title = '使用重置卡'; body = <div className="space-y-2"><p>为 {accountName(dialog.account)} 消费一张重置卡？</p><p className="text-xs text-muted break-all">{dialog.account.email} · 工作区 {dialog.account.account_id}</p></div>;
        actions = <Button isDisabled={!!busy} onPress={() => {setDialog({...dialog, request: {...dialog.request, started: true}}); void run('正在使用重置卡', () => backend().Consume(dialog.account.id, dialog.request.id), result => {
          const messages: Record<string, string> = {reset: `重置成功 · ${result.windows_reset} 个窗口`, already_redeemed: '该请求已兑换', nothing_to_reset: '没有可重置窗口', no_credit: '没有可用重置卡'};
          if (result.refresh_error) {setDialog({kind: 'credits', account: dialog.account}); setNotice({kind: 'error', text: `${messages[result.code]}。刷新失败：${result.refresh_error}`})} else success(messages[result.code]);
        })}}>{dialog.request.started ? '重试原请求' : '确认使用'}</Button>; break;
      case 'credits':
        title = '重置卡';
        body = <div className="space-y-4">
          <p className="text-sm">{activeAccount && accountName(activeAccount)} <span className="ml-2 text-muted">{activeAccount && creditCount(activeAccount) != null ? `${creditCount(activeAccount)} 张可用` : '数量未知'}</span></p>
          {activeAccount?.credits_error && <p className="text-sm text-danger">{activeAccount.credits_error}</p>}
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
        <Button variant="secondary" size="sm" aria-label={automatic.refreshing ? '正在自动刷新' : '刷新'} isDisabled={!!busy || automatic.refreshing || !state} onPress={() => void run('正在刷新', () => backend().RefreshAll(), setState)}>{automatic.refreshing && <Spinner size="sm" />}刷新</Button>
        <Button variant="secondary" size="sm" isDisabled={!!busy || !state} onPress={() => {setSettings({...state!.settings});setNotice(null);setDialog({kind: 'settings'})}}>设置</Button>
        <Button size="sm" isDisabled={!!busy || !state} onPress={() => void login()}>添加账号</Button>
      </div>
    </header>
    {!dialog && feedback}
    {automatic.error && <p role="alert" className="feedback error">自动刷新失败：{automatic.error}</p>}
    {state?.cli_error && <p role="alert" className="feedback error">{state.cli_error}</p>}
    {state?.backup.error && dialog?.kind !== 'settings' && <div className="feedback error flex items-center justify-between gap-3"><span>{state.backup.error}</span><Button variant="ghost" size="sm" isDisabled={!!busy} onPress={() => void run('正在备份', () => backend().Backup())}>重试备份</Button></div>}
    {busy && !dialog && <p role="status" className="mb-5 flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
    {state && state.accounts.length === 0 ? <div className="empty-state"><Button variant="secondary" isDisabled={!!busy} onPress={() => void login()}>添加账号</Button></div> : state && <div className="table-scroll"><table aria-label="Codex 账号">
      <thead><tr><th>账号</th><th>套餐</th><th>5H 剩余</th><th>Weekly 剩余</th><th>重置卡</th><th className="text-right">操作</th></tr></thead>
      <tbody>{state.accounts.map(account => <tr key={account.id}>
        <td className="account-cell"><div className="flex items-center gap-2"><span className="font-medium break-all">{accountName(account)}</span>{account.current && <span className="current-tag">当前</span>}</div>
          {account.alias && account.email && <p className="mt-1 text-xs text-muted break-all">{account.email}</p>}
          <p className="mt-1 text-xs text-muted" title={account.account_id}>工作区 {account.account_id.slice(0, 12)}</p>
          {account.usage_error && <p className="row-error" title={account.usage_error}>{account.usage_error}</p>}
          {account.usage && <p className="mt-2 text-[11px] text-muted" title={dateTime(account.usage.queried_at)}>{account.usage_error ? '缓存' : '更新'} {new Date(account.usage.queried_at).toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit', hour12: false})}</p>}
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
        <Modal.Body className="space-y-4">{body}{dialog && feedback}{busy && dialog?.kind !== 'login' && <p role="status" className="flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}</Modal.Body>
        <Modal.Footer><Button variant="tertiary" isDisabled={!!busy && dialog?.kind !== 'login'} onPress={() => void close()}>{dialog?.kind === 'credits' || dialog?.kind === 'settings' ? '关闭' : '取消'}</Button>{actions}</Modal.Footer>
      </Modal.Dialog></Modal.Container>
    </Modal.Backdrop>
  </main>;
}
