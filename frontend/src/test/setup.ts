import {afterEach, beforeEach, vi} from 'vitest';

beforeEach(() => {
  // jsdom 不提供媒体查询，Toast 通过该浏览器接口判断布局和动画。
  vi.stubGlobal('matchMedia', vi.fn((media: string) => ({
    media, matches: media.includes('prefers-reduced-motion'), onchange: null,
    addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {},
    dispatchEvent: () => true,
  })));
  vi.stubGlobal('ResizeObserver', class {
    observe() {} unobserve() {} disconnect() {}
  });
  // jsdom 不实现 Web Animations，Tabs 指示条会读取当前动画。
  Object.defineProperty(Element.prototype, 'getAnimations', {configurable: true, value: vi.fn(() => [])});
});
afterEach(() => {vi.unstubAllGlobals(); Reflect.deleteProperty(Element.prototype, 'getAnimations')});
