import {useEffect, useRef, useState} from 'react';
import {Button} from '@heroui/react';
import {backend, type SessionEvent} from '../bridge/api';
import {copyToClipboard} from '../bridge/clipboard';
import {Highlight} from './SessionText';
import {errorText} from './useSessionBrowser';

export function TimelineEvent({event, sessionID, revision, textStart = 0, query, expanded, forceOpen = false, safeText}: {
  event: SessionEvent; sessionID: string; revision: string; textStart?: number; query: string; expanded: boolean; forceOpen?: boolean; safeText: (text: string) => string;
}) {
  const [open, setOpen] = useState(false);
  const [fragment, setFragment] = useState({text: event.text, start: textStart, next: event.text_cursor});
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [offset, setOffset] = useState<number | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [copying, setCopying] = useState(false);
  const [copyError, setCopyError] = useState('');
  const mounted = useRef(true);
  const latestSafeText = useRef(safeText);
  useEffect(() => {latestSafeText.current = safeText}, [safeText]);
  useEffect(() => {mounted.current = true; return () => {mounted.current = false}}, []);
  const copyFull = async () => {
    setCopying(true); setCopyError('');
    try {
      const parts: string[] = [];
      let offset = 0;
      do {
        const page = await backend().ReadSessionText({id: sessionID, event_id: event.id, revision, offset});
        if (!mounted.current) return;
        parts.push(page.text); offset = page.next_offset;
      } while (offset > 0);
      await copyToClipboard(latestSafeText.current(parts.join('')), '完整内容');
    } catch (error) {if (mounted.current) setCopyError(errorText(error))}
    finally {if (mounted.current) setCopying(false)}
  };
  useEffect(() => {setOpen(expanded && event.kind === 'tool' || forceOpen)}, [expanded, forceOpen, event.kind]);
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
  const partial = fragment.next > 0 || fragment.start > 0;
  const text = <>
    <div className="session-copy-actions">
      <Button size="sm" variant="ghost" isDisabled={loading || copying} onPress={() => void copyToClipboard(safeText(fragment.text), partial ? '当前片段' : '内容')}>{partial ? '复制当前片段' : '复制内容'}</Button>
      {partial && <Button size="sm" variant="ghost" isDisabled={loading || copying} onPress={() => void copyFull()}>{copying ? '正在读取完整内容…' : '复制完整内容'}</Button>}
    </div>
    {fragment.start > 0 && <p className="session-fragment-note">从第 {fragment.start + 1} 个字符显示</p>}
    <pre className="session-message-text"><Highlight text={safeText(fragment.text)} query={safeText(query)} /></pre>
    {(fragment.next > 0 || fragment.start > 0) && <div className="session-chunk-controls">
      <Button size="sm" variant="ghost" isDisabled={loading || fragment.start === 0} onPress={() => setOffset(Math.max(0, fragment.start - 8192))}>上一段内容</Button>
      <Button size="sm" variant="secondary" isDisabled={loading || !fragment.next} onPress={() => setOffset(fragment.next)}>下一段内容</Button>
    </div>}
    {error && <div role="alert">{safeText(error)}<Button variant="ghost" size="sm" onPress={() => setAttempt(value => value + 1)}>重试展开</Button></div>}
    {copyError && <p role="alert" className="session-error">{safeText(copyError)}</p>}
  </>;
  const tool = event.kind !== 'user' && event.kind !== 'assistant';
  const match = query.trim() && safeText(fragment.text).toLowerCase().includes(safeText(query.trim()).toLowerCase());
  const status = event.status === 'success' ? '成功' : event.status === 'error' ? '失败' : '状态未知';
  return <article id={`event-${event.id}`} tabIndex={-1} className={tool ? `session-tool-event ${event.status === 'error' ? 'has-error' : ''}` : `session-message session-message-${event.kind}`}>
    {tool ? <details open={open || forceOpen || !!match} onToggle={e => setOpen(e.currentTarget.open)}>
      <summary><span className="session-tool-chevron" aria-hidden="true">›</span><span>{safeText(event.kind === 'unknown' ? '未支持的记录' : event.name || '工具调用')}</span>{event.kind === 'tool' && <span className={`session-tool-status ${event.status === 'error' ? 'error' : ''}`}>{status}</span>}</summary>
      {(open || forceOpen || !!match) && <div className="session-tool-output">{event.kind === 'unknown' && <p>{safeText(event.name ?? '')}</p>}{text}</div>}
    </details> : <>
      <div className="session-message-heading"><strong>{event.kind === 'user' ? '你' : 'Codex'}</strong></div>
      {text}
    </>}
  </article>;
}
