import {useState} from 'react';
import {Button} from '@heroui/react';
import {backend} from '../bridge/api';
import {accountName, dateTime, privateText, workspaceName} from '../accounts/accounts';
import {DialogFrame} from './DialogFrame';
import type {Dialog, DialogProps} from './DialogHost';

export function BackupDialogs(props: DialogProps & {dialog: Extract<Dialog, {kind: 'backups' | 'restore'}>}) {
  const {dialog, state, busy, run, onClose, setDialog} = props;
  if (dialog.kind === 'backups') return <BackupListDialog {...props} files={dialog.files} />;
  const {preview} = dialog;
  const privacy = state.preferences.privacy;
  const safeText = (value: string) => privateText(value, privacy, state.accounts);
  return <DialogFrame title="恢复备份" busy={busy} onClose={onClose} actions={
    <>
      <Button variant="secondary" isDisabled={!!busy} onPress={() => void run('正在返回列表', () => backend().CancelRestore(), () => setDialog({kind: 'backups', files: dialog.files}))}>返回列表</Button>
      <Button variant="danger" isDisabled={!!busy} onPress={() => void run('正在恢复', () => backend().Restore(preview.id), () => setDialog(null))}>确认替换</Button>
    </>
  }>
    <div className="space-y-3">
      <p>整体替换本地账号、认证和设置？</p>
      <p className="text-sm break-all">{safeText(preview.file.hostname) || '未知设备'} · {dateTime(preview.file.created_at || preview.file.modified_at)}</p>
      <p className="text-xs text-muted break-all">{safeText(preview.file.name)}</p>
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
      {preview.removes_current && <p className="text-sm text-muted">CLI 当前账号将移出列表，刷新账号或重启应用后会重新导入。</p>}
      {preview.webdav_url !== state.settings.webdav_url && <p className="text-sm text-muted break-all">恢复后的 WebDAV 地址为 {safeText(preview.webdav_url) || '未配置'}。</p>}
    </div>
  </DialogFrame>;
}

function BackupListDialog({files, state, busy, run, onClose, setDialog}: DialogProps & {files: Extract<Dialog, {kind: 'backups'}>['files']}) {
  const [visible, setVisible] = useState(25);
  const [error, setError] = useState('');
  const safeText = (value: string) => privateText(value, state.preferences.privacy, state.accounts);
  return <DialogFrame title="选择备份" busy={busy} onClose={onClose} actions={
    <Button variant="secondary" isDisabled={!!busy} onPress={() => {
      setError('');
      void run('正在读取备份列表', () => backend().ListBackups(), files => {setVisible(25); setDialog({kind: 'backups', files})}, setError);
    }}>刷新列表</Button>
  }>
    {error && <p role="alert" className="text-sm text-danger break-words">{safeText(error)}</p>}
    {files.length === 0 ? <p className="py-6 text-center text-sm text-muted">暂无远端备份</p> : <div className="max-h-[55vh] overflow-y-auto">
      <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_5rem] gap-3 px-3 pb-2 text-xs text-muted" aria-hidden="true"><span>时间</span><span>来源设备</span><span className="text-right">大小</span></div>
      <div className="divide-y divide-border">
        {files.slice(0, visible).map(file => <Button key={file.id} variant="ghost" isDisabled={!!busy} aria-label={`选择 ${safeText(file.hostname) || '未知设备'} 的备份，${dateTime(file.created_at || file.modified_at)}，${safeText(file.name)}`} className="h-auto w-full justify-start rounded-none px-3 py-3 text-left" onPress={() => {
          setError('');
          void run('正在下载备份', () => backend().PrepareRestore(file.id), preview => setDialog({kind: 'restore', preview, files}), setError);
        }}>
          <span className="block w-full min-w-0 space-y-1">
            <span className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_5rem] gap-3 text-sm">
              <span className="whitespace-normal tabular-nums">{dateTime(file.created_at || file.modified_at)}</span>
              <span className="truncate" title={safeText(file.hostname) || '未知设备'}>{safeText(file.hostname) || '未知设备'}</span>
              <span className="text-right text-xs text-muted tabular-nums">{file.size < 1024 ? `${file.size} B` : `${(file.size / 1024).toLocaleString('zh-CN', {maximumFractionDigits: 1})} KiB`}</span>
            </span>
            <span className="block truncate text-xs text-muted" title={safeText(file.name)}>{file.legacy ? '旧版备份 · ' : ''}{safeText(file.name)}</span>
          </span>
        </Button>)}
      </div>
      {visible < files.length && <Button variant="ghost" size="sm" className="mt-3 w-full" isDisabled={!!busy} onPress={() => setVisible(value => value + 25)}>显示更多</Button>}
    </div>}
  </DialogFrame>;
}
