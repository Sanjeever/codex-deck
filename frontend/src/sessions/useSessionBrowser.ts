import {useCallback, useEffect, useRef, useState} from 'react';
import {backend, type SessionListPage, type SessionReadPage, type SessionSummary, type SessionMatch} from '../bridge/api';

export const errorText = (error: unknown) => error instanceof Error ? error.message : String(error);
export type DisplayPage = SessionReadPage & {textStarts?: Record<string, number>};

export function useSessionBrowser(active: boolean) {
  const searchRef = useRef<HTMLInputElement>(null);
  const [search, setSearch] = useState('');
  const [project, setProject] = useState('');
  const [date, setDate] = useState('');
  const [list, setList] = useState<SessionListPage | null>(null);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState('');
  const [refreshVersion, setRefreshVersion] = useState(0);
  const listRequest = useRef(0);
  const [selected, setSelected] = useState<SessionSummary | null>(null);
  const [mobileDetail, setMobileDetail] = useState(false);
  const [filter, setFilter] = useState('all');
  const [target, setTarget] = useState<{event_id: string; text_offset: number} | null>(null);
  const [detail, setDetail] = useState<DisplayPage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const detailRequest = useRef(0);
  const id = selected?.id ?? '';
  const initialEvent = selected?.match_event_id ?? '';

  const loadList = useCallback(async (append = false) => {
    const request = ++listRequest.current;
    setListLoading(true); setListError('');
    try {
      const result = await backend().ListSessions({search, project, date, cursor: append ? list?.next_cursor ?? '' : ''});
      if (request !== listRequest.current) return;
      setList(previous => append && previous ? {...result, sessions: [...previous.sessions, ...result.sessions]} : result);
    } catch (error) {if (request === listRequest.current) setListError(errorText(error))}
    finally {if (request === listRequest.current) setListLoading(false)}
  }, [search, project, date, list?.next_cursor]);

  useEffect(() => {
    if (!active) return;
    // Invalidate immediately, before the debounce, so an older search cannot flash.
    ++listRequest.current; setListLoading(true);
    setList(previous => previous ? {...previous, sessions: [], next_cursor: ''} : previous);
    const timer = setTimeout(() => void loadList(), search ? 300 : 0);
    return () => {clearTimeout(timer); ++listRequest.current};
    // Pagination changes are handled by explicit actions, not another scan.
  }, [active, search, project, date, refreshVersion]);

  const loadDetail = useCallback(async (eventID = '', append = false, textOffset = 0) => {
    if (!id) return;
    const request = ++detailRequest.current;
    setDetailLoading(true); setDetailError('');
    try {
      const result: DisplayPage = await backend().ReadSession({id, filter, event_id: eventID, cursor: append ? detail?.next_cursor ?? '' : ''});
      if (request !== detailRequest.current) return;
      if (textOffset > 0 && eventID) {
        const offset = Math.max(0, textOffset - 40);
        const text = await backend().ReadSessionText({id, event_id: eventID, revision: result.revision, offset});
        if (request !== detailRequest.current) return;
        result.events = result.events.map(event => event.id === eventID ? {...event, text: text.text, text_cursor: text.next_offset} : event);
        result.textStarts = {[eventID]: offset};
      }
      setDetail(previous => append && previous?.revision === result.revision ? {...result, events: [...previous.events, ...result.events], textStarts: {...previous.textStarts, ...result.textStarts}} : result);
      if (eventID) requestAnimationFrame(() => document.getElementById(`event-${eventID}`)?.focus());
    } catch (error) {if (request === detailRequest.current) setDetailError(errorText(error))}
    finally {if (request === detailRequest.current) setDetailLoading(false)}
  }, [id, filter, detail?.next_cursor]);

  useEffect(() => {
    if (!active || !id) return;
    setDetail(null);
    void loadDetail(filter === 'all' ? target?.event_id ?? initialEvent : '', false, filter === 'all' ? target?.text_offset ?? 0 : 0);
    return () => {++detailRequest.current};
    // Cursor updates must not reload the first page.
  }, [active, id, filter, initialEvent, refreshVersion, target]);

  const select = (session: SessionSummary) => {setSelected(session); setFilter('all'); setTarget(null); setMobileDetail(true)};
  const focusSearch = () => {setMobileDetail(false); requestAnimationFrame(() => searchRef.current?.focus())};
  const refresh = () => setRefreshVersion(value => value + 1);
  const jump = (match: Pick<SessionMatch, 'event_id' | 'text_offset'>) => {
    // Find and turn navigation intentionally return to the unfiltered timeline.
    setFilter('all'); setTarget(match);
  };
  return {searchRef, search, setSearch, project, setProject, date, setDate, list, listLoading, listError, loadList, refresh, refreshVersion,
    selected, select, mobileDetail, setMobileDetail, focusSearch, filter, setFilter, detail, detailLoading, detailError, loadDetail, jump};
}

export type SessionBrowserController = ReturnType<typeof useSessionBrowser>;
