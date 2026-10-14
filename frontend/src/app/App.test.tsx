import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from './App';
import type {Configuration, State} from '../bridge/api';
import {createState} from '../test/fixtures';
import {toast} from '@heroui/react';

afterEach(() => {cleanup(); vi.restoreAllMocks(); vi.useRealTimers()});

it('工作区切换保留账号搜索与设置草稿，会话刷新不暂停或调用账号接口', async () => {
  vi.useFakeTimers();
  const state = createState({accounts: [{id: 'isolated', account_id: 'workspace', email: 'test@example.test', alias: '', plan: '', current: false, auth_error: '', usage: null, credits: null, usage_error: '', credits_error: '', pending_reset: null}]});
  const api = {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), RefreshAll: vi.fn(async () => state), BeginLogin: vi.fn(), GetAutoStart: vi.fn(async () => false), ListSessions: vi.fn(async () => ({sessions: [], projects: [], dates: [], source: 'D:/isolated/sessions', missing: false, next_cursor: '', issues: []}))};
  vi.stubGlobal('go', {main: {App: api}});
  try {
    render(<App />); await act(async () => {await Promise.resolve()});
    const accountSearch = screen.getByRole('textbox', {name: '搜索账号'}) as HTMLInputElement;
    const sessionsButton = screen.getByRole('button', {name: '会话'});
    fireEvent.change(accountSearch, {target: {value: '保留搜索'}});
    const pausedCalls = api.SetAutomaticPaused.mock.calls.length, stateCalls = api.GetState.mock.calls.length;
    fireEvent.click(screen.getByRole('button', {name: '会话'})); await act(async () => {await vi.advanceTimersByTimeAsync(0)});
    expect(screen.getByText('D:/isolated/sessions')).toBeTruthy();
    fireEvent.keyDown(window, {key: 'r', ctrlKey: true}); await act(async () => {await vi.advanceTimersByTimeAsync(0)});
    fireEvent.keyDown(window, {key: 'n', ctrlKey: true});
    expect(api.SetAutomaticPaused.mock.calls.length).toBe(pausedCalls); expect(api.GetState.mock.calls.length).toBe(stateCalls); expect(api.RefreshAll).not.toHaveBeenCalled(); expect(api.BeginLogin).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {name: '账号'}));
    expect(accountSearch.value).toBe('保留搜索');
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    fireEvent.click(screen.getByRole('tab', {name: '备份'}));
    const address = screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement;
    fireEvent.change(address, {target: {value: 'https://example.test/draft/'}});
    fireEvent.click(sessionsButton);
    expect(address.value).toBe('https://example.test/draft/'); expect(screen.getByRole('dialog')).toBeTruthy();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('后端自动请求期间仍能打开设置，状态更新不关闭弹窗或覆盖输入', async () => {
  vi.useFakeTimers();
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
  const state = createState({preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 0}, next_refresh_at: new Date().toISOString(), auto_refreshing: true});
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
    await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '备份'}))});
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
  const state = createState({preferences: {privacy: true, sort: '', low_quota_notify: false, low_quota_threshold: 0}, accounts: [account], next_refresh_at: new Date(Date.now() + 300_000).toISOString()});
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state)}}});
  try {
    const view = render(<App />);
    await act(async () => {await Promise.resolve()});
    expect(view.container.innerHTML).not.toContain(account.email);
    expect(view.container.innerHTML).not.toContain(account.account_id);
    expect(document.body.innerHTML).not.toContain(account.email);
    expect(document.body.innerHTML).not.toContain(account.account_id);
    expect(screen.getByRole('button', {name: '隐私'}).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', {name: '账号 record 操作'})).toBeTruthy();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('设置保存和失败使用 Toast，错误同时保留在页内，输入仍保留', async () => {
  vi.useFakeTimers();
  let state = createState();
  const save = vi.fn(async (config: Configuration) => {
    state = {...state, settings: config.settings, network: config.network, preferences: {...state.preferences, ...config.preferences}};
    return {configuration: config, section: '', error: ''};
  });
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), SaveConfiguration: save}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '备份'}))});
    const address = screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement;
    fireEvent.change(address, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存更改'})); await Promise.resolve()});
    expect(screen.getByRole('alertdialog', {name: '设置已保存'})).toBeTruthy();
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(address.value).toBe('https://example.test/backup/');
    await act(async () => {await vi.advanceTimersByTimeAsync(5000)});
    expect(screen.queryByText('设置已保存')).toBeNull();
    expect(screen.getByRole('dialog')).toBeTruthy();
    save.mockRejectedValueOnce(new Error('设置写入失败'));
    fireEvent.change(address, {target: {value: 'https://example.test/changed/'}});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存更改'})); await Promise.resolve()});
    expect(screen.getByRole('alertdialog', {name: '操作失败'})).toBeTruthy();
    expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('设置写入失败');
    await act(async () => {await vi.advanceTimersByTimeAsync(8000)});
    expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('设置写入失败');
    expect(address.value).toBe('https://example.test/changed/');
    expect(screen.getByRole('dialog')).toBeTruthy();
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('配置加载失败保留错误和重试入口，重试成功后恢复界面', async () => {
  vi.useFakeTimers();
  const state = createState();
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
  const state = createState({accounts: [current, other]});
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

it('自启动从系统读取，编辑先保留草稿，保存失败保留待保存修改', async () => {
  const state = createState();
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
    expect(set).not.toHaveBeenCalled();
    expect(toggle.checked).toBe(true);
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存更改'}))});
    expect(set).toHaveBeenCalledWith(true);
    expect(toggle.checked).toBe(true);
    expect(toggle.disabled).toBe(true);
    await act(async () => {systemEnabled = true; finish(true); await Promise.resolve()});
    expect(toggle.checked).toBe(true);
    expect(screen.getByText('设置已保存')).toBeTruthy();
    set.mockRejectedValueOnce(new Error('系统拒绝修改启动项'));
    await act(async () => {fireEvent.click(toggle); await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '保存更改'}))});
    expect(set).toHaveBeenLastCalledWith(false);
    expect(toggle.checked).toBe(false);
    expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toContain('系统拒绝修改启动项');
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '关闭'}))});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '放弃更改'}))});
    systemEnabled = false;
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    expect((screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement).checked).toBe(false);
    expect(get).toHaveBeenCalledTimes(2);
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});

it('读取自启动状态失败可重试，不清空设置输入', async () => {
  const state = createState();
  const get = vi.fn<() => Promise<boolean>>().mockRejectedValueOnce(new Error('读取启动项失败')).mockResolvedValue(false);
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: get}}});
  try {
    render(<App />);
    await act(async () => {await Promise.resolve()});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '设置'})); await Promise.resolve()});
    const toggle = screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement;
    expect(toggle.disabled).toBe(true);
    expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('读取启动项失败');
    await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '备份'}))});
    const input = screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement;
    fireEvent.change(input, {target: {value: 'https://example.test/backup/'}});
    await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '常规'}))});
    await act(async () => {fireEvent.click(screen.getByRole('button', {name: '重试读取'})); await Promise.resolve()});
    expect((screen.getByRole('switch', {name: '开机自启动'}) as HTMLInputElement).disabled).toBe(false);
    await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '备份'}))});
    expect((screen.getByRole('textbox', {name: 'WebDAV 目录地址'}) as HTMLInputElement).value).toBe('https://example.test/backup/');
  } finally {cleanup(); toast.clear(); vi.unstubAllGlobals()}
});
