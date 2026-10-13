import {useEffect, useRef, useState} from 'react';
import {Button, Input, Spinner, TextField} from '@heroui/react';
import {backend, type SessionMatchPage} from '../bridge/api';
import {groupBy, Highlight, localDate, localTime, ReadIssues} from './SessionText';
import {TimelineEvent} from './TimelineEvent';
import {errorText, nextSessionRequest, type SessionBrowserController} from './useSessionBrowser';

export function SessionDetail({browser: b, active, safeText}: {browser: SessionBrowserController; active: boolean; safeText: (text: string) => string}) {
  const [findQuery, setFindQuery] = useState('');
  const [matches, setMatches] = useState<SessionMatchPage | null>(null);
  const [findError, setFindError] = useState('');
  const [finding, setFinding] = useState(false);
  const [matchIndex, setMatchIndex] = useState(-1);
  const [expanded, setExpanded] = useState(false);
  const matchRequest = useRef(0);
  const heading = useRef<HTMLHeadingElement>(null);
  const id = b.selected?.id ?? '';
  const find = async (append = false) => {
    const request = nextSessionRequest(); matchRequest.current = request;
    setFinding(true); setFindError('');
    try {
      const result = await backend().FindSessionMatches({id, search: findQuery, cursor: append ? matches?.next_cursor ?? '' : '', request_id: request});
      if (request !== matchRequest.current) return;
      const combined = append && matches ? {...result, matches: [...matches.matches, ...result.matches]} : result;
      setMatches(combined);
      if (append && result.matches.length) {setMatchIndex(matches?.matches.length ?? 0); b.jump(result.matches[0])}
    } catch (error) {if (request === matchRequest.current) setFindError(errorText(error))}
    finally {if (request === matchRequest.current) {setFinding(false); matchRequest.current = 0}}
  };
  useEffect(() => {
    matchRequest.current = 0; setMatches(null); setMatchIndex(-1); setFindError(''); setFinding(false);
    if (!id || !findQuery.trim() || !active) return;
    setFinding(true);
    const timer = setTimeout(() => void find(), 300);
    return () => {
      clearTimeout(timer);
      const request = matchRequest.current; matchRequest.current = 0;
      if (request) void backend().CancelSessionRequest('find', request).catch(error => setFindError(errorText(error)));
    };
    // Search covers the whole file independently of displayed pages or filters.
  }, [id, findQuery, active, b.refreshVersion]);
  useEffect(() => {setFindQuery(''); setExpanded(false); heading.current?.focus()}, [id]);
  const jumpMatch = (direction: number) => {
    const values = matches?.matches ?? [];
    if (!values.length) {if (matches?.next_cursor) void find(true); return}
    const next = matchIndex < 0 ? direction < 0 ? values.length - 1 : 0 : matchIndex + direction;
    if (next >= values.length && matches?.next_cursor) {void find(true); return}
    const index = (next + values.length) % values.length;
    setMatchIndex(index); b.jump(values[index]);
  };
  const page = b.detail?.session.id === id ? b.detail : null;
  const session = page?.session ?? b.selected;
  const effectiveQuery = findQuery || b.search;
  const turns = groupBy(page?.events ?? [], event => event.turn);
  const jumpTurn = (eventID: string) => {b.jump({event_id: eventID, text_offset: 0})};
  return <section className="session-detail" aria-label="会话详情" aria-busy={b.detailLoading}>
    {!session ? <div className="session-detail-empty">选择会话查看记录</div> : <>
      <header className="session-detail-heading">
        <Button size="sm" variant="ghost" className="session-mobile-back" onPress={() => {b.setMobileDetail(false); requestAnimationFrame(() => b.searchRef.current?.focus())}}>← 会话列表</Button>
        <div className="session-detail-breadcrumb">{safeText(session.project || '项目未知')}<span>/</span><span>会话记录</span></div>
        <h2 ref={heading} tabIndex={-1}>{safeText(session.title || '未记录用户请求')}</h2>
        <div className="session-detail-meta">{session.models.map(model => <span className="session-model" key={model}>{safeText(model)}</span>)}<span>{localDate(session.updated_at)} · {localTime(session.updated_at)}</span><span>{session.turn_count} 轮对话</span><span>{session.tool_count} 次工具调用</span></div>
      </header>
      <div className="session-detail-toolbar"><div className="session-event-filters" aria-label="事件筛选">{[{id: 'all', label: '全部事件'}, {id: 'chat', label: '仅对话'}, {id: 'error', label: '异常'}].map(item => <Button key={item.id} variant="ghost" size="sm" aria-pressed={b.filter === item.id} className={b.filter === item.id ? 'is-active' : ''} onPress={() => b.setFilter(item.id)}>{item.label}</Button>)}</div><Button size="sm" variant="ghost" className="session-expand" isDisabled={b.filter === 'chat'} onPress={() => setExpanded(value => !value)}>{expanded ? '折叠工具' : '展开工具'}</Button></div>
      <div className="session-find-bar">
        <TextField aria-label="查找当前会话" value={findQuery} onChange={setFindQuery}><div className="session-find-input"><Input placeholder="查找当前会话…" autoComplete="off" />{findQuery && <Button isIconOnly size="sm" variant="ghost" aria-label="清除会话查找" onPress={() => setFindQuery('')}>×</Button>}</div></TextField>
        {findQuery.trim() ? <div className="session-find-matches"><span role="status">{finding ? '查找中…' : matches?.matches.length ? `${matchIndex + 1} / ${matches.matches.length}${matches.next_cursor ? '+' : ''}` : '无匹配事件'}</span><Button isIconOnly size="sm" variant="ghost" aria-label="上一个匹配事件" isDisabled={finding || !matches?.matches.length} onPress={() => jumpMatch(-1)}>↑</Button><Button isIconOnly size="sm" variant="ghost" aria-label="下一个匹配事件" isDisabled={finding || (!matches?.matches.length && !matches?.next_cursor)} onPress={() => jumpMatch(1)}>↓</Button></div> : <label className="session-turn-jump"><select aria-label="跳到对话轮次" value="" onChange={event => jumpTurn(event.target.value)}><option value="" disabled>跳到轮次</option>{page?.turns.map(turn => <option value={turn.event_id} key={turn.number}>第 {turn.number} 轮</option>)}</select></label>}
      </div>
      <div className="session-timeline">
        {findError && <div role="alert" className="session-error">{safeText(findError)}<Button size="sm" variant="ghost" onPress={() => void find()}>重试查找</Button></div>}
        {matchIndex >= 0 && matches?.matches[matchIndex] && <p className="session-match-snippet"><Highlight text={safeText(matches.matches[matchIndex].snippet)} query={safeText(findQuery)} /></p>}
        {b.detailError && <div role="alert" className="session-error">{safeText(b.detailError)}<Button size="sm" variant="secondary" onPress={() => void b.loadDetail()}>重试详情</Button></div>}
        {b.detailLoading && <p role="status" className="session-loading"><Spinner size="sm" />正在读取记录…</p>}
        {!b.detailLoading && page && !page.events.length && <div className="session-no-results"><h3>{b.filter === 'error' ? '没有记录到异常' : '没有可展示的事件'}</h3></div>}
        {[...turns].map(([number, events]) => <section className="session-turn" key={number} aria-label={number ? `第 ${number} 轮对话` : '轮次前记录'}><div className="session-turn-heading"><span>{number ? `第 ${String(number).padStart(2, '0')} 轮` : '轮次前记录'}</span><time>{localTime(events[0]?.time)}</time></div>{events.map(event => <TimelineEvent key={`${event.id}-${page!.revision}-${page!.textStarts?.[event.id] ?? 0}`} event={event} sessionID={id} revision={page!.revision} textStart={page!.textStarts?.[event.id]} query={effectiveQuery} expanded={expanded} safeText={safeText} />)}</section>)}
        {page?.next_cursor ? <Button size="sm" variant="secondary" isDisabled={b.detailLoading} onPress={() => void b.loadDetail('', true)}>加载后续记录</Button> : page && !b.detailLoading && <div className="session-record-end"><span />当前读取范围结束<span /></div>}
        <ReadIssues issues={page?.issues ?? matches?.issues ?? []} safeText={safeText} />
      </div>
      <footer className="session-detail-footer"><span>本地记录</span><span>只读</span></footer>
    </>}
  </section>;
}
