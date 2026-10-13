import {Button, Input, Spinner, TextField} from '@heroui/react';
import {groupBy, Highlight, localDate, localTime, ReadIssues} from './SessionText';
import type {SessionBrowserController} from './useSessionBrowser';

export function SessionList({browser: b, safeText}: {browser: SessionBrowserController; safeText: (text: string) => string}) {
  const groups = groupBy(b.list?.sessions ?? [], session => localDate(session.updated_at));
  const query = safeText(b.listSearch);
  return <section className="session-browser" aria-label="会话列表">
    <div className="session-browser-controls">
      <TextField aria-label="搜索全部会话" value={b.search} onChange={b.setSearch} className="session-search-field">
        <div className="session-search"><span aria-hidden="true">⌕</span><Input ref={b.searchRef} placeholder="搜索会话或内容…" autoComplete="off" />{b.search ? <Button isIconOnly size="sm" variant="ghost" aria-label="清除搜索" onPress={() => b.setSearch('')}>×</Button> : <kbd>Ctrl / ⌘ F</kbd>}</div>
      </TextField>
      <div className="session-filters">
        <label className="session-filter"><select aria-label="按项目筛选" value={b.project ? String((b.list?.projects ?? []).indexOf(b.project) + 1) : ''} onChange={event => b.setProject(event.target.value ? b.list?.projects[Number(event.target.value) - 1] ?? b.project : '')}><option value="">全部项目</option>{b.project && !b.list?.projects.includes(b.project) && <option value="0">{safeText(b.project)}</option>}{(b.list?.projects ?? []).map((project, index) => <option key={project} value={String(index + 1)}>{safeText(project)}</option>)}</select></label>
        <label className="session-filter"><select aria-label="按日期筛选" value={b.date} onChange={event => b.setDate(event.target.value)}><option value="">全部日期</option>{(b.list?.dates ?? []).map(date => <option key={date}>{date}</option>)}</select></label>
      </div>
    </div>
    <div className="session-list" aria-busy={b.listLoading}>
      {b.listError && <div className="session-no-results"><p role="alert">{safeText(b.listError)}</p><Button variant="secondary" size="sm" onPress={() => void b.loadList()}>重试读取</Button></div>}
      {b.listLoading && <p role="status" className="session-loading"><Spinner size="sm" />正在读取会话…</p>}
      {!b.listLoading && !b.listError && b.list && !b.list.sessions.length && <div className="session-no-results">
        <h2>{b.search || b.project || b.date ? '没有匹配的会话' : b.list.missing ? '会话目录不存在' : '暂无可读会话'}</h2>
        {(b.search || b.project || b.date) ? <Button variant="secondary" size="sm" onPress={() => {b.setSearch(''); b.setProject(''); b.setDate('')}}>清除筛选</Button> : <Button variant="secondary" size="sm" onPress={b.refresh}>刷新</Button>}
      </div>}
      {[...groups].map(([date, sessions]) => <section className="session-date-group" key={date}><h2>{date}<span>{sessions.length}</span></h2>
        {sessions.map(session => <button key={session.id} className={`session-list-item ${b.selected?.id === session.id ? 'is-selected' : ''}`} aria-pressed={b.selected?.id === session.id} onClick={() => b.select(session)}>
          <span className="session-item-top"><span className="session-item-project"><Highlight text={safeText(session.project || '项目未知')} query={query} /></span><time>{localTime(session.updated_at)}</time></span>
          <span className="session-item-title"><Highlight text={safeText(session.title || '未记录用户请求')} query={query} /></span>
          {session.snippet && <span className="session-item-excerpt"><Highlight text={safeText(session.snippet)} query={query} /></span>}
          <span className="session-item-meta">{session.models.length > 0 && <span>{safeText(session.models.join(' / '))}</span>}<span>{session.turn_count} 轮</span><span>{session.tool_count} 次工具调用</span></span>
        </button>)}
      </section>)}
      {b.list?.next_cursor && <Button size="sm" variant="secondary" isDisabled={b.listLoading} onPress={() => void b.loadList(true)}>加载更多会话</Button>}
      <ReadIssues issues={b.list?.issues ?? []} safeText={safeText} />
    </div>
    <footer className="session-list-footer"><span>{b.list?.sessions.length ?? 0} 条已加载</span><span>最近优先</span></footer>
  </section>;
}
