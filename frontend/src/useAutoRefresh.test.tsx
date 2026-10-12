import {useCallback, useState} from 'react';
import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {useAutoRefresh} from './useAutoRefresh';

const INTERVAL = 5 * 60_000;
let visible: DocumentVisibilityState;
let focused: boolean;
let online: boolean;

beforeEach(() => {
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-13T00:00:00Z'));
  visible = 'visible'; focused = true; online = true;
  vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visible);
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  vi.spyOn(navigator, 'onLine', 'get').mockImplementation(() => online);
});
afterEach(() => {cleanup(); vi.restoreAllMocks(); vi.useRealTimers()});

function mount(request = vi.fn(async () => Date.now() + INTERVAL), firstDelay = 0) {
  const accepted = vi.fn();
  const hook = renderHook(({paused}) => {
    const [next, setNext] = useState(Date.now() + firstDelay);
    const apply = useCallback((at: number) => {accepted(at); setNext(at)}, []);
    return useAutoRefresh(request, apply, true, paused, next);
  }, {initialProps: {paused: false}});
  return {...hook, request, accepted};
}

async function tick(ms: number) {await act(async () => {await vi.advanceTimersByTimeAsync(ms)})}
async function event(target: Window | Document, name: string) {await act(async () => {target.dispatchEvent(new Event(name)); await Promise.resolve()})}

describe('自动刷新', () => {
  it('按到期时间更新并合并连续窗口事件', async () => {
    const hook = mount();
    await tick(0);
    expect(hook.request).toHaveBeenCalledTimes(1);
    await event(window, 'focus'); await event(document, 'visibilitychange');
    await tick(999); expect(hook.request).toHaveBeenCalledTimes(1);
    await tick(1); expect(hook.request).toHaveBeenCalledTimes(2);
    await tick(INTERVAL - 1); expect(hook.request).toHaveBeenCalledTimes(2);
    await tick(1); expect(hook.request).toHaveBeenCalledTimes(3);
  });

  it('窗口隐藏或失去焦点时暂停，回到窗口立即检查', async () => {
    const hook = mount(); await tick(0);
    focused = false; await event(window, 'blur');
    await tick(INTERVAL * 2); expect(hook.request).toHaveBeenCalledTimes(1);
    focused = true; visible = 'hidden'; await event(document, 'visibilitychange');
    await tick(INTERVAL); expect(hook.request).toHaveBeenCalledTimes(1);
    visible = 'visible'; await event(document, 'visibilitychange');
    expect(hook.request).toHaveBeenCalledTimes(2);
  });

  it('操作或弹窗期间暂停，结束后处理已到期数据', async () => {
    const hook = mount(); await tick(0);
    hook.rerender({paused: true});
    await tick(INTERVAL * 2); await event(window, 'focus');
    expect(hook.request).toHaveBeenCalledTimes(1);
    hook.rerender({paused: false}); await tick(0);
    expect(hook.request).toHaveBeenCalledTimes(2);
  });

  it('请求未完成时不重复发起，进入操作后丢弃旧结果', async () => {
    let complete!: (at: number) => void;
    const request = vi.fn(() => new Promise<number>(resolve => {complete = resolve}));
    const hook = mount(request); await tick(0);
    expect(hook.result.current.refreshing).toBe(true);
    await event(window, 'focus'); await tick(INTERVAL);
    expect(request).toHaveBeenCalledTimes(1);
    hook.rerender({paused: true});
    await act(async () => {complete(Date.now() + INTERVAL); await Promise.resolve()});
    expect(hook.accepted).not.toHaveBeenCalled();
    expect(hook.result.current.refreshing).toBe(false);
  });

  it('失败显示错误并等下个周期，不因焦点事件立即重试', async () => {
    const request = vi.fn<() => Promise<number>>().mockRejectedValueOnce(new Error('连接失败')).mockImplementation(async () => Date.now() + INTERVAL);
    const hook = mount(request); await tick(0);
    expect(hook.result.current.error).toBe('连接失败');
    await event(window, 'focus'); await tick(INTERVAL - 1);
    expect(request).toHaveBeenCalledTimes(1);
    await tick(1); expect(request).toHaveBeenCalledTimes(2);
    expect(hook.result.current.error).toBe('');
  });

  it('离线停止请求，重新联网检查数据', async () => {
    const hook = mount(); await tick(0);
    online = false; await event(window, 'offline');
    await tick(INTERVAL * 2); expect(hook.request).toHaveBeenCalledTimes(1);
    online = true; await event(window, 'online');
    expect(hook.request).toHaveBeenCalledTimes(2);
  });

  it('恢复时间早于常规间隔时提前刷新', async () => {
    const hook = mount(undefined, 30_000);
    await tick(29_999); expect(hook.request).not.toHaveBeenCalled();
    await tick(1); expect(hook.request).toHaveBeenCalledTimes(1);
  });

  it('卸载后移除定时器和事件处理，不应用未完成的结果', async () => {
    let complete!: (at: number) => void;
    const request = vi.fn(() => new Promise<number>(resolve => {complete = resolve}));
    const hook = mount(request); await tick(0); hook.unmount();
    await act(async () => {complete(Date.now() + INTERVAL); await Promise.resolve()});
    await tick(INTERVAL * 2); await event(window, 'focus');
    expect(request).toHaveBeenCalledTimes(1); expect(hook.accepted).not.toHaveBeenCalled();
  });
});
