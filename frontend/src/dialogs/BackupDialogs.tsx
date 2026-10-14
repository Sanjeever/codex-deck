import {Button} from '@heroui/react';
import {backend} from '../bridge/api';
import {accountName, dateTime, workspaceName} from '../accounts/accounts';
import {DialogFrame} from './DialogFrame';
import type {Dialog, DialogProps} from './DialogHost';

export function BackupDialogs({dialog, state, busy, run, onClose, setDialog}: DialogProps & {dialog: Extract<Dialog, {kind: 'overwrite' | 'restore'}>}) {
  if (dialog.kind === 'overwrite') return <DialogFrame title="覆盖远端备份" busy={busy} onClose={onClose} actions={
    <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在覆盖备份', () => backend().OverwriteBackup(), () => setDialog(null))}>确认覆盖</Button>
  }>
    <p>用本地账号、认证和设置覆盖当前远端备份？其他设备的修改会被替换。</p>
  </DialogFrame>;
  const {preview} = dialog;
  const privacy = state.preferences.privacy;
  return <DialogFrame title="恢复备份" busy={busy} onClose={onClose} actions={
    <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在恢复', () => backend().Restore(preview.id), () => setDialog(null))}>确认替换</Button>
  }>
    <div className="space-y-3">
      <p>整体替换本地账号、认证和设置？</p>
      <p className="text-sm text-muted">远端更新时间 · {dateTime(preview.modified_at)}</p>
      {preview.added.length > 0 && <div>
        <p className="text-sm">新增账号</p>
        {preview.added.map(a => <p key={a.account_id + a.email} className="text-xs text-muted break-all">{accountName(a, privacy)} · {workspaceName(a.account_id, privacy)}</p>)}
      </div>}
      {preview.removed.length > 0 && <div>
        <p className="text-sm">移除账号</p>
        {preview.removed.map(a => <p key={a.account_id + a.email} className="text-xs text-muted break-all">{accountName(a, privacy)} · {workspaceName(a.account_id, privacy)}</p>)}
      </div>}
      <p className="text-sm text-muted">备份包含 {preview.accounts} 个账号。CLI 当前账号保持不变。</p>
      {preview.webdav_url !== state.settings.webdav_url && <p className="text-sm text-muted break-all">恢复后的 WebDAV 地址为 {preview.webdav_url || '未配置'}。</p>}
    </div>
  </DialogFrame>;
}
