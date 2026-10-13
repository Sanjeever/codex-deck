import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from './App';
import type {State} from '../bridge/api';
import {toast} from '@heroui/react';

afterEach(() => {cleanup(); vi.restoreAllMocks(); vi.useRealTimers()});

it('后端自动请求期间仍能打开设置，状态更新不关闭弹窗或覆盖输入', async () => {
  vi.useFakeTimers();
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: new Date().toISOString(), auto_refreshing: true};
  let update!: (value: State) => void;
  const api = {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), WakeAutomatic: vi.fn(async () => {}), GetAutoStart: vi.fn(async () => false)};
  vi.stubGlobal('go', {main: {App: api}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {await vi.advanceTimersByTimeAsync(0)});
    expect(screen.getByRole('button', {name: '正在自动刷新'})).toBeTruthy();
    expect((screen.getByRole('button', {name: '设置'}) as HTMLButtonElement).disabled).toBe(false);
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
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
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), SaveSettings: save}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
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

it('当前账号的删除菜单禁用，确认期间成为当前账号也禁用删除', async () => {
  const current = {id: 'current', account_id: 'workspace', email: 'current@example.test', alias: '', plan: '', current: true, auth_error: '', usage: null, credits: null, usage_error: '', credits_error: '', pending_reset: null};
  const other = {...current, id: 'other', account_id: 'other-workspace', email: 'other@example.test', current: false};
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [current, other], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
  let update!: (value: State) => void;
  const remove = vi.fn(async () => {});
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), Delete: remove}}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    fireEvent.click(screen.getByRole('button', {name: 'current@example.test 操作'}));
    expect(screen.getByRole('menuitem', {name: '删除'}).getAttribute('aria-disabled')).toBe('true');
    fireEvent.keyDown(screen.getByRole('menu'), {key: 'Escape'});
    fireEvent.click(screen.getByRole('button', {name: 'other@example.test 操作'}));
    fireEvent.click(screen.getByRole('menuitem', {name: '删除'}));
    expect((screen.getByRole('button', {name: '删除'}) as HTMLButtonElement).disabled).toBe(false);
    await act(async () => {update({...state, accounts: [{...current, current: false}, {...other, current: true}]})});
    expect(screen.getByText('当前账号不能删除')).toBeTruthy();
    expect((screen.getByRole('button', {name: '删除'}) as HTMLButtonElement).disabled).toBe(true);
    expect(remove).not.toHaveBeenCalled();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('自启动从系统读取，修改成功才更新开关，失败保留原状态', async () => {
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
  let systemEnabled = false;
  let finish!: (value: boolean) => void;
  const get = vi.fn(async () => systemEnabled);
  const set = vi.fn<(enabled: boolean) => Promise<boolean>>().mockImplementationOnce(() => new Promise<boolean>(resolve => {finish = resolve}));
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: get, SetAutoStart: set}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    const toggle = screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    await act(async () => {fireEvent.click(toggle); await Promise.resolve()});
    expect(set).toHaveBeenCalledWith(true);
    expect(toggle.checked).toBe(false);
    expect(toggle.disabled).toBe(true);
    await act(async () => {systemEnabled = true; finish(true); await Promise.resolve()});
    expect(toggle.checked).toBe(true);
    expect(screen.getByText('已开启开机自启动')).toBeTruthy();
    set.mockRejectedValueOnce(new Error('系统拒绝修改启动项'));
    await act(async () => {fireEvent.click(toggle); await Promise.resolve()});
    expect(set).toHaveBeenLastCalledWith(false);
    expect(toggle.checked).toBe(true);
    expect(screen.getByText('系统拒绝修改启动项')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', {name: '关闭'}));
    systemEnabled = false;
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    expect((screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement).checked).toBe(false);
    expect(get).toHaveBeenCalledTimes(2);
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('读取自启动状态失败可重试，不清空设置输入', async () => {
  const state: State = {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
  const get = vi.fn<() => Promise<boolean>>().mockRejectedValueOnce(new Error('读取启动项失败')).mockResolvedValue(false);
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: get}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    const toggle = screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement;
    expect(toggle.disabled).toBe(true);
    expect(screen.getByText('读取启动项失败')).toBeTruthy();
    const input = screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement;
    fireEvent.change(input, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '重试读取'})); await Promise.resolve()});
    expect(toggle.disabled).toBe(false);
    expect(input.value).toBe('https://example.test/backup/');
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});
