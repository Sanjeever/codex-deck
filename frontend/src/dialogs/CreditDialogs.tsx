import {Button} from '@heroui/react';
import {backend, type Account} from '../bridge/api';
import {accountName, creditCount, dateTime, pendingConsumption, privateText, workspaceName} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import {DialogFrame} from './DialogFrame';
import type {Dialog, DialogProps} from './DialogHost';

export function CreditDialogs({dialog, state, busy, run, onClose, setDialog, useCredit}: DialogProps & {dialog: Extract<Dialog, {kind: 'consume' | 'credits'}>; useCredit: (account: Account, creditID?: string) => void}) {
  const privacy = state.preferences.privacy;
  if (dialog.kind === 'consume') return <DialogFrame title="使用重置卡" busy={busy} onClose={onClose} actions={
    <Button isDisabled={!!busy} onPress={() => {
      setDialog({...dialog, request: {...dialog.request, started: true}});
      void run('正在使用重置卡', () => backend().Consume(dialog.account.id, dialog.request.id), result => {
        const messages: Record<string, string> = {reset: `重置成功 · ${result.windows_reset} 个窗口`, already_redeemed: '该请求已兑换', nothing_to_reset: '没有可重置窗口', no_credit: '没有可用重置卡'};
        if (result.refresh_error) {
          setDialog({kind: 'credits', account: dialog.account});
          notify('error', messages[result.code], `刷新失败：${result.refresh_error}`);
        } else {setDialog(null); notify('success', messages[result.code])}
      });
    }}>{dialog.request.started ? '重试原请求' : '确认使用'}</Button>
  }>
    <div className="space-y-2">
      <p>为 {accountName(dialog.account, privacy)} 消费一张重置卡？</p>
      <p className="text-xs text-muted break-all">{privacy ? '••••@••••' : dialog.account.email} · 工作区 {workspaceName(dialog.account.account_id, privacy)}</p>
    </div>
  </DialogFrame>;

  const account = state.accounts.find(a => a.id === dialog.account.id) ?? dialog.account;
  const count = creditCount(account);
  const pending = pendingConsumption(account);
  return <DialogFrame title="重置卡" busy={busy} onClose={onClose} closeLabel="关闭" size="lg" actions={<>
    <Button variant="secondary" isDisabled={!!busy} onPress={() => void run('正在查询重置卡', () => backend().RefreshCredits(account.id))}>刷新</Button>
    <Button isDisabled={!!busy || (count === 0 && !pending)} onPress={() => useCredit(account)}>{pending ? '重试重置卡' : '使用重置卡'}</Button>
  </>}>
    <div className="space-y-4">
      <p className="text-sm">{accountName(account, privacy)} <span className="ml-2 text-muted">{count != null ? `${count} 张可用` : '数量未知'}</span></p>
      {account.credits_error && <p className="text-sm text-danger" title={privateText(account.credits_error, privacy, state.accounts)}>查询失败</p>}
      {account.credits?.credits?.length ? <ul className="space-y-3">{account.credits.credits.map(credit => <li key={credit.id} className="credit-item">
        <div>
          <p className="text-sm">{credit.title || credit.id}</p>
          <p className="mt-1 text-xs text-muted">{({available: '可用', redeemed: '已兑换', expired: '已过期', redeeming: '兑换中'} as Record<string, string>)[credit.status] || credit.status} · {credit.expires_at ? `${dateTime(credit.expires_at)} 到期` : '未提供到期时间'}</p>
        </div>
        {credit.status === 'available' && credit.reset_type === 'codex_rate_limits' && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => useCredit(account, credit.id)}>使用</Button>}
      </li>)}</ul> : <p className="text-sm text-muted">{busy ? '正在查询' : '暂无详情'}</p>}
      {account.credits && <p className="text-xs text-muted">{account.credits_error ? '缓存' : '查询时间'} · {dateTime(account.credits.queried_at)}</p>}
    </div>
  </DialogFrame>;
}
