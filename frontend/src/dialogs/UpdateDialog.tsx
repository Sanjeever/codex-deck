import {Button, Spinner} from '@heroui/react';
import {backend} from '../bridge/api';
import {DialogFrame} from './DialogFrame';
import type {DialogProps} from './DialogHost';

export function UpdateDialog({state, busy, run, onClose}: DialogProps) {
  const update = state.update;
  const progress = update.total > 0 ? `${Math.min(100, Math.round(update.downloaded / update.total * 100))}%` : `${Math.round(update.downloaded / 1024)} KB`;
  const downloading = update.phase === 'downloading';
  return <DialogFrame title="发现新版本" busy={update.phase ? '更新进行中' : busy} onClose={onClose} closeLabel="稍后" actions={
    downloading ? <Button variant="secondary" onPress={() => void backend().CancelUpdate()}>取消下载</Button> : <Button isDisabled={!!busy || !!update.phase || state.build_info.development} onPress={() => void run('正在准备更新', () => backend().InstallUpdate())}>下载并安装</Button>
  }>
    <p className="text-sm">v{state.build_info.version} → v{update.version}</p>
    <p className="text-xs text-muted">安装完成后自动重启。</p>
    <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-sans text-sm">{update.notes}</pre>
    {downloading && <p role="status" className="flex gap-2 text-sm"><Spinner size="sm" />正在下载 · {progress}</p>}
    {update.error && <p role="alert" className="text-sm text-danger">{update.error}</p>}
    {update.url && <a href={update.url} target="_blank" rel="noreferrer" className="text-sm underline">查看发布说明与手动下载</a>}
  </DialogFrame>;
}
