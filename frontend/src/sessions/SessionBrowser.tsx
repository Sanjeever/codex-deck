import {SessionList} from './SessionList';
import {SessionDetail} from './SessionDetail';
import type {SessionBrowserController} from './useSessionBrowser';
import type {SessionSummary} from '../bridge/api';
import './sessions.css';

export function SessionBrowser({browser, active, safeText, keyboardEnabled = true, onResume}: {browser: SessionBrowserController; active: boolean; safeText: (text: string) => string; keyboardEnabled?: boolean; onResume?: (session: SessionSummary) => void}) {
  return <div className="session-area" hidden={!active}>
    <div className={`session-workspace ${browser.mobileDetail && browser.selected ? 'show-detail' : ''}`}><SessionList browser={browser} safeText={safeText} keyboardEnabled={active && keyboardEnabled} /><SessionDetail browser={browser} active={active} safeText={safeText} keyboardEnabled={keyboardEnabled} onResume={onResume} /></div>
  </div>;
}
