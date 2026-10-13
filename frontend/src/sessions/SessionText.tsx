import type {ReactNode} from 'react';
import type {SessionIssue} from '../bridge/api';

export function Highlight({text, query}: {text: string; query: string}) {
  const needle = query.trim().toLowerCase();
  if (!needle) return <>{text}</>;
  const parts: ReactNode[] = [];
  const lower = text.toLowerCase();
  let cursor = 0;
  for (let index = lower.indexOf(needle); index >= 0; index = lower.indexOf(needle, cursor)) {
    parts.push(text.slice(cursor, index), <mark key={index}>{text.slice(index, index + needle.length)}</mark>);
    cursor = index + needle.length;
  }
  parts.push(text.slice(cursor));
  return <>{parts}</>;
}

export function ReadIssues({issues, safeText}: {issues: SessionIssue[]; safeText: (text: string) => string}) {
  if (!issues.length) return null;
  return <details className="session-issues"><summary>读取提示 · {issues.length} 项</summary>
    <p>部分内容未参与展示或检索，详情见下方。</p>
    {issues.map((issue, index) => <p key={index}>{safeText(`${issue.file}${issue.line ? `:${issue.line}` : ''} · ${issue.message}`)}</p>)}
  </details>;
}

export function localDate(value?: string) {
  if (!value) return '日期未知';
  const date = new Date(value);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

export function localTime(value?: string) {return value ? new Date(value).toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit', hour12: false}) : ''}

export function groupBy<T, K>(values: T[], key: (value: T) => K) {
  const groups = new Map<K, T[]>();
  for (const value of values) {const group = key(value); const items = groups.get(group); if (items) items.push(value); else groups.set(group, [value])}
  return groups;
}
