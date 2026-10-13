import {useState} from 'react';
import {Button, Spinner} from '@heroui/react';
import {backend, type Account} from '../bridge/api';
import {accountName} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import {DialogFrame, Field} from './DialogFrame';
import type {Dialog, DialogProps} from './DialogHost';

export function AccountDialogs({dialog, ...props}: DialogProps & {dialog: Extract<Dialog, {kind: 'login' | 'alias' | 'delete' | 'switch'}>}) {
  const {state, busy, run, onClose, setDialog} = props;
  switch (dialog.kind) {
    case 'login':
      return <DialogFrame title="浏览器登录" busy="" onClose={onClose}>
        <div className="flex items-center gap-3 py-5"><Spinner size="sm" /><span>等待浏览器授权</span></div>
      </DialogFrame>;
    case 'alias':
      return <AliasDialog {...props} account={dialog.account} />;
    case 'delete': {
      const account = state.accounts.find(a => a.id === dialog.account.id) ?? dialog.account;
      return <DialogFrame title="删除账号" busy={busy} onClose={onClose} actions={
        <Button variant="danger" isDisabled={!!busy || account.current} onPress={() => void run('正在删除', () => backend().Delete(dialog.account.id), () => {setDialog(null); notify('success', '账号已删除')})}>删除</Button>
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
    <Button isDisabled={!!busy} onPress={() => void run('正在保存', () => backend().Rename(account.id, alias), () => {setDialog(null); notify('success', '别名已保存')})}>保存</Button>
  }>
    <Field label="别名" value={alias} onChange={setAlias} disabled={!!busy} />
  </DialogFrame>;
}
