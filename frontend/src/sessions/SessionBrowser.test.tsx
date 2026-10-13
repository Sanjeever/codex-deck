import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {privateText} from '../accounts/accounts';
import type {SessionEvent, SessionListPage, SessionReadPage, SessionSummary} from '../bridge/api';
import {SessionBrowser} from './SessionBrowser';
import {useSessionBrowser} from './useSessionBrowser';

const summary = (id = 'one'): SessionSummary => ({id, title: `请求 ${id}`, project: '/project', updated_at: '2026-10-13T08:00:00Z', models: ['recorded-model'], turn_count: 2, tool_count: 1});
const listPage = (sessions = [summary()]): SessionListPage => ({sessions, projects: ['/project'], dates: ['2026-10-13'], source: 'D:/isolated/sessions', missing: false, next_cursor: '', issues: []});
const event = (id = 'e1', text = '用户正文', kind = 'user'): SessionEvent => ({id, turn: 1, kind, text, text_cursor: 0});
const detailPage = (id = 'one', events = [event()]): SessionReadPage => ({session: summary(id), revision: 'r1', events, turns: [{number: 1, event_id: events[0]?.id ?? 'e1'}, {number: 2, event_id: 'later'}], next_cursor: '', issues: []});
function Harness({privacy = false}: {privacy?: boolean}) {
  const browser = useSessionBrowser(true);
  return <SessionBrowser browser={browser} active safeText={text => privateText(text, privacy, [{email: 'test@example.test', account_id: 'workspace-secret'}])} />;
}
const flush = async (ms = 0) => {await act(async () => {await vi.advanceTimersByTimeAsync(ms)})};
let api: {ListSessions: ReturnType<typeof vi.fn>; ReadSession: ReturnType<typeof vi.fn>; FindSessionMatches: ReturnType<typeof vi.fn>; ReadSessionText: ReturnType<typeof vi.fn>};
beforeEach(() => {
  vi.useFakeTimers();
  api = {ListSessions: vi.fn(async () => listPage()), ReadSession: vi.fn(async query => detailPage(query.id)), FindSessionMatches: vi.fn(async () => ({matches: [], next_cursor: '', issues: []})), ReadSessionText: vi.fn(async () => ({text: '下一段真实输出', next_offset: 0}))};
  vi.stubGlobal('go', {main: {App: api}});
});
afterEach(() => {cleanup(); vi.useRealTimers()});

it('由真实请求展示来源、选择详情、筛选和后续分页', async () => {
  api.ListSessions.mockResolvedValueOnce({...listPage(), next_cursor: 'next-list'}).mockResolvedValueOnce(listPage([summary('two')]));
  api.ReadSession.mockResolvedValueOnce({...detailPage(), next_cursor: 'next-events'}).mockResolvedValueOnce(detailPage('one', [event('e2', '失败输出', 'tool')]));
  render(<Harness />); await flush();
  expect(screen.getByText('D:/isolated/sessions')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: /请求 one/})); await flush();
  expect(screen.getByText('用户正文')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '加载后续记录'})); await flush();
  expect(api.ReadSession.mock.calls[1][0].cursor).toBe('next-events');
  expect(screen.getByText('状态未知')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '加载更多会话'})); await flush();
  expect(api.ListSessions.mock.calls[1][0].cursor).toBe('next-list');
  expect(screen.getByRole('button', {name: /请求 two/})).toBeTruthy();
  fireEvent.change(screen.getByRole('combobox', {name: '按项目筛选'}), {target: {value: '1'}}); await flush();
  expect(api.ListSessions.mock.calls.at(-1)?.[0].project).toBe('/project');
  fireEvent.click(screen.getByRole('button', {name: '异常'})); await flush();
  expect(api.ReadSession.mock.calls.at(-1)?.[0].filter).toBe('error');
});

it('空目录、无结果、部分不可读和错误重试不伪造数据或清空筛选', async () => {
  api.ListSessions.mockRejectedValueOnce(new Error('读取权限不足')).mockResolvedValueOnce({...listPage([]), missing: true}).mockResolvedValue({...listPage([]), issues: [{file: 'broken.jsonl', line: 3, message: 'JSON 损坏'}]});
  render(<Harness />); await flush();
  expect(screen.getByRole('alert').textContent).toBe('读取权限不足');
  fireEvent.click(screen.getByRole('button', {name: '重试读取'})); await flush();
  expect(screen.getByText('会话目录不存在')).toBeTruthy();
  fireEvent.change(screen.getByRole('textbox', {name: '搜索全部会话'}), {target: {value: 'needle'}}); await flush(300);
  expect(screen.getByText('没有匹配的会话')).toBeTruthy();
  expect(screen.getByText('broken.jsonl:3 · JSON 损坏')).toBeTruthy();
  expect((screen.getByRole('textbox', {name: '搜索全部会话'}) as HTMLInputElement).value).toBe('needle');
});

it('输入期间和快速切换会话时丢弃过期结果', async () => {
  let resolveOldList!: (page: SessionListPage) => void;
  let resolveOldDetail!: (page: SessionReadPage) => void;
  api.ListSessions.mockImplementationOnce(() => new Promise(resolve => {resolveOldList = resolve})).mockResolvedValue(listPage([summary('two'), summary('three')]));
  api.ReadSession.mockImplementationOnce(() => new Promise(resolve => {resolveOldDetail = resolve})).mockImplementation(async query => detailPage(query.id, [event('fresh', '新的详情')]));
  render(<Harness />); await flush();
  fireEvent.change(screen.getByRole('textbox', {name: '搜索全部会话'}), {target: {value: 'new'}});
  await act(async () => {resolveOldList(listPage([summary('old')])); await Promise.resolve()});
  expect(screen.queryByRole('button', {name: /请求 old/})).toBeNull();
  await flush(300);
  fireEvent.click(screen.getByRole('button', {name: /请求 two/})); await flush();
  fireEvent.click(screen.getByRole('button', {name: /请求 three/})); await flush();
  await act(async () => {resolveOldDetail(detailPage('two', [event('old', '过期正文')])); await Promise.resolve()});
  expect(screen.getByText('新的详情')).toBeTruthy(); expect(screen.queryByText('过期正文')).toBeNull();
});

it('会话内查找和轮次跳转可读取尚未展示的事件及长输出匹配片段', async () => {
  api.FindSessionMatches.mockResolvedValue({matches: [{event_id: 'later', turn: 2, snippet: 'long needle', text_offset: 9000}], next_cursor: '', issues: []});
  api.ReadSession.mockImplementation(async query => detailPage(query.id, [event(query.event_id || 'e1', query.event_id ? 'before needle' : '用户正文')]));
  api.ReadSessionText.mockResolvedValue({text: '片段 needle', next_offset: 10000});
  render(<Harness />); await flush(); fireEvent.click(screen.getByRole('button', {name: /请求 one/})); await flush();
  fireEvent.change(screen.getByRole('combobox', {name: '跳到对话轮次'}), {target: {value: 'later'}}); await flush();
  expect(api.ReadSession.mock.calls.at(-1)?.[0].event_id).toBe('later');
  fireEvent.change(screen.getByRole('textbox', {name: '查找当前会话'}), {target: {value: 'needle'}}); await flush(300);
  expect(api.FindSessionMatches).toHaveBeenCalledWith({id: 'one', search: 'needle', cursor: ''});
  fireEvent.click(screen.getByRole('button', {name: '下一个匹配事件'})); await flush();
  expect(api.ReadSessionText).toHaveBeenCalledWith({id: 'one', event_id: 'later', revision: 'r1', offset: 8960});
  expect(screen.getByText('从第 8961 个字符显示')).toBeTruthy();
});

it('列表中的长输出命中直接定位匹配片段', async () => {
  api.ListSessions.mockResolvedValue(listPage([{...summary(), match_event_id: 'later', match_text_offset: 9000, snippet: 'long needle'}]));
  api.ReadSession.mockResolvedValue(detailPage('one', [event('later', '原始首段', 'tool')]));
  api.ReadSessionText.mockResolvedValue({text: '已定位 needle', next_offset: 0});
  render(<Harness />); await flush(); fireEvent.click(screen.getByRole('button', {name: /请求 one/})); await flush();
  expect(api.ReadSessionText).toHaveBeenCalledWith({id: 'one', event_id: 'later', revision: 'r1', offset: 8960});
  expect(screen.getByText('已定位 needle', {selector: 'pre'})).toBeTruthy();
});

it('隐私覆盖正文、摘要、路径、错误和可访问名称，高亮与 HTML 保持纯文本', async () => {
  const secret = 'test@example.test workspace-secret <img src=x onerror=alert(1)>';
  api.ListSessions.mockResolvedValue({...listPage([{...summary(), title: secret, project: secret, snippet: secret}]), source: secret, issues: [{file: secret, message: secret}]});
  api.ReadSession.mockResolvedValue({...detailPage('one', [event('secret', secret)]), session: {...summary(), title: secret, project: secret}});
  const {container} = render(<Harness privacy />); await flush();
  fireEvent.click(screen.getByRole('button', {name: /••••@••••/})); await flush();
  expect(container.textContent).not.toContain('test@example.test'); expect(container.textContent).not.toContain('workspace-secret');
  expect(container.querySelector('img')).toBeNull();
  for (const element of container.querySelectorAll('[aria-label],[title]')) {expect(element.outerHTML).not.toContain('workspace-secret')}
});

it('工具默认折叠、未知状态准确，分段失败可显式重试', async () => {
  api.ReadSession.mockResolvedValue(detailPage('one', [{...event('tool', '原始工具输出', 'tool'), name: 'tool', status: 'unknown', text_cursor: 8192}]));
  api.ReadSessionText.mockRejectedValueOnce(new Error('文件已变化')).mockResolvedValue({text: '下一段真实输出', next_offset: 0});
  const {container} = render(<Harness />); await flush(); fireEvent.click(screen.getByRole('button', {name: /请求 one/})); await flush();
  const details = container.querySelector('.session-tool-event details') as HTMLDetailsElement;
  expect(details.open).toBe(false); expect(screen.getByText('状态未知')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '展开工具'})); await flush(); expect(details.open).toBe(true);
  fireEvent.click(screen.getByRole('button', {name: '下一段内容'})); await flush();
  expect(screen.getByRole('alert').textContent).toContain('文件已变化');
  fireEvent.click(screen.getByRole('button', {name: '重试展开'})); await flush();
  expect(screen.getByText('下一段真实输出')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '折叠工具'})); await flush(); expect(details.open).toBe(false);
  expect(within(screen.getByRole('region', {name: '会话详情'})).queryByText('成功')).toBeNull();
});
