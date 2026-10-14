import {useRef, type KeyboardEvent} from 'react';
import {Button, Input, Spinner, TextField} from '@heroui/react';
import {groupBy, Highlight, projectLabel, localDate, localTime, ReadIssues} from './SessionText';
import type {SessionBrowserController} from './useSessionBrowser';

export function SessionList({browser: b, safeText, keyboardEnabled}: {browser: SessionBrowserController; safeText: (text: string) => string; keyboardEnabled: boolean}) {
  const list = useRef<HTMLDivElement>(null);
  const move = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!keyboardEnabled || event.altKey || event.ctrlKey || event.metaKey || event.nativeEvent.isComposing) return;
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('.session-list-item'));
    const index = items.findIndex(item => item === event.target);
    if (index < 0) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      items[Math.max(0, Math.min(items.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))].focus();
    } else if (event.key === 'Enter') {event.preventDefault(); items[index].click()}
  };
  const groups = groupBy(b.list?.sessions ?? [], session => localDate(session.updated_at));
  const query = safeText(b.listSearch);
  return <section className="session-browser" aria-label="会话列表">
    <div className="session-browser-controls">
      <TextField aria-label="搜索全部会话" value={b.search} onChange={b.setSearch} className="session-search-field">
        <div className="session-search"><span aria-hidden="true">⌕</span><Input ref={b.searchRef} placeholder="搜索会话或内容…" autoComplete="off" aria-keyshortcuts="Control+f Meta+f Control+k Meta+k" onKeyDown={event => {if (keyboardEnabled && event.key === 'ArrowDown' && !event.altKey && !event.ctrlKey && !event.metaKey && !event.nativeEvent.isComposing) {const item = list.current?.querySelector<HTMLButtonElement>('.session-list-item[aria-pressed="true"]') ?? list.current?.querySelector<HTMLButtonElement>('.session-list-item'); if (item) {event.preventDefault(); item.focus()}}}} />{b.search ? <Button isIconOnly size="sm" variant="ghost" aria-label="清除搜索" onPress={() => b.setSearch('')}>×</Button> : <kbd>⌘ / Ctrl F</kbd>}</div>
      </TextField>
      <div className="session-filters">
        <label className="session-filter"><select aria-label="按项目筛选" value={b.project ? String((b.list?.projects ?? []).indexOf(b.project) + 1) : ''} onChange={event => b.setProject(event.target.value ? b.list?.projects[Number(event.target.value) - 1] ?? b.project : '')}><option value="">全部项目</option>{b.project && !b.list?.projects.includes(b.project) && <option value="0">{safeText(b.project)}</option>}{(b.list?.projects ?? []).map((project, index) => <option key={project} value={String(index + 1)}>{safeText(projectLabel(project, b.list?.projects ?? []))}</option>)}</select></label>
        <label className="session-filter"><select aria-label="按日期筛选" value={b.date} onChange={event => b.setDate(event.target.value)}><option value="">全部日期</option>{(b.list?.dates ?? []).map(date => <option key={date}>{date}</option>)}</select></label>
      </div>
    </div>
    <div ref={list} className="session-list" aria-busy={b.listLoading} onKeyDown={move}>
      {b.listError && <div className="session-no-results"><p role="alert">{safeText(b.listError)}</p><Button variant="secondary" size="sm" onPress={() => void b.loadList()}>重试读取</Button></div>}
      {b.listLoading && <p role="status" className="session-loading"><Spinner size="sm" />正在读取会话…</p>}
      {!b.listLoading && !b.listError && b.list && !b.list.sessions.length && <div className="session-no-results">
        <h2>{b.search || b.project || b.date ? '没有匹配的会话' : b.list.missing ? '会话目录不存在' : '暂无可读会话'}</h2>
        {(b.search || b.project || b.date) ? <Button variant="secondary" size="sm" onPress={() => {b.setSearch(''); b.setProject(''); b.setDate('')}}>清除筛选</Button> : <Button variant="secondary" size="sm" onPress={b.refresh}>刷新</Button>}
      </div>}
      {[...groups].map(([date, sessions]) => <section className="session-date-group" key={date}><h2>{date}</h2>
        {sessions.map(session => <button key={session.id} className={`session-list-item ${b.selected?.id === session.id ? 'is-selected' : ''}`} aria-pressed={b.selected?.id === session.id} onClick={() => b.select(session)}>
          <span className="session-item-top"><span className="session-item-project" title={safeText(session.project)}><Highlight text={safeText(projectLabel(session.project, b.list?.projects ?? []))} query={query} /></span><time>{localTime(session.updated_at)}</time></span>
          <span className="session-item-title"><Highlight text={safeText(session.title || '未记录用户请求')} query={query} /></span>
          {b.listSearch && session.snippet && <span className="session-item-excerpt"><Highlight text={safeText(session.snippet)} query={query} /></span>}
        </button>)}
      </section>)}
      {b.list?.next_cursor && <Button size="sm" variant="secondary" isDisabled={b.listLoading} onPress={() => void b.loadList(true)}>加载更多会话</Button>}
      <ReadIssues issues={b.list?.issues ?? []} safeText={safeText} />
    </div>
    <footer className="session-list-footer">{b.list?.source && <details className="session-source-info"><summary>来源</summary><code>{safeText(b.list.source)}</code></details>}</footer>
  </section>;
}
