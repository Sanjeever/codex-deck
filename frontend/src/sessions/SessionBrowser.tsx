import {SessionList} from './SessionList';
import {SessionDetail} from './SessionDetail';
import type {SessionBrowserController} from './useSessionBrowser';
import './sessions.css';

export function SessionBrowser({browser, active, safeText}: {browser: SessionBrowserController; active: boolean; safeText: (text: string) => string}) {
  return <div className="session-area" hidden={!active}>
    <div className={`session-workspace ${browser.mobileDetail && browser.selected ? 'show-detail' : ''}`}><SessionList browser={browser} safeText={safeText} /><SessionDetail browser={browser} active={active} safeText={safeText} /></div>
  </div>;
}
