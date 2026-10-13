import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from './App';
import type {State} from './api';
import {toast} from '@heroui/react';

afterEach(() => {cleanup(); vi.restoreAllMocks(); vi.useRealTimers()});

it('后端自动请求期间仍能打开设置，状态更新不关闭弹窗或覆盖输入', async () => {
  vi.useFakeTimers();
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: new Date().toISOString(), auto_refreshing: true};
  let update!: (value: State) => void;
  const api = {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), WakeAutomatic: vi.fn(async () => {})};
  vi.stubGlobal('go', {main: {App: api}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {await vi.advanceTimersByTimeAsync(0)});
    expect(screen.getByRole('button', {name: '正在自动刷新'})).toBeTruthy();
    expect((screen.getByRole('button', {name: '设置'}) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button', {name: '设置'}));
    expect(screen.getByRole('switch', {name: '低额度通知'})).toBeTruthy();
    const address = screen.getByRole('textbox', {name: 'WebDAV 目录地址'});
    fireEvent.change(address, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {update({...state, auto_refreshing: false, next_refresh_at: new Date(Date.now() + 300_000).toISOString()}); await Promise.resolve()});
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect((address as HTMLInputElement).value).toBe('https://example.test/backup/');
    await act(async () => {await vi.advanceTimersByTimeAsync(600_000)});
    expect(api.SetAutomaticPaused).toHaveBeenLastCalledWith(true);
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('隐私模式在账号行、提示和菜单标签中隐藏邮箱与工作区', async () => {
  const account = {id: 'record', account_id: 'workspace-sensitive', email: 'private@example.test', alias: '', plan: 'plus', current: true, auth_error: '', usage: null, credits: null, usage_error: '查询 private@example.test workspace-sensitive 失败', credits_error: '', pending_reset: null};
  const state: State = {desktop_error: '', preferences: {privacy: true, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [account], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: new Date(Date.now() + 300_000).toISOString(), auto_refreshing: false};
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state)}}});
  try {
    const view = render(<App />);
    await act(async () => {await Promise.resolve()});
    expect(view.container.innerHTML).not.toContain(account.email);
    expect(view.container.innerHTML).not.toContain(account.account_id);
    expect(document.body.innerHTML).not.toContain(account.email);
    expect(document.body.innerHTML).not.toContain(account.account_id);
    expect(screen.getByRole('button', {name: '隐私'}).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', {name: '••••@•••• 操作'})).toBeTruthy();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('设置保存和失败使用 Toast，消息消失后弹窗和输入仍保留', async () => {
  vi.useFakeTimers();
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
  const save = vi.fn(async () => {});
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), SaveSettings: save}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    fireEvent.click(screen.getByRole('button', {name: '设置'}));
    const address = screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement;
    fireEvent.change(address, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存'})); await Promise.resolve()});
    expect(screen.getByRole('alertdialog', {name: '设置已保存'})).toBeTruthy();
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(address.value).toBe('https://example.test/backup/');
    await act(async () => {await vi.advanceTimersByTimeAsync(5000)});
    expect(screen.queryByText('设置已保存')).toBeNull();
    expect(screen.getByRole('dialog')).toBeTruthy();
    save.mockRejectedValueOnce(new Error('设置写入失败'));
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存'})); await Promise.resolve()});
    expect(screen.getByRole('alertdialog', {name: '操作失败'})).toBeTruthy();
    expect(screen.getByText('设置写入失败')).toBeTruthy();
    await act(async () => {await vi.advanceTimersByTimeAsync(8000)});
    expect(screen.queryByText('设置写入失败')).toBeNull();
    expect(address.value).toBe('https://example.test/backup/');
    expect(screen.getByRole('dialog')).toBeTruthy();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('配置加载失败保留错误和重试入口，重试成功后恢复界面', async () => {
  vi.useFakeTimers();
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
  const getState = vi.fn<() => Promise<State>>().mockRejectedValue(new Error('配置 JSON 损坏'));
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: getState}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {await vi.advanceTimersByTimeAsync(8000)});
    expect(screen.getByRole('alert').textContent).toBe('配置 JSON 损坏');
    getState.mockResolvedValue(state);
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '重试加载'})); await Promise.resolve()});
    expect(screen.queryByText('配置 JSON 损坏')).toBeNull();
    expect((screen.getByRole('button', {name: '设置'}) as HTMLButtonElement).disabled).toBe(false);
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});
