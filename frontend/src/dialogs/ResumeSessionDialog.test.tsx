import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import App from '../app/App';
import {createAccount, createState} from '../test/fixtures';
import type {SessionSummary} from '../bridge/api';

const session: SessionSummary = {id: 'enumerated-id', resume_command: 'codex resume 9fb19d9b-42a9-4346-a9d0-27c242de94df', title: '继续开发终端', project: '/work/termquay', updated_at: '2026-11-17T11:58:36Z', models: [], turn_count: 1, tool_count: 0};
const plan = {command: "CODEX_HOME='/isolated/codex' codex -C '/work/termquay' resume 9fb19d9b-42a9-4346-a9d0-27c242de94df", restart_command: 'restart && resume', shell: 'Bash / Zsh'};
let state = createState();
let api: {GetState: ReturnType<typeof vi.fn>; SetAutomaticPaused: ReturnType<typeof vi.fn>; ListSessions: ReturnType<typeof vi.fn>; ReadSession: ReturnType<typeof vi.fn>; CancelSessionRequest: ReturnType<typeof vi.fn>; PrepareSessionResume: ReturnType<typeof vi.fn>; Switch: ReturnType<typeof vi.fn>};
let writeText: ReturnType<typeof vi.fn>;
const press = async (name: string) => {await act(async () => {fireEvent.click(screen.getByRole('button', {name}))})};

beforeEach(() => {
  state = createState({accounts: [createAccount({id: 'old', email: 'old@example.test', current: true}), createAccount({id: 'target', email: 'target@example.test', account_id: 'target-workspace'}), createAccount({id: 'expired', email: 'expired@example.test', auth_error: '凭据失效'})]});
  api = {
    GetState: vi.fn(async () => state), SetAutomaticPaused: vi.fn(async () => {}),
    ListSessions: vi.fn(async () => ({sessions: [session], projects: [session.project], dates: [], source: '/isolated/codex/sessions', missing: false, next_cursor: '', issues: []})),
    ReadSession: vi.fn(async () => ({session, events: [], turns: [], issues: [], revision: 'r1', next_cursor: ''})),
    CancelSessionRequest: vi.fn(async () => {}), PrepareSessionResume: vi.fn(async () => plan),
    Switch: vi.fn(async () => ({cli_daemon_running: false})),
  };
  writeText = vi.fn(async () => {});
  Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
  vi.stubGlobal('go', {main: {App: api}});
});
afterEach(() => {cleanup(); Reflect.deleteProperty(navigator, 'clipboard')});

async function open() {
  render(<App />);
  await act(async () => {});
  await press('会话');
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: /继续开发终端/}))});
  await press('换号继续');
}

it.each([false, true])('从会话切换账号后复制指定 ID 续接命令，daemon=%s', async daemon => {
  api.Switch.mockResolvedValue({cli_daemon_running: daemon});
  await open();
  expect((screen.getByRole('button', {name: '切换账号'}) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole('radio', {name: /old@example.test/}) as HTMLInputElement).disabled).toBe(true);
  expect((screen.getByRole('radio', {name: /expired@example.test/}) as HTMLInputElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole('radio', {name: /target@example.test/}));
  await press('切换账号');
  expect(api.PrepareSessionResume).toHaveBeenCalledExactlyOnceWith(session.id);
  expect(api.Switch).toHaveBeenCalledExactlyOnceWith('target', false);
  expect(api.PrepareSessionResume.mock.invocationCallOrder[0]).toBeLessThan(api.Switch.mock.invocationCallOrder[0]);
  expect(screen.getByRole('heading', {name: '账号已切换'})).toBeTruthy();
  expect(writeText).not.toHaveBeenCalled();
  await press('复制续接命令');
  expect(writeText).toHaveBeenLastCalledWith(daemon ? plan.restart_command : plan.command);
  expect(screen.getByText('续接命令已复制')).toBeTruthy();
  await press('关闭');
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.getByRole('heading', {name: session.title})).toBeTruthy();
});

it('准备会话失败时不切换账号，切换失败保留选择并可重试', async () => {
  api.PrepareSessionResume.mockRejectedValueOnce(new Error('会话文件已删除'));
  await open();
  fireEvent.click(screen.getByRole('radio', {name: /target@example.test/}));
  await press('切换账号');
  expect(api.Switch).not.toHaveBeenCalled();
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('会话文件已删除');
  api.Switch.mockRejectedValueOnce(new Error('账号需要重新登录'));
  await press('切换账号');
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('账号需要重新登录');
  expect((screen.getByRole('radio', {name: /target@example.test/}) as HTMLInputElement).checked).toBe(true);
  expect(screen.queryByRole('button', {name: '复制续接命令'})).toBeNull();
  await press('切换账号');
  expect(screen.getByRole('button', {name: '复制续接命令'})).toBeTruthy();
});

it('隐私模式隐藏身份，非文件存储明确确认，刷新或复制失败仍保留续接命令', async () => {
  state.cli_store = 'keyring'; state.preferences.privacy = true;
  await open();
  expect(screen.getByRole('dialog').innerHTML).not.toContain('target@example.test');
  expect(screen.getByRole('dialog').innerHTML).not.toContain('target-workspace');
  fireEvent.click(screen.getByRole('radio', {name: /账号 target/}));
  api.GetState.mockRejectedValueOnce(new Error('状态读取失败')).mockRejectedValueOnce(new Error('状态读取失败'));
  await press('改为文件存储并切换');
  expect(api.Switch).toHaveBeenCalledExactlyOnceWith('target', true);
  expect(screen.getByRole('heading', {name: '账号已切换'})).toBeTruthy();
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('状态读取失败');
  writeText.mockRejectedValueOnce(new Error('剪贴板拒绝访问'));
  await press('复制续接命令');
  expect(within(screen.getByRole('dialog')).getByRole('alert').textContent).toBe('剪贴板拒绝访问');
  expect(screen.queryByText('续接命令已复制')).toBeNull();
  await press('复制续接命令');
  expect(writeText).toHaveBeenLastCalledWith(plan.command);
  expect(api.Switch).toHaveBeenCalledTimes(1);
});
