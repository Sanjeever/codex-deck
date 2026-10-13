import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {useAutoRefresh} from './useAutoRefresh';

const setPaused = vi.fn(async (_paused: boolean) => {});
const wake = vi.fn(async () => {});

beforeEach(() => {
  vi.useFakeTimers();
  setPaused.mockReset(); wake.mockReset();
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: setPaused, WakeAutomatic: wake}}});
});
afterEach(() => {cleanup(); vi.unstubAllGlobals(); vi.useRealTimers()});

async function flush() {await act(async () => {await Promise.resolve()})}

it('窗口隐藏和失去焦点不暂停后台刷新，前端不运行定时查询', async () => {
  renderHook(() => useAutoRefresh(true, false));
  await flush();
  expect(setPaused.mock.calls).toEqual([[false]]);
  await act(async () => {
    window.dispatchEvent(new Event('blur'));
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(600_000);
  });
  expect(setPaused.mock.calls).toEqual([[false]]);
  expect(wake).not.toHaveBeenCalled();
});

it('操作或对话框暂停，结束后恢复后端调度', async () => {
  const hook = renderHook(({paused}) => useAutoRefresh(true, paused), {initialProps: {paused: false}});
  await flush();
  hook.rerender({paused: true}); await flush();
  hook.rerender({paused: false}); await flush();
  expect(setPaused.mock.calls).toEqual([[false], [true], [false]]);
});

it('等待暂停完成后才能开始操作，后续恢复按顺序执行', async () => {
  let finish!: () => void;
  setPaused.mockImplementationOnce(() => new Promise<void>(resolve => {finish = resolve}));
  const hook = renderHook(({paused}) => useAutoRefresh(true, paused), {initialProps: {paused: true}});
  await flush();
  let operationReady = false;
  const operation = hook.result.current.pause().then(() => {operationReady = true});
  hook.rerender({paused: false}); await flush();
  expect(operationReady).toBe(false);
  expect(setPaused).toHaveBeenCalledTimes(1);
  await act(async () => {finish(); await operation});
  await flush();
  expect(setPaused.mock.calls).toEqual([[true], [true], [false]]);
});

it('重新联网唤醒调度，卸载后移除监听，控制失败显示错误', async () => {
  setPaused.mockRejectedValueOnce(new Error('后端不可用'));
  const hook = renderHook(() => useAutoRefresh(true, false)); await flush();
  expect(hook.result.current.error).toBe('后端不可用');
  await act(async () => {window.dispatchEvent(new Event('online'))});
  expect(wake).toHaveBeenCalledTimes(1);
  hook.unmount();
  window.dispatchEvent(new Event('online'));
  expect(wake).toHaveBeenCalledTimes(1);
});
