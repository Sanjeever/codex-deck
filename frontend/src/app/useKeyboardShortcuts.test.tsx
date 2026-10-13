import {act, cleanup, fireEvent, renderHook} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import {useKeyboardShortcuts} from './useKeyboardShortcuts';
afterEach(cleanup);
it('快捷键调用对应操作，弹窗期间停用，输入框不触发添加账号', () => {
 const actions = {refresh: vi.fn(), settings: vi.fn(), login: vi.fn(), search: vi.fn()};
 const {rerender} = renderHook(({enabled}) => useKeyboardShortcuts(enabled, actions), {initialProps: {enabled: true}});
 for (const key of ['r', ',', 'n', 'f']) fireEvent.keyDown(window, {key, metaKey: true});
 expect(Object.values(actions).map(action => action.mock.calls.length)).toEqual([1, 1, 1, 1]);
 const input = document.createElement('input'); document.body.append(input);
 fireEvent.keyDown(input, {key: 'n', ctrlKey: true}); expect(actions.login).toHaveBeenCalledTimes(1); input.remove();
 act(() => rerender({enabled: false})); fireEvent.keyDown(window, {key: 'r', ctrlKey: true}); expect(actions.refresh).toHaveBeenCalledTimes(1);
});

it('重新渲染后快捷键使用最新操作', () => {
 const actions = {refresh: vi.fn(), settings: vi.fn(), login: vi.fn(), search: vi.fn()};
 const next = {...actions, refresh: vi.fn()};
 const {rerender} = renderHook(({actions}) => useKeyboardShortcuts(true, actions), {initialProps: {actions}});
 act(() => rerender({actions: next}));
 fireEvent.keyDown(window, {key: 'r', ctrlKey: true});
 expect(actions.refresh).not.toHaveBeenCalled();
 expect(next.refresh).toHaveBeenCalledTimes(1);
});

it('会话工作区只路由会话搜索与刷新，停用添加账号并沿用弹窗边界', () => {
 const actions = {refresh: vi.fn(), settings: vi.fn(), search: vi.fn(), sessionSearch: vi.fn()};
 const {rerender} = renderHook(({enabled}) => useKeyboardShortcuts(enabled, actions), {initialProps: {enabled: true}});
 for (const key of ['r', 'f', 'k', 'n']) fireEvent.keyDown(window, {key, ctrlKey: true});
 expect(actions.refresh).toHaveBeenCalledTimes(1); expect(actions.search).toHaveBeenCalledTimes(1); expect(actions.sessionSearch).toHaveBeenCalledTimes(1);
 rerender({enabled: false}); fireEvent.keyDown(window, {key: 'k', metaKey: true}); expect(actions.sessionSearch).toHaveBeenCalledTimes(1);
});
