import {useState} from 'react';
import {Button, Spinner} from '@heroui/react';
import {backend, type Account} from '../bridge/api';
import {accountName, workspaceName} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import {DialogFrame, Field} from './DialogFrame';
import type {Dialog, DialogProps} from './DialogHost';

export function AccountDialogs({dialog, ...props}: DialogProps & {dialog: Extract<Dialog, {kind: 'login' | 'alias' | 'delete' | 'switch' | 'import' | 'export'}>}) {
  const {state, busy, run, onClose, setDialog} = props;
  switch (dialog.kind) {
    case 'login':
      return <DialogFrame title="浏览器登录" busy="" onClose={onClose}>
        <div className="flex items-center gap-3 py-5"><Spinner size="sm" /><span>等待浏览器授权</span></div>
      </DialogFrame>;
    case 'alias':
      return <AliasDialog {...props} account={dialog.account} />;
    case 'import':
      return <DialogFrame title="更新账号凭据" busy={busy} onClose={onClose} actions={
        <Button isDisabled={!!busy} onPress={() => void run('正在导入账号', () => backend().ConfirmAuthImport(dialog.preview.id), () => setDialog(null))}>确认更新</Button>
      }>
        <p>更新 {accountName(dialog.preview, state.preferences.privacy)} 的登录凭据？</p>
        <p className="text-xs text-muted break-all">工作区 {workspaceName(dialog.preview.account_id, state.preferences.privacy)}</p>
      </DialogFrame>;
    case 'export':
      return <DialogFrame title="导出 auth.json" busy={busy} onClose={onClose} actions={
        <Button isDisabled={!!busy} onPress={() => void run('正在导出账号', () => backend().ExportAuth(dialog.account.id), exported => {if (exported) setDialog(null)})}>选择保存位置</Button>
      }>
        <p>导出 {accountName(dialog.account, state.preferences.privacy)} 的登录凭据？</p>
        <p className="text-xs text-muted break-all">工作区 {workspaceName(dialog.account.account_id, state.preferences.privacy)}</p>
        <p className="text-sm">文件包含登录凭据，请勿公开分享。</p>
      </DialogFrame>;
    case 'delete': {
      const account = state.accounts.find(a => a.id === dialog.account.id) ?? dialog.account;
      return <DialogFrame title="删除账号" busy={busy} onClose={onClose} actions={
        <Button variant="danger" isDisabled={!!busy || account.current} onPress={() => void run('正在删除', () => backend().Delete(dialog.account.id), () => setDialog(null))}>删除</Button>
      }>
        <p>{account.current ? '当前账号不能删除' : `删除 ${accountName(dialog.account, state.preferences.privacy)}？`}</p>
      </DialogFrame>;
    }
    case 'switch':
      return <DialogFrame title="切换账号" busy={busy} onClose={onClose} actions={
        <Button isDisabled={!!busy} onPress={() => void run('正在切换', () => backend().Switch(dialog.account.id, true), () => {setDialog(null); notify('info', '账号已切换', '重新启动 Codex CLI 后生效')})}>确认切换</Button>
      }>
        <p>CLI 当前使用 {state.cli_store || '未知'} 存储。改为文件存储并切换至 {accountName(dialog.account, state.preferences.privacy)}？</p>
      </DialogFrame>;
  }
}

function AliasDialog({account, busy, run, onClose, setDialog}: DialogProps & {account: Account}) {
  const [alias, setAlias] = useState(account.alias);
  return <DialogFrame title="编辑别名" busy={busy} onClose={onClose} actions={
    <Button isDisabled={!!busy} onPress={() => void run('正在保存', () => backend().Rename(account.id, alias), () => setDialog(null))}>保存</Button>
  }>
    <Field label="别名" value={alias} onChange={setAlias} disabled={!!busy} />
  </DialogFrame>;
}
