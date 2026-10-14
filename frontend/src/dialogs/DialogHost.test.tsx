import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from '../app/App';
import type {State} from '../bridge/api';
import {createAccount, createState} from '../test/fixtures';

afterEach(() => {cleanup(); toast.clear(); vi.unstubAllGlobals()});

async function press(name: string) {
  await act(async () => {fireEvent.click(screen.getByRole('button', {name}))});
}

async function openMenu(name: string) {
  await press('account@example.test 操作');
  await act(async () => {fireEvent.click(screen.getByRole('menuitem', {name}))});
}

it('别名草稿不被后台推送覆盖，保存失败后可以继续编辑', async () => {
  const account = createAccount({alias: '原别名'});
  const state = createState({accounts: [account]});
  let update!: (value: State) => void;
  const Rename = vi.fn().mockRejectedValueOnce(new Error('保存失败')).mockResolvedValue(undefined);
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), Rename}}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (event: string, callback: (value: State) => void) => {if (event === 'deck:state') update = callback; return () => {}}});
  render(<App />);
  await act(async () => {});
  await press('原别名 操作');
  await act(async () => {fireEvent.click(screen.getByRole('menuitem', {name: '编辑别名'}))});
  const input = screen.getByRole('textbox', {name: '别名'}) as HTMLInputElement;
  fireEvent.change(input, {target: {value: '新别名'}});
  await act(async () => {update({...state, accounts: [{...account, alias: '远端别名'}]})});
  expect(input.value).toBe('新别名');
  await press('保存');
  expect(screen.getByRole('dialog')).toBeTruthy();
  expect(input.value).toBe('新别名');
  fireEvent.change(input, {target: {value: '最终别名'}});
  await press('保存');
  expect(Rename).toHaveBeenLastCalledWith(account.id, '最终别名');
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('等待浏览器登录时可取消，不把主动取消显示成登录失败', async () => {
  let rejectLogin!: (error: Error) => void;
  const WaitLogin = vi.fn(() => new Promise<void>((_resolve, reject) => {rejectLogin = reject}));
  const CancelLogin = vi.fn(async () => {rejectLogin(new Error('登录已取消'))});
  const RefreshAll = vi.fn();
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => createState()), BeginLogin: vi.fn(async () => ({id: 'login-session'})), WaitLogin, CancelLogin, RefreshAll}}});
  render(<App />);
  await act(async () => {});
  await press('添加账号');
  expect(screen.getByText('等待浏览器授权')).toBeTruthy();
  await press('取消');
  expect(CancelLogin).toHaveBeenCalledWith('login-session');
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.queryByText('登录失败')).toBeNull();
  expect(RefreshAll).not.toHaveBeenCalled();
});

it.each([false, true])('恢复预览隐藏身份并按需提示重新导入，移除当前账号=%s', async removesCurrent => {
  const state = createState({settings: {webdav_url: 'https://example.test/backup', webdav_username: '', webdav_password: ''}});
  state.preferences.privacy = true;
  const CancelRestore = vi.fn(async () => {});
  const Restore = vi.fn();
  const preview = {id: 'preview', accounts: 1, webdav_url: state.settings.webdav_url, modified_at: null, added: [createAccount()], removed: [], removes_current: removesCurrent};
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), PrepareRestore: vi.fn(async () => preview), CancelRestore, Restore}}});
  render(<App />);
  await act(async () => {});
  await press('设置');
  await act(async () => {fireEvent.click(screen.getByRole('tab', {name: '备份'}))});
  await press('恢复备份');
  expect(screen.getByText('新增账号')).toBeTruthy();
  expect(!!screen.queryByText('CLI 当前账号将移出列表，刷新账号或重启应用后会重新导入。')).toBe(removesCurrent);
  expect(screen.getByRole('dialog').innerHTML).not.toContain('account@example.test');
  expect(screen.getByRole('dialog').innerHTML).not.toContain('workspace');
  await press('取消');
  expect(CancelRestore).toHaveBeenCalledTimes(1);
  expect(Restore).not.toHaveBeenCalled();
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('重置卡消费失败后沿用原请求重试，查询失败时回到卡片详情', async () => {
  const account = createAccount();
  const state = createState({accounts: [account]});
  const PrepareConsume = vi.fn(async () => ({id: 'request-id', credit_id: 'credit-id', started: false, result: null}));
  const Consume = vi.fn().mockRejectedValueOnce(new Error('连接中断')).mockResolvedValue({code: 'reset', windows_reset: 1, refresh_error: '查询失败'});
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), PrepareConsume, Consume}}});
  render(<App />);
  await act(async () => {});
  await openMenu('使用重置卡');
  await press('确认使用');
  expect(screen.getByRole('dialog')).toBeTruthy();
  await press('重试原请求');
  expect(PrepareConsume).toHaveBeenCalledTimes(1);
  expect(Consume.mock.calls).toEqual([[account.id, 'request-id'], [account.id, 'request-id']]);
  expect(within(screen.getByRole('dialog')).getByRole('heading', {name: '重置卡'})).toBeTruthy();
  expect(screen.getByText('刷新失败：查询失败')).toBeTruthy();
});
