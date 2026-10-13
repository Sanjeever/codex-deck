import {useEffect, useState} from 'react';
import {Button} from '@heroui/react';
import {backend, type SessionEvent} from '../bridge/api';
import {Highlight, localTime} from './SessionText';
import {errorText} from './useSessionBrowser';

export function TimelineEvent({event, sessionID, revision, textStart = 0, query, expanded, safeText}: {
  event: SessionEvent; sessionID: string; revision: string; textStart?: number; query: string; expanded: boolean; safeText: (text: string) => string;
}) {
  const [open, setOpen] = useState(false);
  const [fragment, setFragment] = useState({text: event.text, start: textStart, next: event.text_cursor});
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [offset, setOffset] = useState<number | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {setOpen(expanded)}, [expanded]);
  useEffect(() => {setFragment({text: event.text, start: textStart, next: event.text_cursor}); setOffset(null); setError('')}, [event.text, event.text_cursor, textStart, revision]);
  useEffect(() => {
    if (offset === null) return;
    let current = true;
    setLoading(true); setError('');
    void backend().ReadSessionText({id: sessionID, event_id: event.id, revision, offset}).then(result => {
      if (current) setFragment({text: result.text, start: offset, next: result.next_offset});
    }).catch(error => {if (current) setError(errorText(error))}).finally(() => {if (current) setLoading(false)});
    return () => {current = false};
  }, [offset, event.id, sessionID, revision, attempt]);
  const text = <>
    {fragment.start > 0 && <p className="session-fragment-note">从第 {fragment.start + 1} 个字符显示</p>}
    <pre className="session-message-text"><Highlight text={safeText(fragment.text)} query={safeText(query)} /></pre>
    {(fragment.next > 0 || fragment.start > 0) && <div className="session-chunk-controls">
      <Button size="sm" variant="ghost" isDisabled={loading || fragment.start === 0} onPress={() => setOffset(Math.max(0, fragment.start - 8192))}>上一段内容</Button>
      <Button size="sm" variant="secondary" isDisabled={loading || !fragment.next} onPress={() => setOffset(fragment.next)}>下一段内容</Button>
    </div>}
    {error && <div role="alert">{safeText(error)}<Button variant="ghost" size="sm" onPress={() => setAttempt(value => value + 1)}>重试展开</Button></div>}
  </>;
  const tool = event.kind === 'tool' || event.kind === 'unknown';
  const match = query.trim() && safeText(fragment.text).toLowerCase().includes(safeText(query.trim()).toLowerCase());
  const status = event.status === 'success' ? '成功' : event.status === 'error' ? '失败' : '状态未知';
  return <article id={`event-${event.id}`} tabIndex={-1} className={tool ? `session-tool-event ${event.status === 'error' ? 'has-error' : ''}` : `session-message session-message-${event.kind}`}>
    {tool ? <details open={open || !!match} onToggle={e => setOpen(e.currentTarget.open)}>
      <summary><span className="session-tool-chevron" aria-hidden="true">›</span><span>{safeText(event.name || (event.kind === 'unknown' ? '未支持的事件' : '工具调用'))}</span><code>{safeText(event.text).slice(0, 100)}</code><span className={`session-tool-status ${event.status === 'error' ? 'error' : ''}`}>{status}</span></summary>
      <div className="session-tool-output">{text}</div>
    </details> : <>
      <div className="session-message-heading"><span className={`session-avatar ${event.kind === 'user' ? 'user' : ''}`}>{event.kind === 'user' ? '你' : event.kind === 'assistant' ? '>_' : '·'}</span><strong>{event.kind === 'user' ? '你' : event.kind === 'assistant' ? 'Codex' : safeText(event.name || '记录')}</strong><time>{localTime(event.time)}</time></div>
      {text}
    </>}
  </article>;
}
