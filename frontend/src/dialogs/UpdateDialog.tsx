import {lazy, Suspense, type MouseEvent} from 'react';
import {Button} from '@heroui/react';
import {backend} from '../bridge/api';
import {DialogFrame} from './DialogFrame';
import type {DialogProps} from './DialogHost';

const Markdown = lazy(() => import('react-markdown'));

function openExternalLink(event: MouseEvent<HTMLAnchorElement>) {
  event.preventDefault();
  const url = event.currentTarget.getAttribute('href');
  if (url) window.runtime!.BrowserOpenURL(url);
}

export function UpdateDialog({state, busy, run, onClose}: DialogProps) {
  const update = state.update;
  const progress = update.total > 0 ? `${Math.min(100, Math.round(update.downloaded / update.total * 100))}%` : `${Math.round(update.downloaded / 1024)} KB`;
  const downloading = update.phase === 'downloading';
  return <DialogFrame title="更新程序" busy={downloading ? `正在下载 · ${progress}` : update.phase ? '更新进行中' : busy} onClose={onClose} closeLabel="关闭" actions={
    downloading ? <Button variant="secondary" onPress={() => void backend().CancelUpdate()}>取消下载</Button> : <Button isDisabled={!!busy || !!update.phase || state.build_info.development} onPress={() => void run('正在准备更新', () => backend().InstallUpdate())}>下载并安装</Button>
  }>
    <p className="text-sm">v{state.build_info.version} → v{update.version}</p>
    <p className="text-xs text-muted">安装完成后自动重启。</p>
    <section aria-label="更新日志" className="release-notes">
      {update.notes.trim() ? <Suspense fallback={<p role="status" className="text-muted">正在加载更新日志…</p>}><Markdown skipHtml components={{a: ({children, href, title}) => <a href={href} title={title} target="_blank" rel="noopener noreferrer" onClick={openExternalLink}>{children}</a>}}>{update.notes}</Markdown></Suspense> : <p className="text-muted">此版本未提供更新日志</p>}
    </section>
    {update.error && <p role="alert" className="text-sm text-danger">{update.error}</p>}
    {update.url && <a href={update.url} target="_blank" rel="noreferrer" className="text-sm underline" onClick={openExternalLink}>查看发布说明与手动下载</a>}
  </DialogFrame>;
}
