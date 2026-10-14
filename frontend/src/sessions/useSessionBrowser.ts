import {useCallback, useEffect, useRef, useState} from 'react';
import {backend, type SessionListPage, type SessionReadPage, type SessionSummary, type SessionMatch} from '../bridge/api';

let requestSequence = Date.now() * 1000;
export const nextSessionRequest = () => ++requestSequence;

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
  const [listSearch, setListSearch] = useState('');
  const loadedListKey = useRef('');
  const loadedDetailKey = useRef('');
  const [refreshVersion, setRefreshVersion] = useState(0);
  const listRequest = useRef(0);
  const [selected, setSelected] = useState<SessionSummary | null>(null);
  const [mobileDetail, setMobileDetail] = useState(false);
  const [filter, setFilter] = useState('chat');
  const [selectedSearch, setSelectedSearch] = useState('');
  const [target, setTarget] = useState<{event_id: string; text_offset: number; focus: boolean} | null>(null);
  const [detail, setDetail] = useState<DisplayPage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const detailRequest = useRef(0);
  const id = selected?.id ?? '';
  const initialEvent = selected?.match_event_id ?? '';
  const initialOffset = selected?.match_text_offset ?? 0;
  const listKey = JSON.stringify([search, project, date, refreshVersion]);
  const detailKey = JSON.stringify([id, filter, initialEvent, initialOffset, refreshVersion, target]);
  const cancelList = () => {
    const request = listRequest.current; listRequest.current = 0;
    if (request) void backend().CancelSessionRequest('list', request).catch(error => setListError(errorText(error)));
  };
  const cancelDetail = () => {
    const request = detailRequest.current; detailRequest.current = 0;
    if (request) void backend().CancelSessionRequest('detail', request).catch(error => setDetailError(errorText(error)));
  };

  const loadList = useCallback(async (append = false) => {
    cancelList();
    const request = nextSessionRequest(); listRequest.current = request;
    setListLoading(true); setListError('');
    try {
      const result = await backend().ListSessions({search, project, date, cursor: append ? list?.next_cursor ?? '' : '', request_id: request});
      if (request !== listRequest.current) return;
      loadedListKey.current = listKey; setListSearch(search);
      setList(previous => append && previous ? {...result, sessions: [...previous.sessions, ...result.sessions]} : result);
    } catch (error) {if (request === listRequest.current) setListError(errorText(error))}
    finally {if (request === listRequest.current) {setListLoading(false); listRequest.current = 0}}
  }, [search, project, date, list?.next_cursor, refreshVersion]);

  useEffect(() => {
    cancelList();
    if (!active) {setListLoading(false); return}
    if (loadedListKey.current === listKey) return;
    setListLoading(true);
    const timer = setTimeout(() => void loadList(), search ? 300 : 0);
    return () => {clearTimeout(timer); cancelList()};
    // Pagination changes are handled by explicit actions, not another scan.
  }, [active, search, project, date, refreshVersion]);

  const loadDetail = useCallback(async (eventID = '', append = false, textOffset = 0) => {
    if (!id) return;
    cancelDetail();
    const request = nextSessionRequest(); detailRequest.current = request;
    setDetailLoading(true); setDetailError('');
    try {
      const result: DisplayPage = await backend().ReadSession({id, filter, event_id: eventID, cursor: append ? detail?.next_cursor ?? '' : '', request_id: request});
      if (request !== detailRequest.current) return;
      if (textOffset > 0 && eventID) {
        const offset = Math.max(0, textOffset - 40);
        const text = await backend().ReadSessionText({id, event_id: eventID, revision: result.revision, offset});
        if (request !== detailRequest.current) return;
        result.events = result.events.map(event => event.id === eventID ? {...event, text: text.text, text_cursor: text.next_offset} : event);
        result.textStarts = {[eventID]: offset};
      }
      loadedDetailKey.current = detailKey;
      setDetail(previous => append && previous?.revision === result.revision ? {...result, events: [...previous.events, ...result.events], textStarts: {...previous.textStarts, ...result.textStarts}} : result);
      if (eventID) requestAnimationFrame(() => {
        const element = document.getElementById(`event-${eventID}`);
        if (target?.focus === false) element?.scrollIntoView({block: 'nearest'});
        else element?.focus();
      });
    } catch (error) {if (request === detailRequest.current) setDetailError(errorText(error))}
    finally {if (request === detailRequest.current) {setDetailLoading(false); detailRequest.current = 0}}
  }, [id, filter, detail?.next_cursor, refreshVersion, target, initialEvent, initialOffset]);

  useEffect(() => {
    cancelDetail();
    if (!active || !id) {setDetailLoading(false); return}
    if (loadedDetailKey.current === detailKey) return;
    void loadDetail(filter === 'all' ? target?.event_id ?? initialEvent : '', false, filter === 'all' ? target?.text_offset ?? initialOffset : 0);
    return () => cancelDetail();
    // Cursor updates must not reload the first page.
  }, [active, id, filter, initialEvent, initialOffset, refreshVersion, target]);

  const select = (session: SessionSummary) => {setSelected(session); setSelectedSearch(listSearch); setFilter(session.match_event_id ? 'all' : 'chat'); setTarget(null); setMobileDetail(true)};
  const focusSearch = () => {setMobileDetail(false); requestAnimationFrame(() => searchRef.current?.focus())};
  const refresh = () => setRefreshVersion(value => value + 1);
  const jump = (match: Pick<SessionMatch, 'event_id' | 'text_offset'>, focus = true) => {
    // Find and turn navigation intentionally return to the unfiltered timeline.
    setFilter('all'); setTarget({...match, focus});
  };
  return {searchRef, search, setSearch, project, setProject, date, setDate, list, listSearch, listLoading, listError, loadList, refresh, refreshVersion,
    selected, selectedSearch, select, focusEventID: filter === 'all' ? target?.event_id ?? initialEvent : '', mobileDetail, setMobileDetail, focusSearch, filter, setFilter, detail, detailLoading, detailError, loadDetail, jump};
}

export type SessionBrowserController = ReturnType<typeof useSessionBrowser>;
