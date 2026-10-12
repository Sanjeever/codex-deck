import type {Account, PreviewAccount} from './api';

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
