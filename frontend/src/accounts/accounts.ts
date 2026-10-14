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
  if (privacy && !a.alias && 'id' in a && typeof a.id === 'string') return `账号 ${a.id.slice(0, 8)}`;
  return privateText(a.alias || a.email || a.account_id, privacy, [a]);
}
export function workspaceName(id: string, privacy: boolean) {return privacy ? '••••••••' : id}
export function usageStale(account: Account, now = Date.now()) {
  const usage = account.usage;
  if (!usage) return false;
  const at = Date.parse(usage.queried_at);
  return now - at >= 300_000 || [usage.five_hour, usage.weekly].some(window => window?.reset_at && Date.parse(window.reset_at) <= now && at < Date.parse(window.reset_at));
}

export type AccountFilter = 'all' | 'low' | 'auth';

export function visibleAccounts(accounts: Account[], query: string, sort: string, now = Date.now(), filter: AccountFilter = 'all', privacy = false) {
  const needle = query.trim().toLocaleLowerCase();
  const quota = (a: Account) => {
    if (!a.usage || usageStale(a, now)) return null;
    if (sort === 'five_hour') return a.usage.five_hour?.remaining_percent ?? null;
    if (sort === 'weekly') return a.usage.weekly?.remaining_percent ?? null;
    const values = [a.usage.five_hour, a.usage.weekly].flatMap(window => window ? [window.remaining_percent] : []);
    return values.length ? Math.min(...values) : null;
  };
  return accounts.filter(a => {
    const fields = privacy ? [accountName(a, true)] : [a.alias, a.email, a.account_id];
    if (!fields.some(value => value.toLocaleLowerCase().includes(needle))) return false;
    if (filter === 'auth') return !!a.auth_error;
    if (filter === 'low') return !a.auth_error && !!a.usage && !usageStale(a, now) && [a.usage.five_hour, a.usage.weekly].some(window => window && window.remaining_percent < LOW_QUOTA_THRESHOLD);
    return true;
  }).sort((a, b) => {
    if (a.current !== b.current) return a.current ? -1 : 1;
    if (sort === 'name') return accountName(a, privacy).localeCompare(accountName(b, privacy), 'zh-CN');
    if (sort === 'quota' || sort === 'five_hour' || sort === 'weekly') {
      const left = quota(a), right = quota(b);
      if (left == null || right == null) return left == null ? right == null ? 0 : 1 : -1;
      return right - left;
    }
    return 0;
  });
}
