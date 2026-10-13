import {SessionList} from './SessionList';
import {SessionDetail} from './SessionDetail';
import type {SessionBrowserController} from './useSessionBrowser';
import './sessions.css';

export function SessionBrowser({browser, active, safeText}: {browser: SessionBrowserController; active: boolean; safeText: (text: string) => string}) {
  return <div className="session-area" hidden={!active}>
    <div className="session-page-heading"><h2>会话</h2>{browser.list?.source && <span className="session-source"><code>{safeText(browser.list.source)}</code></span>}</div>
    <div className={`session-workspace ${browser.mobileDetail && browser.selected ? 'show-detail' : ''}`}><SessionList browser={browser} safeText={safeText} /><SessionDetail browser={browser} active={active} safeText={safeText} /></div>
  </div>;
}
