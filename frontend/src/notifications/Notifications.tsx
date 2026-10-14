import {useEffect} from 'react';
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

export function Notifications({state}: {state: State | null}) {
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
