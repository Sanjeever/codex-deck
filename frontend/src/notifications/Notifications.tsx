import {useEffect, useRef} from 'react';
import {Toast, toast} from '@heroui/react';
import type {State} from '../bridge/api';
import {privateText} from '../accounts/accounts';

type NoticeKind = 'success' | 'info' | 'error';

export function notify(kind: NoticeKind, title: string, description?: string) {
  if (document.visibilityState !== 'visible') return;
  const message = description || title;
  if (toast.getQueue().visibleToasts.some(item => (item.content.description || item.content.title) === message)) return;
  const options = {description, timeout: kind === 'success' ? 5000 : 8000};
  if (kind === 'error') toast.danger(title, options);
  else if (kind === 'info') toast.info(title, options);
  else toast.success(title, options);
}

export function Notifications({state, automaticError}: {state: State | null; automaticError: string}) {
  const previous = useRef(new Map<string, string>());
  const privacy = state?.preferences.privacy ?? false;
  const safeText = (text: string) => privateText(text, privacy, state?.accounts ?? []);

  useEffect(() => {
    toast.clear();
  }, [privacy]);
  useEffect(() => {
    const clearHidden = () => {if (document.visibilityState !== 'visible') toast.clear()};
    document.addEventListener('visibilitychange', clearHidden);
    return () => {document.removeEventListener('visibilitychange', clearHidden); toast.clear()};
  }, []);
  useEffect(() => {
    const errors: [string, string, string][] = [
      ['automatic', '自动刷新失败', automaticError],
      ['desktop', '桌面功能异常', state?.desktop_error ?? ''],
      ['cli', 'CLI 账号同步失败', state?.cli_error ?? ''],
      ['backup', state?.backup.conflict ? '备份冲突' : '备份失败', state?.backup.error ?? ''],
    ];
    for (const account of state?.accounts ?? []) {
      const name = account.alias || account.email || account.account_id;
      errors.push(
        [`${account.id}:auth`, `${name} · 需重新登录`, account.auth_error],
        [`${account.id}:usage`, `${name} · 用量查询失败`, account.usage_error],
        [`${account.id}:credits`, `${name} · 重置卡查询失败`, account.credits_error],
      );
    }
    for (const [key, title, message] of errors) {
      if (message && previous.current.get(key) !== message) notify('error', title, message);
    }
    previous.current = new Map(errors.map(([key, , message]) => [key, message]));
  }, [state, automaticError]);

  return <Toast.Provider placement="bottom end" maxVisibleToasts={3} width={360} aria-label="操作消息">
    {({toast: item}) => <Toast toast={item} variant={item.content.variant}>
      <Toast.Indicator variant={item.content.variant} />
      <Toast.Content>
        <Toast.Title>{safeText(item.content.title as string)}</Toast.Title>
        {item.content.description && <Toast.Description>{safeText(item.content.description as string)}</Toast.Description>}
      </Toast.Content>
      <Toast.CloseButton aria-label="关闭消息" />
    </Toast>}
  </Toast.Provider>;
}
