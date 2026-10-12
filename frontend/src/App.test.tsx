import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from './App';
import type {State} from './api';

afterEach(() => {cleanup(); vi.restoreAllMocks(); vi.useRealTimers()});

it('自动请求期间仍能打开设置，返回结果不关闭弹窗或覆盖输入', async () => {
  vi.useFakeTimers();
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: new Date().toISOString()};
  let complete!: (value: State) => void;
  const automatic = vi.fn(() => new Promise<State>(resolve => {complete = resolve}));
  const api = {CancelAutomatic: vi.fn(async () => {}), GetState: vi.fn(async () => state), RefreshAutomatic: automatic};
  vi.stubGlobal('go', {main: {App: api}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {await vi.advanceTimersByTimeAsync(0)});
    expect(automatic).toHaveBeenCalledTimes(1);
    expect((screen.getByRole('button', {name: '设置'}) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button', {name: '设置'}));
    const address = screen.getByRole('textbox', {name: 'WebDAV 目录地址'});
    fireEvent.change(address, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {complete({...state, next_refresh_at: new Date(Date.now() + 300_000).toISOString()}); await Promise.resolve()});
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect((address as HTMLInputElement).value).toBe('https://example.test/backup/');
    await act(async () => {await vi.advanceTimersByTimeAsync(600_000)});
    expect(automatic).toHaveBeenCalledTimes(1);
  } finally {vi.unstubAllGlobals()}
});

it('隐私模式在账号行、提示和菜单标签中隐藏邮箱与工作区', async () => {
  const account = {id: 'record', account_id: 'workspace-sensitive', email: 'private@example.test', alias: '', plan: 'plus', current: true, auth_error: '', usage: null, credits: null, usage_error: '查询 private@example.test workspace-sensitive 失败', credits_error: '', pending_reset: null};
  const state: State = {desktop_error: '', preferences: {privacy: true, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [account], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: new Date(Date.now() + 300_000).toISOString()};
  vi.stubGlobal('go', {main: {App: {CancelAutomatic: vi.fn(async () => {}), GetState: vi.fn(async () => state)}}});
  try {
    const view = render(<App />);
    await act(async () => {await Promise.resolve()});
    expect(view.container.innerHTML).not.toContain(account.email);
    expect(view.container.innerHTML).not.toContain(account.account_id);
    expect(screen.getByRole('button', {name: '隐私'}).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', {name: '••••@•••• 操作'})).toBeTruthy();
  } finally {vi.unstubAllGlobals()}
});
