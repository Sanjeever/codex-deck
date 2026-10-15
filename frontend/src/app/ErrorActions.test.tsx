import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import {createAccount, createState} from '../test/fixtures';
import App from './App';

afterEach(() => {cleanup(); toast.clear(); Reflect.deleteProperty(navigator, 'clipboard')});

it('错误详情与复制遵循隐私模式，重试只请求对应账号', async () => {
  const account = createAccount({usage_error: '网络失败 account@example.test workspace'});
  const state = createState({accounts: [account]});
  state.preferences.privacy = true;
  const refresh = vi.fn(async () => {});
  const writeText = vi.fn(async () => {});
  Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
  vi.stubGlobal('go', {main: {App: {GetState: vi.fn(async () => state), SetAutomaticPaused: vi.fn(async () => {}), RefreshAccount: refresh}}});
  render(<App />);
  await act(async () => {});
  fireEvent.click(screen.getByText('用量查询失败'));
  const details = within(screen.getByText('用量查询失败').closest('details')!);
  expect(details.getByText('网络失败 ••••@•••• ••••••••')).toBeTruthy();
  await act(async () => {fireEvent.click(details.getByRole('button', {name: '复制错误'}))});
  expect(writeText).toHaveBeenCalledWith('网络失败 ••••@•••• ••••••••');
  await act(async () => {fireEvent.click(details.getByRole('button', {name: '重试此账号'}))});
  expect(refresh).toHaveBeenCalledExactlyOnceWith(account.id);
});

it('备份处理入口直接打开备份设置', async () => {
  const state = createState({backup: {busy: false, error: '上传失败', at: null}});
  vi.stubGlobal('go', {main: {App: {GetState: vi.fn(async () => state), SetAutomaticPaused: vi.fn(async () => {}), GetAutoStart: vi.fn(async () => false)}}});
  render(<App />);
  await act(async () => {});
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '备份失败 · 查看'}))});
  expect(screen.getByRole('tab', {name: '备份'}).getAttribute('aria-selected')).toBe('true');
  expect(screen.getByRole('textbox', {name: 'WebDAV 目录地址'})).toBeTruthy();
});
