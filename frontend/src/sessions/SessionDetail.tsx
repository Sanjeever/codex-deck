import {useEffect, useRef, useState} from 'react';
import {Button, Dropdown, Input, Spinner, TextField} from '@heroui/react';
import {backend, type SessionMatchPage} from '../bridge/api';
import {groupBy, localDate, localTime, projectLabel, ReadIssues} from './SessionText';
import {TimelineEvents} from './TimelineEvents';
import {copyToClipboard} from '../bridge/clipboard';
import {readConversation} from './conversation';
import {errorText, nextSessionRequest, type SessionBrowserController} from './useSessionBrowser';

export function SessionDetail({browser: b, active, safeText, keyboardEnabled}: {browser: SessionBrowserController; active: boolean; safeText: (text: string) => string; keyboardEnabled: boolean}) {
  const [findQuery, setFindQuery] = useState('');
  const [showFind, setShowFind] = useState(false);
  const [showInfo, setShowInfo] = useState(false);
  const [matches, setMatches] = useState<SessionMatchPage | null>(null);
  const [findError, setFindError] = useState('');
  const [finding, setFinding] = useState(false);
  const [matchIndex, setMatchIndex] = useState(-1);
  const [expanded, setExpanded] = useState(false);
  const [copying, setCopying] = useState(false);
  const [copyError, setCopyError] = useState('');
  const copyRequest = useRef(0);
  const latestSafeText = useRef(safeText);
  useEffect(() => {latestSafeText.current = safeText}, [safeText]);
  useEffect(() => {copyRequest.current++; setCopying(false); setCopyError(''); return () => {copyRequest.current++}}, [b.selected?.id, b.detail?.revision, active]);
  const matchRequest = useRef(0);
  const heading = useRef<HTMLHeadingElement>(null);
  const findButton = useRef<HTMLButtonElement>(null);
  const id = b.selected?.id ?? '';
  const cancelFind = () => {
    const request = matchRequest.current; matchRequest.current = 0;
    if (request) void backend().CancelSessionRequest('find', request).catch(error => setFindError(errorText(error)));
  };
  const find = async (append = false, focus = true) => {
    cancelFind();
    const request = nextSessionRequest(); matchRequest.current = request;
    setFinding(true); setFindError('');
    try {
      const result = await backend().FindSessionMatches({id, search: findQuery, cursor: append ? matches?.next_cursor ?? '' : '', request_id: request});
      if (request !== matchRequest.current) return;
      const combined = append && matches ? {...result, matches: [...matches.matches, ...result.matches]} : result;
      setMatches(combined);
      if (append && result.matches.length) {setMatchIndex(matches?.matches.length ?? 0); b.jump(result.matches[0], focus)}
    } catch (error) {if (request === matchRequest.current) setFindError(errorText(error))}
    finally {if (request === matchRequest.current) {setFinding(false); matchRequest.current = 0}}
  };
  useEffect(() => {
    cancelFind(); setMatches(null); setMatchIndex(-1); setFindError(''); setFinding(false);
    if (!id || !findQuery.trim() || !active) return;
    setFinding(true);
    const timer = setTimeout(() => void find(), 300);
    return () => {clearTimeout(timer); cancelFind()};
    // Search covers the whole file independently of displayed pages or filters.
  }, [id, findQuery, active, b.refreshVersion]);
  useEffect(() => {setFindQuery(''); setShowFind(false); setShowInfo(false); setExpanded(false); heading.current?.focus()}, [id]);
  const jumpMatch = (direction: number, focus = true) => {
    const values = matches?.matches ?? [];
    if (!values.length) {if (matches?.next_cursor) void find(true, focus); return}
    const next = matchIndex < 0 ? direction < 0 ? values.length - 1 : 0 : matchIndex + direction;
    if (next >= values.length && matches?.next_cursor) {void find(true, focus); return}
    const index = (next + values.length) % values.length;
    setMatchIndex(index); b.jump(values[index], focus);
  };
  const page = b.detail?.session.id === id ? b.detail : null;
  const session = page?.session ?? b.selected;
  const effectiveQuery = findQuery || b.selectedSearch;
  const turns = groupBy(page?.events ?? [], event => event.turn);
  const copy = async (turn: number | null = null) => {
    if (!page || copying) return;
    const request = ++copyRequest.current;
    setCopying(true); setCopyError('');
    try {
      const eventID = turn === null ? '' : page.turns.find(item => item.number === turn)?.event_id ?? '';
      const text = await readConversation(id, page.revision, turn, eventID, () => request === copyRequest.current);
      if (text !== null) await copyToClipboard(latestSafeText.current(text), turn === null ? '对话' : '本轮对话');
    } catch (error) {if (request === copyRequest.current) setCopyError(errorText(error))}
    finally {if (request === copyRequest.current) setCopying(false)}
  };
  return <section className="session-detail" aria-label="会话详情" aria-busy={b.detailLoading}>
    {!session ? <div className="session-detail-empty">选择会话查看对话</div> : <>
      <header className="session-detail-heading">
        <Button size="sm" variant="ghost" className="session-mobile-back" onPress={() => {b.setMobileDetail(false); requestAnimationFrame(() => b.searchRef.current?.focus())}}>← 会话列表</Button>
        <div className="session-detail-title"><h2 ref={heading} tabIndex={-1}>{safeText(session.title || '未记录用户请求')}</h2><Button size="sm" variant="ghost" aria-label="会话信息" aria-expanded={showInfo} onPress={() => setShowInfo(value => !value)}>信息</Button></div>
        <div className="session-detail-meta"><span title={safeText(session.project)}>{safeText(projectLabel(session.project, b.list?.projects ?? []))}</span><time>{localDate(session.updated_at)} · {localTime(session.updated_at)}</time></div>
        {showInfo && <dl className="session-info">
          <dt>项目</dt><dd>{safeText(session.project || '项目未知')}</dd>
          {session.models.length > 0 && <><dt>模型</dt><dd>{safeText(session.models.join(' / '))}</dd></>}
          <dt>记录</dt><dd>{session.turn_count} 轮对话 · {session.tool_count} 次工具调用</dd>
        </dl>}
      </header>
      <div className="session-detail-toolbar">
        <div className="session-event-filters" aria-label="事件筛选">{[{id: 'chat', label: '对话'}, {id: 'all', label: '全部记录'}, {id: 'error', label: '异常'}].map(item => <Button key={item.id} variant="ghost" size="sm" aria-pressed={b.filter === item.id} className={b.filter === item.id ? 'is-active' : ''} onPress={() => b.setFilter(item.id)}>{item.label}</Button>)}</div>
        <div className="session-detail-actions">
          <Dropdown>
            <Dropdown.Trigger aria-label="会话操作" className="row-menu" isDisabled={copying}>···</Dropdown.Trigger>
            <Dropdown.Popover><Dropdown.Menu aria-label="会话操作" onAction={key => {if (key === 'copy') void copy(); else void copyToClipboard(session.resume_command!, '恢复命令')}}>
              <Dropdown.Item id="copy" isDisabled={!page || b.detailLoading}>复制对话</Dropdown.Item>
              <Dropdown.Item id="resume" isDisabled={!session.resume_command}><span title={session.resume_command || '会话未记录有效 Codex ID'}>复制恢复命令</span></Dropdown.Item>
            </Dropdown.Menu></Dropdown.Popover>
          </Dropdown>
          {b.filter !== 'chat' && <Button size="sm" variant="ghost" className="session-expand" onPress={() => setExpanded(value => !value)}>{expanded ? '折叠工具' : '展开工具'}</Button>}
          <Dropdown>
            <Button size="sm" variant="ghost" aria-label="跳到对话轮次" isDisabled={!page?.turns.length}>轮次</Button>
            <Dropdown.Popover><Dropdown.Menu aria-label="对话轮次" onAction={key => b.jump({event_id: String(key), text_offset: 0})}>{page?.turns.map(turn => <Dropdown.Item key={turn.event_id} id={turn.event_id} textValue={`第 ${turn.number} 轮`}>第 {turn.number} 轮</Dropdown.Item>)}</Dropdown.Menu></Dropdown.Popover>
          </Dropdown>
          <Button ref={findButton} size="sm" variant="ghost" aria-expanded={showFind} onPress={() => {setShowFind(value => !value); if (showFind) setFindQuery('')}}>查找</Button>
        </div>
      </div>
      {showFind && <div className="session-find-bar">
        <TextField aria-label="查找当前会话" value={findQuery} onChange={setFindQuery}><div className="session-find-input"><Input placeholder="查找当前会话…" autoComplete="off" autoFocus onKeyDown={event => {
          if (!active || !keyboardEnabled || event.altKey || event.ctrlKey || event.metaKey || event.nativeEvent.isComposing) return;
          if (event.key === 'Escape') {event.preventDefault(); event.stopPropagation(); setShowFind(false); setFindQuery(''); findButton.current?.focus()}
          else if (event.key === 'Enter' && !finding && findQuery.trim()) {event.preventDefault(); jumpMatch(event.shiftKey ? -1 : 1, false)}
        }} />{findQuery && <Button isIconOnly size="sm" variant="ghost" aria-label="清除会话查找" onPress={() => setFindQuery('')}>×</Button>}</div></TextField>
        {findQuery.trim() && <div className="session-find-matches"><span role="status">{finding ? '查找中…' : matches?.matches.length ? `${matchIndex + 1} / ${matches.matches.length}${matches.next_cursor ? '+' : ''}` : '无匹配事件'}</span><Button isIconOnly size="sm" variant="ghost" aria-label="上一个匹配事件" isDisabled={finding || !matches?.matches.length} onPress={() => jumpMatch(-1)}>↑</Button><Button isIconOnly size="sm" variant="ghost" aria-label="下一个匹配事件" isDisabled={finding || (!matches?.matches.length && !matches?.next_cursor)} onPress={() => jumpMatch(1)}>↓</Button></div>}
      </div>}
      <div className="session-timeline">
        {copying && <p role="status" className="session-loading"><Spinner size="sm" />正在读取对话…</p>}
        {copyError && <p role="alert" className="session-error">{safeText(copyError)}</p>}
        {findError && <div role="alert" className="session-error">{safeText(findError)}<Button size="sm" variant="ghost" onPress={() => void find()}>重试查找</Button></div>}
        {b.detailError && <div role="alert" className="session-error">{safeText(b.detailError)}<Button size="sm" variant="secondary" onPress={() => void b.loadDetail()}>重试详情</Button></div>}
        {b.detailLoading && <p role="status" className="session-loading"><Spinner size="sm" />正在读取记录…</p>}
        {!b.detailLoading && page && !page.events.length && <div className="session-no-results"><h3>{b.filter === 'error' ? '没有记录到异常' : b.filter === 'chat' ? '没有可展示的对话' : '没有可展示的记录'}</h3></div>}
        {[...turns].map(([number, events]) => <section className="session-turn" key={number} aria-label={number ? `第 ${number} 轮对话` : '轮次前记录'}><div className="session-turn-heading"><span>{number ? `第 ${number} 轮` : '轮次前记录'}</span><time>{localTime(events[0]?.time)}</time>{number > 0 && <Button size="sm" variant="ghost" isDisabled={copying || b.detailLoading} onPress={() => void copy(number)}>复制本轮</Button>}</div><TimelineEvents events={events} page={page!} sessionID={id} query={effectiveQuery} expanded={expanded} focusEventID={b.focusEventID} safeText={safeText} /></section>)}
        {page?.next_cursor && <Button size="sm" variant="secondary" isDisabled={b.detailLoading} onPress={() => void b.loadDetail('', true)}>加载后续记录</Button>}
        <ReadIssues issues={page?.issues ?? matches?.issues ?? []} safeText={safeText} />
      </div>
    </>}
  </section>;
}
