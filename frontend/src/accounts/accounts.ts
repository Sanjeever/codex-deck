import type {Account, PreviewAccount} from '../bridge/api';

export function dateTime(value: string | null | undefined) {return value ? new Date(value).toLocaleString('zh-CN', {hour12: false}) : '未知'}

export const LOW_QUOTA_THRESHOLD = 15;

export function updatedAgo(value: string, now = Date.now()) {
  const minutes = Math.floor((now - Date.parse(value)) / 60_000);
  if (minutes < 1) return '刚刚更新';
  if (minutes < 60) return `${minutes} 分钟前更新`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前更新`;
  return `${Math.floor(minutes / 1440)} 天前更新`;
}

export function recoveryText(value: string | null, now = Date.now()) {
  if (!value) return '恢复时间未知';
  const remaining = Date.parse(value) - now;
  if (remaining <= 0) return '等待确认恢复';
  const minutes = Math.ceil(remaining / 60_000);
  if (minutes < 60) return `约 ${minutes} 分钟后恢复`;
  if (minutes < 1440) return `约 ${Math.ceil(minutes / 60)} 小时后恢复`;
  return `约 ${Math.ceil(minutes / 1440)} 天后恢复`;
}

export function creditCount(a: Account) {
  if (a.usage?.reset_credits != null && (!a.credits || new Date(a.usage.queried_at) >= new Date(a.credits.queried_at))) return a.usage.reset_credits;
  return a.credits?.available_count ?? a.usage?.reset_credits ?? null;
}

export function pendingConsumption(a: Account) {return !!a.pending_reset?.started && !a.pending_reset.result}

export function privateText(text: string, privacy: boolean, accounts: Pick<Account, 'email' | 'account_id'>[]) {
  if (!privacy) return text;
  let result = text.replace(/[\w.+-]+@[\w.-]+/g, '••••@••••');
  for (const account of accounts) {
    if (account.email) result = result.split(account.email).join('••••@••••');
    if (account.account_id) result = result.split(account.account_id).join('••••••••');
  }
  return result;
}

export function accountName(a: PreviewAccount, privacy = false) {
  return privateText(a.alias || a.email || a.account_id, privacy, [a]);
}
export function workspaceName(id: string, privacy: boolean) {return privacy ? '••••••••' : id}
export function usageStale(account: Account, now = Date.now()) {
  const usage = account.usage;
  if (!usage) return false;
  const at = Date.parse(usage.queried_at);
  return now - at >= 300_000 || [usage.five_hour, usage.weekly].some(window => window?.reset_at && Date.parse(window.reset_at) <= now && at < Date.parse(window.reset_at));
}

export function visibleAccounts(accounts: Account[], query: string, sort: string, now = Date.now()) {
  const needle = query.trim().toLocaleLowerCase();
  const quota = (a: Account) => {
    if (!a.usage || usageStale(a, now)) return null;
    const values = [a.usage.five_hour, a.usage.weekly].flatMap(window => window ? [window.remaining_percent] : []);
    return values.length ? Math.min(...values) : null;
  };
  return accounts.filter(a => [a.alias, a.email, a.account_id].some(value => value.toLocaleLowerCase().includes(needle))).sort((a, b) => {
    if (a.current !== b.current) return a.current ? -1 : 1;
    if (sort === 'name') return accountName(a).localeCompare(accountName(b), 'zh-CN');
    if (sort === 'quota') {
      const left = quota(a), right = quota(b);
      if (left == null || right == null) return left == null ? right == null ? 0 : 1 : -1;
      return right - left;
    }
    return 0;
  });
}
