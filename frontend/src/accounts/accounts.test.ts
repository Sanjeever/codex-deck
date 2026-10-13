import {expect, it} from 'vitest';
import {accountName, privateText, usageStale, visibleAccounts} from './accounts';
import type {Account} from '../bridge/api';

const now = Date.now();
const account = (id: string, quota: number | null, age = 0): Account => ({id, account_id: `workspace-${id}`, email: `${id}@example.test`, alias: '', plan: 'plus', current: false, auth_error: '', usage_error: '', credits_error: '', credits: null, pending_reset: null, usage: quota == null ? null : {plan: 'plus', five_hour: {remaining_percent: quota, reset_at: null}, weekly: null, reset_credits: null, queried_at: new Date(now - age).toISOString()}});

it('隐私模式覆盖邮箱、工作区和错误信息，不修改源账号', () => {
 const a = account('alice', 30);
 expect(accountName(a, true)).toBe('••••@••••');
 expect(privateText(`账号 ${a.email}，工作区 ${a.account_id}`, true, [a])).toBe('账号 ••••@••••，工作区 ••••••••');
 expect(a.email).toBe('alice@example.test');
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
