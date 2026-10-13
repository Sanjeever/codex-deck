import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from '../app/App';
import {createState} from '../test/fixtures';
import type {NetworkConfig, State} from '../bridge/api';

afterEach(() => {cleanup(); toast.clear(); vi.unstubAllGlobals()});
async function press(name: string, role: 'button' | 'menuitem' = 'button') {await act(async () => {fireEvent.click(screen.getByRole(role, {name}))})}
async function customProxy() {await press('设置'); await press('代理模式'); await press('自定义 HTTP 代理', 'menuitem')}

it('测试使用未保存草稿，状态推送不覆盖输入，保存成功才更新已保存配置', async () => {
  let state = createState();
  let update!: (value: State) => void;
  const TestNetwork = vi.fn(async (_config: NetworkConfig) => {});
  const SaveNetwork = vi.fn(async (config: NetworkConfig) => {state = {...state, network: config}; return config});
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), TestNetwork, SaveNetwork}}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  render(<App />); await act(async () => {}); await customProxy();
  const address = screen.getByRole('textbox', {name: '代理地址'}) as HTMLInputElement;
  fireEvent.change(address, {target: {value: 'http://127.0.0.1:7897'}});
  fireEvent.change(screen.getByRole('textbox', {name: '代理用户名'}), {target: {value: 'proxy-user'}});
  fireEvent.change(screen.getByLabelText('代理密码'), {target: {value: 'proxy-password'}});
  await act(async () => {update({...state, auto_refreshing: true})});
  expect(address.value).toBe('http://127.0.0.1:7897');
  await press('测试连接');
  expect(TestNetwork).toHaveBeenCalledWith({mode: 'custom', url: address.value, username: 'proxy-user', password: 'proxy-password'});
  expect(SaveNetwork).not.toHaveBeenCalled();
  expect(state.network.mode).toBe('environment');
  await press('保存代理');
  expect(SaveNetwork).toHaveBeenCalledWith(TestNetwork.mock.calls[0][0]);
  expect(state.network.mode).toBe('custom');
  expect((screen.getByRole('button', {name: '保存代理'}) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText('代理设置已保存')).toBeTruthy();
});

it('测试和保存失败保留草稿，代理密码使用遮罩', async () => {
  const state = createState(); state.preferences.privacy = true;
  const SaveNetwork = vi.fn().mockRejectedValue(new Error('保存配置失败'));
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), TestNetwork: vi.fn().mockRejectedValue(new Error('连接测试失败')), SaveNetwork}}});
  render(<App />); await act(async () => {}); await customProxy();
  const address = screen.getByRole('textbox', {name: '代理地址'}) as HTMLInputElement;
  fireEvent.change(address, {target: {value: 'http://127.0.0.1:7897'}});
  const password = screen.getByLabelText('代理密码') as HTMLInputElement;
  const username = screen.getByLabelText('代理用户名') as HTMLInputElement;
  expect(password.type).toBe('password'); expect(username.type).toBe('password');
  fireEvent.change(password, {target: {value: 'secret-draft'}});
  await press('测试连接'); await press('保存代理');
  expect(screen.getByRole('dialog')).toBeTruthy();
  expect(address.value).toBe('http://127.0.0.1:7897'); expect(password.value).toBe('secret-draft');
  expect(state.network.mode).toBe('environment');
  expect(screen.queryByText('代理设置已保存')).toBeNull();
});

it('选择直连时隐藏认证字段，保存后采用后端规范化结果', async () => {
  const state = createState({network: {mode: 'custom', url: 'http://127.0.0.1:7897', username: 'user', password: 'secret'}});
  const SaveNetwork = vi.fn(async () => ({mode: 'direct', url: '', username: '', password: ''}));
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), GetAutoStart: vi.fn(async () => false), SaveNetwork}}});
  render(<App />); await act(async () => {}); await press('设置');
  await press('代理模式'); await press('不使用代理', 'menuitem');
  expect(screen.queryByLabelText('代理密码')).toBeNull();
  await press('保存代理');
  const section = screen.getByRole('region', {name: '网络代理'});
  expect(within(section).getByRole('button', {name: '代理模式'}).textContent).toContain('不使用代理');
});
