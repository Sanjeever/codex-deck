import {useEffect, useState, type RefObject} from 'react';
import {Button, Dropdown, Input, Meter, TextField} from '@heroui/react';
import type {Account, QuotaWindow} from '../bridge/api';
import {ErrorDetails} from '../app/ErrorDetails';
import {accountName, creditCount, dateTime, LOW_QUOTA_THRESHOLD, pendingConsumption, privateText, recoveryText, updatedAgo, usageStale, visibleAccounts, workspaceName} from './accounts';
import type {AccountFilter} from './accounts';

interface AccountTableProps {
  accounts: Account[];
  privacy: boolean;
  busy: string;
  query: string;
  sort: string;
  searchRef: RefObject<HTMLInputElement | null>;
  onQuery: (value: string) => void;
  onSort: (value: string) => void;
  onAction: (action: string, account: Account) => void;
  onAdd: () => void;
}

export function AccountTable({accounts, privacy, busy, query, sort, searchRef, onQuery, onSort, onAction, onAdd}: AccountTableProps) {
  const [now, setNow] = useState(Date.now);
  const [filter, setFilter] = useState<AccountFilter>('all');
  const [compact, setCompact] = useState(false);
  useEffect(() => {
    const update = () => setNow(Date.now());
    const timer = window.setInterval(update, 60_000);
    document.addEventListener('visibilitychange', update);
    return () => {window.clearInterval(timer); document.removeEventListener('visibilitychange', update)};
  }, []);
  const visible = visibleAccounts(accounts, query, sort, now, filter, privacy);
  if (accounts.length === 0) return <div className="empty-state"><Button variant="secondary" isDisabled={!!busy} onPress={onAdd}>添加账号</Button></div>;
  return <>
    <div className="account-toolbar">
      <TextField value={query} onChange={onQuery} aria-label="搜索账号" className="max-w-64"><Input ref={searchRef} placeholder="搜索账号" autoComplete="off" /></TextField>
      <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" variant="ghost" aria-label="紧凑行距" aria-pressed={compact} onPress={() => setCompact(value => !value)}>{compact ? '紧凑行距' : '标准行距'}</Button>
      <Dropdown>
        <Dropdown.Trigger className="sort-trigger" aria-label="账号筛选">{{all: '全部账号', low: '额度不足', auth: '需重新登录'}[filter]} ▾</Dropdown.Trigger>
        <Dropdown.Popover><Dropdown.Menu aria-label="账号筛选" selectionMode="single" selectedKeys={[filter]} onAction={key => setFilter(String(key) as AccountFilter)}>
          <Dropdown.Item id="all">全部账号</Dropdown.Item>
          <Dropdown.Item id="low">额度不足</Dropdown.Item>
          <Dropdown.Item id="auth">需重新登录</Dropdown.Item>
        </Dropdown.Menu></Dropdown.Popover>
      </Dropdown>
      <Dropdown>
        <Dropdown.Trigger isDisabled={!!busy} className="sort-trigger" aria-label="账号排序">{({name: '账号名称', quota: '综合剩余额度', five_hour: '5H 剩余额度', weekly: 'Weekly 剩余额度'} as Record<string, string>)[sort] || '默认排序'} ▾</Dropdown.Trigger>
        <Dropdown.Popover><Dropdown.Menu aria-label="账号排序" selectionMode="single" selectedKeys={[sort || 'default']} onAction={key => onSort(String(key) === 'default' ? '' : String(key))}>
          <Dropdown.Item id="default">默认排序</Dropdown.Item>
          <Dropdown.Item id="name">账号名称</Dropdown.Item>
          <Dropdown.Item id="quota">综合剩余额度</Dropdown.Item>
          <Dropdown.Item id="five_hour">5H 剩余额度</Dropdown.Item>
          <Dropdown.Item id="weekly">Weekly 剩余额度</Dropdown.Item>
        </Dropdown.Menu></Dropdown.Popover>
      </Dropdown>
      </div>
    </div>
    {visible.length === 0 && <div className="py-12 text-center text-muted"><p>没有匹配账号</p><Button size="sm" variant="ghost" onPress={() => {setFilter('all'); onQuery('')}}>清除筛选</Button></div>}
    <div className={`table-scroll ${compact ? 'table-compact' : ''}`}><table aria-label="Codex 账号">
      <thead><tr><th>账号</th><th>套餐</th><th>5H 剩余</th><th>Weekly 剩余</th><th>重置卡</th><th className="text-right">操作</th></tr></thead>
      <tbody>{visible.map(account => <AccountRow key={account.id} account={account} accounts={accounts} privacy={privacy} busy={busy} now={now} onAction={onAction} />)}</tbody>
    </table></div>
  </>;
}

function AccountRow({account, accounts, privacy, busy, now, onAction}: Pick<AccountTableProps, 'accounts' | 'privacy' | 'busy' | 'onAction'> & {account: Account; now: number}) {
  const name = accountName(account, privacy);
  const count = creditCount(account);
  const stale = usageStale(account, now);
  const pending = pendingConsumption(account);
  const safeText = (value: string) => privateText(value, privacy, accounts);
  return <tr className={stale ? 'usage-stale' : undefined}>
    <td className="account-cell">
      <div className="flex items-center gap-2"><span className="font-medium break-all">{name}</span>{account.current && <span className="current-tag">当前</span>}</div>
      {account.alias && account.email && <p className="mt-1 text-xs text-muted break-all">{privacy ? '••••@••••' : account.email}</p>}
      <p className="mt-1 text-xs text-muted" title={workspaceName(account.account_id, privacy)}>工作区 {workspaceName(account.account_id, privacy).slice(0, 12)}</p>
      {account.auth_error && <ErrorDetails label="需重新登录" message={safeText(account.auth_error)} busy={busy} onAction={() => onAction('login', account)} actionLabel="重新登录" />}
      {account.usage_error && <ErrorDetails label="用量查询失败" message={safeText(account.usage_error)} busy={busy} onAction={() => onAction('refresh', account)} actionLabel="重试此账号" />}
      {account.credits_error && <ErrorDetails label="重置卡查询失败" message={safeText(account.credits_error)} busy={busy} onAction={() => onAction('credits', account)} actionLabel="重试重置卡查询" />}
      {account.usage && <p className="mt-2 text-[11px] text-muted" title={dateTime(account.usage.queried_at)}>{stale ? '已过期 · ' : account.usage_error ? '缓存 · ' : ''}{updatedAgo(account.usage.queried_at, now)}</p>}
    </td>
    <td><span className="plan-name">{account.plan || '未知'}</span></td>
    <td><Quota label="5H" value={account.usage?.five_hour} now={now} /></td>
    <td><Quota label="Weekly" value={account.usage?.weekly} now={now} /></td>
    <td><Button size="sm" variant="ghost" className="px-0" isDisabled={!!busy} onPress={() => onAction('credits', account)}>{count == null ? '未知' : `${count} 张`}</Button></td>
    <td><div className="flex justify-end items-center gap-2">
      {!account.current && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => onAction('switch', account)}>切换</Button>}
      <Dropdown>
        <Dropdown.Trigger aria-label={`${name} 操作`} isDisabled={!!busy} className="row-menu">···</Dropdown.Trigger>
        <Dropdown.Popover><Dropdown.Menu aria-label="账号操作" onAction={key => onAction(String(key), account)}>
          <Dropdown.Item id="alias">编辑别名</Dropdown.Item>
          <Dropdown.Item id="login">重新登录</Dropdown.Item>
          <Dropdown.Item id="refresh">刷新用量</Dropdown.Item>
          <Dropdown.Item id="credits">重置卡详情</Dropdown.Item>
          <Dropdown.Item id="consume" isDisabled={count === 0 && !pending}>{pending ? '重试重置卡' : '使用重置卡'}</Dropdown.Item>
          <Dropdown.Item id="export">导出 auth.json</Dropdown.Item>
          <Dropdown.Item id="delete" variant="danger" isDisabled={account.current}>删除</Dropdown.Item>
        </Dropdown.Menu></Dropdown.Popover>
      </Dropdown>
    </div></td>
  </tr>;
}

function Quota({label, value, now}: {label: string; value: QuotaWindow | null | undefined; now: number}) {
  if (!value) return <span className="text-muted">未知</span>;
  const percent = Math.round(value.remaining_percent * 10) / 10;
  return <div className="quota">
    <Meter aria-label={`${label} 剩余`} value={percent} minValue={0} maxValue={100} className="gap-2" size="sm">
      <Meter.Output className="tabular-nums text-sm">剩余 {percent}%</Meter.Output>
      <Meter.Track className="h-1"><Meter.Fill className={percent < LOW_QUOTA_THRESHOLD ? 'quota-low' : ''} /></Meter.Track>
    </Meter>
    {percent < LOW_QUOTA_THRESHOLD && <p className="mt-1 text-xs text-warning">额度偏低</p>}
    <p className="mt-2 text-xs text-muted" title={dateTime(value.reset_at)}>{recoveryText(value.reset_at, now)}</p>
  </div>;
}
