import {notify} from '../notifications/Notifications';

export async function copyToClipboard(text: string, label: string) {
  try {await navigator.clipboard.writeText(text); notify('success', `${label}已复制`)}
  catch (error) {notify('error', '复制失败', error instanceof Error ? error.message : String(error))}
}
