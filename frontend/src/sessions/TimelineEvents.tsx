import {useEffect, useState, type ReactNode} from 'react';
import type {SessionEvent} from '../bridge/api';
import type {DisplayPage} from './useSessionBrowser';
import {TimelineEvent} from './TimelineEvent';

type TimelineProps = {events: SessionEvent[]; page: DisplayPage; sessionID: string; query: string; expanded: boolean; focusEventID: string; safeText: (text: string) => string};

function ToolGroup({events, expanded, focused, children}: {events: SessionEvent[]; expanded: boolean; focused: boolean; children: ReactNode}) {
  const [open, setOpen] = useState(expanded || focused);
  useEffect(() => {setOpen(expanded || focused)}, [expanded, focused]);
  const failures = events.filter(event => event.status === 'error').length;
  return <details className="session-tool-group" open={open} onToggle={event => setOpen(event.currentTarget.open)}>
    <summary><span aria-hidden="true">›</span>工具调用<span>{events.length} 次</span>{failures > 0 && <span className="session-error">{failures} 次失败</span>}</summary>
    <div className="session-tool-group-content">{children}</div>
  </details>;
}

export function TimelineEvents({events, page, sessionID, query, expanded, focusEventID, safeText}: TimelineProps) {
  const groups: SessionEvent[][] = [];
  for (const event of events) {
    const last = groups.at(-1);
    if (event.kind === 'tool' && last?.[0].kind === 'tool') last.push(event);
    else groups.push([event]);
  }
  const renderEvent = (event: SessionEvent) => <TimelineEvent key={`${event.id}-${page.revision}-${page.textStarts?.[event.id] ?? 0}`} event={event} sessionID={sessionID} revision={page.revision} textStart={page.textStarts?.[event.id]} query={query} expanded={expanded} forceOpen={event.id === focusEventID} safeText={safeText} />;
  const needle = safeText(query.trim()).toLowerCase();
  return groups.map(group => group[0].kind === 'tool'
    ? <ToolGroup key={`${sessionID}-${page.revision}-${group[0].id}`} events={group} expanded={expanded} focused={group.some(event => event.id === focusEventID || !!needle && safeText(`${event.name}\n${event.text}`).toLowerCase().includes(needle))}>{group.map(renderEvent)}</ToolGroup>
    : renderEvent(group[0]));
}
