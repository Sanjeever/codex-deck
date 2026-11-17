import {useState} from 'react';
import {Button, Label, Radio, RadioGroup} from '@heroui/react';
import {backend, type SessionSummary} from '../bridge/api';
import {accountName, privateText, usageStale, visibleAccounts, workspaceName} from '../accounts/accounts';
import {DialogFrame} from './DialogFrame';
import type {DialogProps} from './DialogHost';

export function ResumeSessionDialog({session, state, busy, run, onClose}: DialogProps & {session: SessionSummary}) {
  const [accountID, setAccountID] = useState('');
  const [result, setResult] = useState<{command: string; shell: string} | null>(null);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  const privacy = state.preferences.privacy;
  const safeText = (text: string) => privateText(text, privacy, state.accounts);
  const accounts = visibleAccounts(state.accounts, '', 'quota');
  const account = accounts.find(item => item.id === accountID);
  const switchAccount = () => {
    if (!account || account.current || account.auth_error) return;
    setError('');
    void run('正在切换账号', async () => {
      const plan = await backend().PrepareSessionResume(session.id);
      const switched = await backend().Switch(account.id, state.cli_store !== 'file');
      setResult({command: switched.cli_daemon_running ? plan.restart_command : plan.command, shell: plan.shell});
    }, undefined, setError);
  };
  const copy = async () => {
    if (!result) return;
    setError(''); setCopied(false);
    try {await navigator.clipboard.writeText(result.command); setCopied(true)}
    catch (error) {setError(error instanceof Error ? error.message : String(error))}
  };
  return <DialogFrame title={result ? '账号已切换' : '换号继续'} busy={busy} onClose={onClose} closeLabel={result ? '关闭' : '取消'} actions={
    result
      ? <Button onPress={() => void copy()} isDisabled={!!busy}>{copied ? '再次复制' : '复制续接命令'}</Button>
      : <Button onPress={switchAccount} isDisabled={!!busy || !account || account.current || !!account.auth_error}>{state.cli_store === 'file' ? '切换账号' : '改为文件存储并切换'}</Button>
  }>
    <p className="break-words text-sm">{safeText(session.title || '未记录用户请求')}</p>
    {result ? <>
      <p className="text-sm">在 {result.shell} 终端执行</p>
      <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-default p-3 text-xs"><code>{safeText(result.command)}</code></pre>
      <p role="status" className="text-xs text-muted">{copied ? '续接命令已复制' : ''}</p>
    </> : <>
      <RadioGroup aria-label="续接账号" value={accountID} onChange={setAccountID} isDisabled={!!busy} className="max-h-64 overflow-y-auto">
        {accounts.map(item => {
          const quota = [['5H', item.usage?.five_hour], ['Weekly', item.usage?.weekly]] as const;
          return <Radio key={item.id} value={item.id} isDisabled={item.current || !!item.auth_error}>
            <Radio.Content><Radio.Control><Radio.Indicator /></Radio.Control><Label className="min-w-0 break-all">
              <span>{accountName(item, privacy)}{item.current ? '（当前）' : item.auth_error ? '（需重新登录）' : ''}</span>
              <span className="block text-xs text-muted">{workspaceName(item.account_id, privacy)} · {item.usage ? `${quota.map(([label, window]) => `${label} ${window ? `${Math.round(window.remaining_percent)}%` : '未知'}`).join(' · ')}${usageStale(item) ? '（已过期）' : ''}` : '额度未知'}</span>
            </Label></Radio.Content>
          </Radio>;
        })}
      </RadioGroup>
      {!accounts.some(item => !item.current && !item.auth_error) && <p className="text-sm text-muted">没有可切换的账号，请先添加账号或重新登录。</p>}
      {state.cli_store !== 'file' && <p className="text-sm">CLI 当前使用 {state.cli_store || '未知'} 存储，切换时将改为文件存储。</p>}
    </>}
    {error && <p role="alert" className="break-all text-sm text-danger">{safeText(error)}</p>}
  </DialogFrame>;
}
