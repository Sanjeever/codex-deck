import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import {createAccount} from '../test/fixtures';
import {AccountTable} from './AccountTable';

afterEach(cleanup);

it('表格警示与筛选随低额度阈值变化，关闭通知不影响判断', () => {
  const account = createAccount({usage: {plan: 'plus', five_hour: {remaining_percent: 20, reset_at: null}, weekly: null, reset_credits: null, queried_at: new Date().toISOString()}});
  const props = {accounts: [account], privacy: false, busy: '', query: '', sort: '', searchRef: {current: null}, onQuery: vi.fn(), onSort: vi.fn(), onAction: vi.fn(), onAdd: vi.fn()};
  const view = render(<AccountTable {...props} lowQuotaThreshold={20} />);
  expect(screen.getByRole('meter', {name: '5H 剩余，额度偏低'})).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '账号筛选'}));
  fireEvent.click(within(screen.getByRole('menu')).getByText('额度不足'));
  expect(screen.getByRole('meter', {name: '5H 剩余，额度偏低'})).toBeTruthy();
  view.rerender(<AccountTable {...props} lowQuotaThreshold={10} />);
  expect(screen.getByText('没有匹配账号')).toBeTruthy();
});

it('筛选账号、清除无结果筛选，并将窗口排序交给偏好保存', async () => {
  const accounts = [createAccount({id: 'healthy', email: 'healthy@example.test'}), createAccount({id: 'expired', email: 'expired@example.test', auth_error: '登录已失效'})];
  const onSort = vi.fn();
  render(<AccountTable accounts={accounts} privacy={false} busy="" query="" sort="" searchRef={{current: null}} onQuery={vi.fn()} onSort={onSort} onAction={vi.fn()} onAdd={vi.fn()} />);
  fireEvent.click(screen.getByRole('button', {name: '账号筛选'}));
  fireEvent.click(within(screen.getByRole('menu')).getByText('需重新登录'));
  expect(screen.queryByText('healthy@example.test')).toBeNull();
  expect(screen.getByText('expired@example.test')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '账号筛选'}));
  fireEvent.click(within(screen.getByRole('menu')).getByText('额度不足'));
  expect(screen.getByText('没有匹配账号')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: '清除筛选'}));
  expect(screen.getByText('healthy@example.test')).toBeTruthy();
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '账号排序'}))});
  fireEvent.click(within(screen.getByRole('menu')).getByText('Weekly 剩余额度'));
  expect(onSort).toHaveBeenCalledExactlyOnceWith('weekly');
});
