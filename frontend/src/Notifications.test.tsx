import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {Notifications, notify} from './Notifications';
import type {State} from './api';

function state(): State {
  return {desktop_error: '', preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10}, accounts: [], settings: {webdav_url: '', webdav_username: '', webdav_password: ''}, cli_store: 'file', cli_error: '', backup: {at: null, error: '', busy: false, conflict: false}, next_refresh_at: null, auto_refreshing: false};
}

beforeEach(() => {vi.useFakeTimers()});
afterEach(() => {cleanup(); toast.clear(); vi.restoreAllMocks(); vi.useRealTimers()});
async function tick(ms: number) {await act(async () => {await vi.advanceTimersByTimeAsync(ms)})}

it('成功消息五秒后消失，错误消息八秒后消失，也能手动关闭', async () => {
  render(<Notifications state={state()} automaticError="" />);
  act(() => {notify('success', '账号已保存'); notify('error', '操作失败', '保存失败')});
  expect(screen.getByText('账号已保存')).toBeTruthy();
  await tick(4999);
  expect(screen.getByText('账号已保存')).toBeTruthy();
  await tick(1);
  expect(screen.queryByText('账号已保存')).toBeNull();
  expect(screen.getByText('保存失败')).toBeTruthy();
  await tick(3000);
  expect(screen.queryByText('保存失败')).toBeNull();
  act(() => {notify('info', '账号已切换', '重新启动 Codex CLI 后生效')});
  fireEvent.click(screen.getByRole('button', {name: '关闭消息'}));
  expect(screen.queryByText('账号已切换')).toBeNull();
});

it('重复状态推送不重复提醒，错误恢复后再次出现可以重新提醒', async () => {
  const current = state(); current.cli_error = '无法读取 CLI 认证';
  const view = render(<Notifications state={current} automaticError="" />);
  expect(screen.getAllByText(current.cli_error)).toHaveLength(1);
  view.rerender(<Notifications state={{...current}} automaticError="" />);
  expect(screen.getAllByText(current.cli_error)).toHaveLength(1);
  await tick(8000);
  view.rerender(<Notifications state={{...current}} automaticError="" />);
  expect(screen.queryByText(current.cli_error)).toBeNull();
  view.rerender(<Notifications state={state()} automaticError="" />);
  view.rerender(<Notifications state={current} automaticError="" />);
  expect(screen.getByText(current.cli_error)).toBeTruthy();
});

it('相同操作错误与后台错误合并，不占用两条消息', () => {
  const current = state(); current.cli_error = 'CLI 认证读取失败';
  const view = render(<Notifications state={state()} automaticError="" />);
  act(() => {notify('error', '操作失败', current.cli_error)});
  view.rerender(<Notifications state={current} automaticError="" />);
  expect(screen.getAllByText(current.cli_error)).toHaveLength(1);
});

it('窗口隐藏时清理旧消息，不积压新消息或旧后台错误', () => {
  let visible: DocumentVisibilityState = 'visible';
  vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visible);
  const view = render(<Notifications state={state()} automaticError="" />);
  act(() => {notify('success', '设置已保存')});
  visible = 'hidden';
  act(() => {document.dispatchEvent(new Event('visibilitychange')); notify('success', '备份成功')});
  expect(screen.queryByText('设置已保存')).toBeNull();
  const current = state(); current.backup.error = '网络不可用';
  view.rerender(<Notifications state={current} automaticError="" />);
  visible = 'visible';
  act(() => {document.dispatchEvent(new Event('visibilitychange'))});
  view.rerender(<Notifications state={{...current}} automaticError="" />);
  expect(screen.queryByText('备份成功')).toBeNull();
  expect(screen.queryByText('网络不可用')).toBeNull();
});

it('隐私模式覆盖 Toast 正文和可访问性文字，切换隐私清理旧消息', () => {
  const current = state();
  current.preferences.privacy = true;
  current.accounts = [{id: 'account', account_id: 'workspace-secret', email: 'private@example.test', alias: '', plan: '', current: true, auth_error: '', usage: null, credits: null, usage_error: 'private@example.test workspace-secret 查询失败', credits_error: '', pending_reset: null}];
  const view = render(<Notifications state={current} automaticError="" />);
  expect(screen.getByText('••••@•••• •••••••• 查询失败')).toBeTruthy();
  expect(document.body.innerHTML).not.toContain('private@example.test');
  expect(document.body.innerHTML).not.toContain('workspace-secret');
  view.rerender(<Notifications state={{...current, preferences: {...current.preferences, privacy: false}}} automaticError="" />);
  act(() => {notify('error', 'private@example.test', 'workspace-secret')});
  expect(screen.getByText('private@example.test')).toBeTruthy();
  view.rerender(<Notifications state={current} automaticError="" />);
  expect(document.body.innerHTML).not.toContain('private@example.test');
  expect(document.body.innerHTML).not.toContain('workspace-secret');
});
