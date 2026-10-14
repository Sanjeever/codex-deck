import {expect, it} from 'vitest';
import {accountName, privateText, recoveryText, updatedAgo, usageStale, visibleAccounts} from './accounts';
import type {Account} from '../bridge/api';

const now = Date.now();
const account = (id: string, quota: number | null, age = 0): Account => ({id, account_id: `workspace-${id}`, email: `${id}@example.test`, alias: '', plan: 'plus', current: false, auth_error: '', usage_error: '', credits_error: '', credits: null, pending_reset: null, usage: quota == null ? null : {plan: 'plus', five_hour: {remaining_percent: quota, reset_at: null}, weekly: null, reset_credits: null, queried_at: new Date(now - age).toISOString()}});

it('隐私模式覆盖邮箱、工作区和错误信息，不修改源账号', () => {
 const a = account('alice', 30);
 expect(accountName(a, true)).toBe('账号 alice');
 expect(privateText(`账号 ${a.email}，工作区 ${a.account_id}`, true, [a])).toBe('账号 ••••@••••，工作区 ••••••••');
 expect(a.email).toBe('alice@example.test');
});

it('按额度窗口排序，筛选排除未知和过期数据，当前账号在匹配范围内置顶', () => {
 const high5h = account('five', 80);
 high5h.usage!.weekly = {remaining_percent: 3, reset_at: null};
 const highWeekly = account('week', 10);
 highWeekly.usage!.weekly = {remaining_percent: 90, reset_at: null};
 const auth = {...account('auth', 0), auth_error: '需要重新登录'};
 const accounts = [highWeekly, account('unknown', null), account('expired', 0, 300_001), high5h, auth];
 expect(visibleAccounts(accounts, '', 'five_hour', now).map(a => a.id)).toEqual(['five', 'week', 'auth', 'unknown', 'expired']);
 expect(visibleAccounts(accounts, '', 'weekly', now).map(a => a.id)).toEqual(['week', 'five', 'unknown', 'expired', 'auth']);
 expect(visibleAccounts(accounts, '', '', now, 'low').map(a => a.id)).toEqual(['week', 'five']);
 expect(visibleAccounts(accounts, '', '', now, 'auth').map(a => a.id)).toEqual(['auth']);
 expect(visibleAccounts(accounts, 'five', '', now, 'low').map(a => a.id)).toEqual(['five']);
 expect(visibleAccounts(accounts, '账号 five', 'weekly', now, 'all', true).map(a => a.id)).toEqual(['five']);
 expect(visibleAccounts(accounts, 'five@example.test', '', now, 'all', true)).toEqual([]);
 expect(accountName(high5h, true)).toBe(accountName([...accounts].reverse().find(a => a.id === 'five')!, true));
});
it('搜索不区分大小写，当前置顶，未知与过期额度排在有效缓存之后', () => {
 const accounts = [account('low', 10), account('unknown', null), account('expired', 100, 300_001), {...account('current', 3), current: true}, account('high', 80)];
 expect(visibleAccounts(accounts, '', 'quota', now).map(a => a.id)).toEqual(['current', 'high', 'low', 'unknown', 'expired']);
 expect(visibleAccounts(accounts, 'HIGH@', '', now).map(a => a.id)).toEqual(['high']);
 expect(visibleAccounts(accounts, 'workspace-low', '', now).map(a => a.id)).toEqual(['low']);
 expect(accounts[0].id).toBe('low');
 expect(usageStale(accounts[2], now)).toBe(true);
});
it('窗口恢复时间已过时将缓存标记为过期', () => {
 const a = account('reset', 0);
 a.usage!.queried_at = new Date(now - 1000).toISOString();
 a.usage!.five_hour!.reset_at = new Date(now - 500).toISOString();
 expect(usageStale(a, now)).toBe(true);
});

it('相对时间区分更新时长、未知恢复和等待服务端确认', () => {
 expect(updatedAgo(new Date(now - 30_000).toISOString(), now)).toBe('刚刚更新');
 expect(updatedAgo(new Date(now - 180_000).toISOString(), now)).toBe('3 分钟前更新');
 expect(updatedAgo(new Date(now - 7_200_000).toISOString(), now)).toBe('2 小时前更新');
 expect(recoveryText(null, now)).toBe('恢复时间未知');
 expect(recoveryText(new Date(now + 42 * 60_000).toISOString(), now)).toBe('约 42 分钟后恢复');
 expect(recoveryText(new Date(now).toISOString(), now)).toBe('等待确认恢复');
 const a = account('low', 0);
 a.usage!.five_hour!.reset_at = new Date(now - 1000).toISOString();
 expect(recoveryText(a.usage!.five_hour!.reset_at, now)).toBe('等待确认恢复');
 expect(a.usage!.five_hour!.remaining_percent).toBe(0);
});
