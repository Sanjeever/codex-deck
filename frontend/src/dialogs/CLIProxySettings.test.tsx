import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import type {CLIProxyChange, CLIProxyState, Configuration, ConfigurationSaveResult, State} from '../bridge/api';
import {createState} from '../test/fixtures';
import App from '../app/App';

afterEach(() => {cleanup(); toast.clear()});
const press = async (name: string, role: 'button' | 'tab' | 'radio' = 'button') => {await act(async () => {fireEvent.click(screen.getByRole(role, {name}))})};
const field = () => screen.getByLabelText('CLI 代理地址') as HTMLInputElement;
const change = (value: string) => fireEvent.change(field(), {target: {value}});

function setup(initial = createState(), variables: CLIProxyState['variables'] = []) {
  let state = initial;
  let cli: CLIProxyState = {path: 'D:/isolated/codex/.env', variables};
  let update!: (value: State) => void;
  const api = {
    SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state),
    GetAutoStart: vi.fn(async () => false), SetAutoStart: vi.fn(async (enabled: boolean) => enabled),
    GetCLIProxy: vi.fn(async () => cli),
    SaveCLIProxy: vi.fn(async (change: CLIProxyChange) => {cli = {...cli, variables: change.remove ? [] : [{key: 'http_proxy', value: change.url}, {key: 'https_proxy', value: change.url}]}; return cli}),
    TestCLIProxy: vi.fn(async (_address: string) => {}),
    SaveConfiguration: vi.fn(async (config: Configuration): Promise<ConfigurationSaveResult> => {state = {...state, network: config.network, settings: config.settings, preferences: {...state.preferences, ...config.preferences}}; return {configuration: config, section: '', error: ''}}),
    Backup: vi.fn(async () => {}), CheckUpdate: vi.fn(async () => {}),
  };
  vi.stubGlobal('go', {main: {App: api}});
  vi.stubGlobal('runtime', {EventsOnMultiple: (_event: string, callback: (value: State) => void) => {update = callback; return () => {}}});
  render(<App />);
  return {api, push: (patch: Partial<State>) => update({...state, ...patch})};
}

it('按需读取，复用 Deck 地址只填草稿，测试不保存，统一保存后提示重启', async () => {
  const {api, push} = setup(createState({network: {mode: 'custom', url: 'http://127.0.0.1:7897', username: '', password: ''}}));
  await act(async () => {});
  await press('设置');
  expect(api.GetCLIProxy).not.toHaveBeenCalled();
  await press('网络', 'tab');
  expect(api.GetCLIProxy).toHaveBeenCalledTimes(1);
  expect(screen.getByText('文件中未配置 HTTP/HTTPS 代理')).toBeTruthy();
  await press('使用 Deck 代理地址');
  expect(field().value).toBe('http://127.0.0.1:7897/');
  expect(api.SaveCLIProxy).not.toHaveBeenCalled();
  await press('测试 CLI 代理');
  expect(api.TestCLIProxy).toHaveBeenCalledWith('http://127.0.0.1:7897/');
  expect(screen.getByText('GitHub 连接成功')).toBeTruthy();
  change('http://127.0.0.1:7898');
  expect(screen.queryByText('GitHub 连接成功')).toBeNull();
  await act(async () => {push({auto_refreshing: true})});
  expect(field().value).toBe('http://127.0.0.1:7898');
  await press('保存更改');
  expect(api.SaveCLIProxy).toHaveBeenCalledWith({url: 'http://127.0.0.1:7898', remove: false, expected: []});
  expect(api.SaveConfiguration).not.toHaveBeenCalled();
  expect(screen.getByText('重启 Codex CLI 后生效')).toBeTruthy();
  await press('关闭'); await press('设置'); await press('网络', 'tab');
  expect(field().value).toBe('http://127.0.0.1:7898');
});

it('文件写入失败保留 CLI 草稿，已保存 Deck 设置不重复保存，外部更新可重新读取', async () => {
  const {api} = setup();
  api.SaveCLIProxy.mockRejectedValueOnce(new Error('保存 CLI 代理失败：写入权限不足'));
  await act(async () => {}); await press('设置'); await press('网络', 'tab');
  await press('自定义代理', 'radio');
  fireEvent.change(screen.getByLabelText('代理地址'), {target: {value: 'http://localhost:7897'}});
  change('http://localhost:7898');
  await press('保存更改');
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toContain('Deck 设置已保存，CLI 代理保存失败');
  expect(field().value).toBe('http://localhost:7898');
  expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(screen.queryByText('设置已保存')).toBeNull();
  await press('保存更改');
  expect(api.SaveConfiguration).toHaveBeenCalledTimes(1);
  expect(api.SaveCLIProxy).toHaveBeenCalledTimes(2);
  change('http://localhost:7899');
  expect((screen.getByRole('button', {name: '重新读取'}) as HTMLButtonElement).disabled).toBe(true);
  await press('撤销 CLI 更改');
  api.GetCLIProxy.mockResolvedValueOnce({path: 'D:/isolated/codex/.env', variables: [{key: 'HTTP_PROXY', value: 'http://external:7890'}]});
  await press('重新读取');
  expect(field().value).toBe('http://external:7890');
});

it('移除是待保存操作，关闭时确认未保存修改，文件读取失败可显式重试', async () => {
  const {api} = setup(createState(), [{key: 'HTTP_PROXY', value: 'http://old:8000'}, {key: 'https_proxy', value: 'http://other:8001'}]);
  api.GetCLIProxy.mockRejectedValueOnce(new Error('CLI 环境文件格式无效'));
  await act(async () => {}); await press('设置'); await press('网络', 'tab');
  expect(within(screen.getByRole('region', {name: 'Codex CLI 代理'})).getByRole('alert').textContent).toBe('CLI 环境文件格式无效');
  await press('读取 CLI 代理');
  expect(field().value).toBe('');
  expect(screen.getByText('http://old:8000')).toBeTruthy();
  expect(screen.getByText('http://other:8001')).toBeTruthy();
  await press('移除代理配置');
  expect(api.SaveCLIProxy).not.toHaveBeenCalled();
  await press('关闭');
  expect(screen.getByRole('heading', {name: '放弃未保存更改？'})).toBeTruthy();
  await press('继续编辑'); await press('保存更改');
  expect(api.SaveCLIProxy).toHaveBeenCalledWith({url: '', remove: true, expected: [{key: 'HTTP_PROXY', value: 'http://old:8000'}, {key: 'https_proxy', value: 'http://other:8001'}]});
  expect(screen.getByText('文件中未配置 HTTP/HTTPS 代理')).toBeTruthy();
});

it('代理认证复制到 CLI 草稿时正确编码，现有认证值使用遮罩，CLI 草稿不阻塞 Deck 连接操作', async () => {
  const {api} = setup(createState({network: {mode: 'custom', url: 'http://localhost:7897', username: 'user@name', password: 'p$#@ss'}, settings: {webdav_url: 'https://example.test/backup', webdav_username: '', webdav_password: ''}}), [{key: 'http_proxy', value: 'http://old:private@localhost:8000'}]);
  await act(async () => {}); await press('设置'); await press('网络', 'tab');
  expect(field().type).toBe('password');
  expect(screen.queryByText('http://old:private@localhost:8000')).toBeNull();
  await press('使用 Deck 代理地址');
  expect(field().value).toBe('http://user%40name:p%24%23%40ss@localhost:7897/');
  await press('备份', 'tab');
  expect((screen.getByRole('button', {name: '立即备份'}) as HTMLButtonElement).disabled).toBe(false);
  await press('关于', 'tab');
  expect((screen.getByRole('button', {name: '检查更新'}) as HTMLButtonElement).disabled).toBe(false);
  expect(api.SaveCLIProxy).not.toHaveBeenCalled();
});
