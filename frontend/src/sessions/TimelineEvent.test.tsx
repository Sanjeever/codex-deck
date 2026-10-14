import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {privateText} from '../accounts/accounts';
import {TimelineEvent} from './TimelineEvent';

let writeText: ReturnType<typeof vi.fn>;
let readText: ReturnType<typeof vi.fn>;
beforeEach(() => {
  writeText = vi.fn(async () => {});
  readText = vi.fn();
  Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
  vi.stubGlobal('go', {main: {App: {ReadSessionText: readText}}});
});
afterEach(() => {cleanup(); toast.clear(); Reflect.deleteProperty(navigator, 'clipboard')});

const event = {id: 'e1', turn: 1, kind: 'assistant', text: '展示片段', text_cursor: 8192};
const props = {event, sessionID: 'session', revision: 'r1', textStart: 400, query: '', expanded: false, forceOpen: false};
const safeText = (text: string) => privateText(text, true, [{email: 'test@example.test', account_id: 'workspace-secret'}]);

it('片段复制不读取全文，完整复制从起点拼接所有段并隐藏身份', async () => {
  readText.mockResolvedValueOnce({text: '前段 test@example.test ', next_offset: 8192}).mockResolvedValueOnce({text: '末段 workspace-secret', next_offset: 0});
  render(<TimelineEvent {...props} safeText={safeText} />);
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '复制当前片段'}))});
  expect(writeText).toHaveBeenCalledWith('展示片段');
  expect(readText).not.toHaveBeenCalled();
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '复制完整内容'}))});
  expect(readText.mock.calls.map(([query]) => query.offset)).toEqual([0, 8192]);
  expect(readText.mock.calls[0][0]).toEqual({id: 'session', event_id: 'e1', revision: 'r1', offset: 0});
  expect(writeText).toHaveBeenLastCalledWith('前段 ••••@•••• 末段 ••••••••');
});

it('读取失败不复制半份内容，切换消息后不写入旧内容', async () => {
  readText.mockResolvedValueOnce({text: '前段', next_offset: 8192}).mockRejectedValueOnce(new Error('会话已变化，请刷新'));
  const view = render(<TimelineEvent {...props} safeText={text => text} />);
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '复制完整内容'}))});
  expect(screen.getByRole('alert').textContent).toBe('会话已变化，请刷新');
  expect(writeText).not.toHaveBeenCalled();
  let finish!: (page: {text: string; next_offset: number}) => void;
  readText.mockImplementationOnce(() => new Promise(resolve => {finish = resolve}));
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '复制完整内容'}))});
  view.unmount();
  await act(async () => {finish({text: '旧消息', next_offset: 0})});
  expect(writeText).not.toHaveBeenCalled();
});

it('完整复制期间切换隐私模式，使用最新展示规则', async () => {
  let finish!: (page: {text: string; next_offset: number}) => void;
  readText.mockImplementationOnce(() => new Promise(resolve => {finish = resolve}));
  const view = render(<TimelineEvent {...props} safeText={text => text} />);
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '复制完整内容'}))});
  view.rerender(<TimelineEvent {...props} safeText={safeText} />);
  await act(async () => {finish({text: 'test@example.test workspace-secret', next_offset: 0})});
  expect(writeText).toHaveBeenCalledWith('••••@•••• ••••••••');
});
