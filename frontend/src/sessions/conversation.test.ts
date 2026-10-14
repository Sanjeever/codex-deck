import {beforeEach, expect, it, vi} from 'vitest';
import type {SessionReadPage} from '../bridge/api';
import {readConversation} from './conversation';

const read = vi.fn();
const text = vi.fn();
const page = (overrides: Partial<SessionReadPage> = {}): SessionReadPage => ({session: {id: 'file-id', title: '真实请求', project: '/project', models: [], turn_count: 2, tool_count: 0}, revision: 'r1', turns: [], events: [], next_cursor: '', issues: [], ...overrides});
const event = (id: string, turn: number, kind: string, text: string, text_cursor = 0) => ({id, turn, kind, text, text_cursor});
beforeEach(() => {read.mockReset(); text.mockReset(); vi.stubGlobal('go', {main: {App: {ReadSession: read, ReadSessionText: text}}})});

it('复制对话读取后续事件和长文本，保留轮次与读取问题', async () => {
  read.mockResolvedValueOnce(page({events: [event('u1', 1, 'user', '请求')], next_cursor: 'next', issues: [{file: 'rollout.jsonl', line: 3, message: '未知事件'}]}));
  read.mockResolvedValueOnce(page({events: [event('a1', 1, 'assistant', '回复首段', 8192), event('u2', 2, 'user', '下一轮')]}));
  text.mockResolvedValue({text: '回复末段', next_offset: 0});
  const copied = await readConversation('file-id', 'r1', null, '', () => true);
  expect(copied).toBe('# 真实请求\n\n> 读取提示：rollout.jsonl:3 · 未知事件\n\n## 第 1 轮\n\n### 你\n\n请求\n\n### Codex\n\n回复首段回复末段\n\n## 第 2 轮\n\n### 你\n\n下一轮');
  expect(read.mock.calls[1][0]).toEqual({id: 'file-id', filter: 'chat', cursor: 'next', event_id: ''});
  expect(text).toHaveBeenCalledWith({id: 'file-id', event_id: 'a1', revision: 'r1', offset: 8192});
});

it('复制本轮从轮次起点读取，跨页收集回复并在下一轮停止', async () => {
  read.mockResolvedValueOnce(page({events: [event('u2', 2, 'user', '第二轮请求')], next_cursor: 'next'}));
  read.mockResolvedValueOnce(page({events: [event('a2', 2, 'assistant', '第二轮回复'), event('u3', 3, 'user', '不应复制')], next_cursor: 'unused'}));
  expect(await readConversation('file-id', 'r1', 2, 'u2', () => true)).toBe('## 第 2 轮\n\n### 你\n\n第二轮请求\n\n### Codex\n\n第二轮回复');
  expect(read.mock.calls[0][0].event_id).toBe('u2');
  expect(read).toHaveBeenCalledTimes(2);
});

it('文件版本变化、读取失败或操作失效时不返回半份对话', async () => {
  read.mockResolvedValueOnce(page({events: [event('u1', 1, 'user', '请求')], next_cursor: 'next'})).mockResolvedValueOnce(page({revision: 'r2'}));
  await expect(readConversation('file-id', 'r1', null, '', () => true)).rejects.toThrow('会话已变化');
  read.mockResolvedValueOnce(page({events: [event('a1', 1, 'assistant', '首段', 8192)]}));
  text.mockRejectedValueOnce(new Error('文件不可读'));
  await expect(readConversation('file-id', 'r1', null, '', () => true)).rejects.toThrow('文件不可读');
  read.mockResolvedValueOnce(page());
  expect(await readConversation('file-id', 'r1', null, '', () => false)).toBeNull();
});
