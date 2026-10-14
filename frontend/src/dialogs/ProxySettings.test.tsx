import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from '../app/App';
import {createState} from '../test/fixtures';
import type {Configuration, ConfigurationSaveResult, NetworkConfig, State} from '../bridge/api';

afterEach(() => {cleanup(); toast.clear(); vi.unstubAllGlobals()});
async function press(name: string, role: 'button' | 'tab' | 'radio' = 'button') {await act(async () => {fireEvent.click(screen.getByRole(role, {name}))})}
async function customProxy() {await press('设置'); await press('网络', 'tab'); await press('自定义代理', 'radio')}
function field(name: string) {return screen.getByLabelText(name) as HTMLInputElement}
function change(name: string, value: string) {fireEvent.change(field(name), {target: {value}})}

function setup(initial = createState()) {
  let state = initial;
  let update!: (value: State) => void;
  const SaveConfiguration = vi.fn(async (config: Configuration): Promise<ConfigurationSaveResult> => {
    const normalized = {...config, network: config.network.mode === 'custom' ? {...config.network, url: config.network.url.trim()} : {mode: config.network.mode, url: '', username: '', password: ''}};
    state = {...state, network: normalized.network, settings: normalized.settings, preferences: {...state.preferences, ...normalized.preferences}};
    return {configuration: normalized, section: '', error: ''};
  });
  const api = {
    SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state),
    GetAutoStart: vi.fn(async () => false), SetAutoStart: vi.fn(async (enabled: boolean) => enabled),
    GetCLIProxy: vi.fn(async () => ({path: 'D:/isolated/codex/.env', variables: []})),
    TestNetwork: vi.fn(async (_config: NetworkConfig) => {}), SaveConfiguration,
    Backup: vi.fn(async () => {}), CheckUpdate: vi.fn(async () => {}),
  };
  vi.stubGlobal('go', {main: {App: api}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  render(<App />);
  return {api, push: (value: Partial<State>) => update({...state, ...value}), state: () => state};
}

it('统一保存所有标签页，测试不保存，后台推送保留草稿，关闭重开保留代理', async () => {
  const {api, push, state} = setup(createState({build_info: {version: '1.2.0', commit: '', development: false}}));
  await act(async () => {}); await press('设置');
  expect((screen.getByRole('button', {name: '保存更改'}) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole('switch', {name: '低额度通知'}));
  await press('关于', 'tab'); fireEvent.click(screen.getByRole('switch', {name: '自动检查更新'}));
  await press('网络', 'tab'); await press('自定义代理', 'radio');
  change('代理地址', 'http://127.0.0.1:7897');
  await act(async () => {fireEvent.click(screen.getByText('代理认证（可选）'))});
  change('代理用户名', 'proxy-user'); change('代理密码', 'proxy-password');
  await press('测试连接');
  expect(api.TestNetwork).toHaveBeenCalledWith({mode: 'custom', url: 'http://127.0.0.1:7897', username: 'proxy-user', password: 'proxy-password'});
  expect(api.SaveConfiguration).not.toHaveBeenCalled();
  expect(screen.getByText('连接成功 · 尚未保存')).toBeTruthy();
  change('代理地址', 'http://127.0.0.1:7898');
  expect(screen.queryByText('连接成功 · 尚未保存')).toBeNull();
  change('代理地址', 'http://127.0.0.1:7897');
  await press('备份', 'tab'); change('WebDAV 目录地址', 'https://example.test/backup/');
  await act(async () => {push({auto_refreshing: true})});
  expect(field('WebDAV 目录地址').value).toBe('https://example.test/backup/');
  await press('保存更改');
  expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(state().network.mode).toBe('custom');
  expect(state().preferences.low_quota_notify).toBe(true);
  expect(state().preferences.disable_update_check).toBe(true);
  expect((screen.getByRole('button', {name: '保存更改'}) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText('设置已保存')).toBeTruthy();
  await press('关闭'); await press('设置'); await press('网络', 'tab');
  expect((screen.getByRole('radio', {name: '自定义代理'}) as HTMLInputElement).checked).toBe(true);
  expect(field('代理地址').value).toBe('http://127.0.0.1:7897');
  expect(field('代理用户名').value).toBe('proxy-user');
});

it('连接和保存失败保留草稿，允许保存未通过连接测试的配置，密码使用遮罩', async () => {
  const initial = createState(); initial.preferences.privacy = true;
  const {api} = setup(initial);
  api.TestNetwork.mockRejectedValueOnce(new Error('连接测试失败'));
  api.SaveConfiguration.mockRejectedValueOnce(new Error('保存配置失败'));
  await act(async () => {}); await customProxy();
  change('代理地址', 'http://127.0.0.1:7897');
  await act(async () => {fireEvent.click(screen.getByText('代理认证（可选）'))});
  expect(field('代理密码').type).toBe('password'); expect(field('代理用户名').type).toBe('password');
  change('代理用户名', 'user'); change('代理密码', 'secret-draft');
  await press('测试连接'); await press('保存更改');
  expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(field('代理地址').value).toBe('http://127.0.0.1:7897');
  expect(field('代理密码').value).toBe('secret-draft');
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('保存配置失败');
  expect(screen.queryByText('设置已保存')).toBeNull();
});

it('直连隐藏认证字段，保存后使用后端规范化结果', async () => {
  setup(createState({network: {mode: 'custom', url: 'http://127.0.0.1:7897', username: 'user', password: 'secret'}}));
  await act(async () => {}); await press('设置'); await press('网络', 'tab');
  await press('不使用代理', 'radio');
  expect(screen.queryByLabelText('代理密码')).toBeNull();
  await press('保存更改'); await press('自定义代理', 'radio');
  expect(field('代理地址').value).toBe('');
  expect(field('代理密码').value).toBe('');
});

it('跨标签页校验失败定位对应页，并保留所有输入', async () => {
  const {api} = setup();
  api.SaveConfiguration.mockResolvedValueOnce({configuration: null, section: 'network', error: '代理地址无效'});
  await act(async () => {}); await customProxy(); change('代理地址', 'bad-url');
  await press('备份', 'tab'); change('WebDAV 目录地址', 'https://example.test/backup/');
  await press('保存更改');
  expect(screen.getByRole('tab', {name: '网络'}).getAttribute('aria-selected')).toBe('true');
  expect(field('代理地址').value).toBe('bad-url');
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('代理地址无效');
  await press('备份', 'tab'); expect(field('WebDAV 目录地址').value).toBe('https://example.test/backup/');
});

it.each(['关闭', 'Escape'])('%s 触发未保存确认，继续编辑保留草稿，放弃后才关闭', async action => {
  setup(); await act(async () => {}); await customProxy(); change('代理地址', 'http://127.0.0.1:7897');
  if (action === '关闭') await press('关闭');
  else await act(async () => {fireEvent.keyDown(screen.getByRole('dialog'), {key: 'Escape', code: 'Escape'})});
  expect(screen.getByRole('heading', {name: '放弃未保存更改？'})).toBeTruthy();
  await press('继续编辑'); expect(field('代理地址').value).toBe('http://127.0.0.1:7897');
  await press('关闭'); await press('放弃更改');
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('网络草稿未保存时禁用备份和更新操作，保存后恢复', async () => {
  const {api} = setup(createState({settings: {webdav_url: 'https://example.test/backup/', webdav_username: '', webdav_password: ''}}));
  await act(async () => {}); await customProxy(); change('代理地址', 'http://127.0.0.1:7897');
  await press('备份', 'tab');
  expect((screen.getByRole('button', {name: '立即备份'}) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole('button', {name: '恢复备份'}) as HTMLButtonElement).disabled).toBe(true);
  await press('关于', 'tab');
  expect((screen.getByRole('button', {name: '检查更新'}) as HTMLButtonElement).disabled).toBe(true);
  expect(api.Backup).not.toHaveBeenCalled(); expect(api.CheckUpdate).not.toHaveBeenCalled();
  await press('保存更改');
  expect((screen.getByRole('button', {name: '检查更新'}) as HTMLButtonElement).disabled).toBe(false);
  await press('备份', 'tab'); expect((screen.getByRole('button', {name: '立即备份'}) as HTMLButtonElement).disabled).toBe(false);
});

it('本地配置成功后自启动失败，只保留自启动修改，重试不重复保存本地配置', async () => {
  const {api} = setup(); api.SetAutoStart.mockRejectedValueOnce(new Error('系统拒绝修改'));
  await act(async () => {}); await press('设置');
  fireEvent.click(screen.getByRole('switch', {name: '开机自启动'}));
  expect(api.SetAutoStart).not.toHaveBeenCalled();
  await press('网络', 'tab'); await press('不使用代理', 'radio'); await press('保存更改');
  expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(api.SetAutoStart).toHaveBeenCalledWith(true);
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toContain('其他设置已保存，开机自启动设置失败');
  expect(screen.queryByText('设置已保存')).toBeNull();
  expect(within(screen.getByRole('tab', {name: '网络'})).queryByLabelText('有未保存更改')).toBeNull();
  await press('保存更改'); expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(api.SetAutoStart).toHaveBeenCalledTimes(2);
  expect((screen.getByRole('button', {name: '保存更改'}) as HTMLButtonElement).disabled).toBe(true);
});
