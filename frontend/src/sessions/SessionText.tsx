import {useState, type ReactNode} from 'react';
import type {SessionIssue} from '../bridge/api';

export function Highlight({text, query}: {text: string; query: string}) {
  const needle = query.trim();
  if (!needle) return <>{text}</>;
  const parts: ReactNode[] = [];
  const pattern = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'giu');
  let cursor = 0;
  for (const match of text.matchAll(pattern)) {
    const index = match.index;
    parts.push(text.slice(cursor, index), <mark key={index}>{match[0]}</mark>);
    cursor = index + match[0].length;
  }
  parts.push(text.slice(cursor));
  return <>{parts}</>;
}

export function ReadIssues({issues, safeText}: {issues: SessionIssue[]; safeText: (text: string) => string}) {
  const [open, setOpen] = useState(false);
  if (!issues.length) return null;
  return <details className="session-issues" open={open} onToggle={event => setOpen(event.currentTarget.open)}><summary>读取提示 · {issues.length} 项</summary>
    {open && issues.map((issue, index) => <p key={index}>{safeText(`${issue.file}${issue.line ? `:${issue.line}` : ''} · ${issue.message}`)}</p>)}
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

export function projectLabel(path: string, projects: string[]) {
  if (!path) return '项目未知';
  const parts = path.split(/[\\/]/).filter(Boolean);
  const name = parts.at(-1) ?? path;
  const repeated = projects.some(project => project !== path && project.split(/[\\/]/).filter(Boolean).at(-1) === name);
  return repeated ? parts.slice(-2).join('/') : name;
}
