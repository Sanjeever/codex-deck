import {Button} from '@heroui/react';
import {copyToClipboard} from '../bridge/clipboard';

export function ErrorDetails({label, message, busy, onAction, actionLabel = '重试'}: {label: string; message: string; busy: string; onAction?: () => void; actionLabel?: string}) {
  return <details className="error-details">
    <summary>{label}</summary>
    <div className="error-details-body">
      <p className="break-words">{message}</p>
      <div className="flex flex-wrap gap-2">
        {onAction && <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={onAction}>{actionLabel}</Button>}
        <Button size="sm" variant="ghost" onPress={() => void copyToClipboard(message, '错误信息')}>复制错误</Button>
      </div>
    </div>
  </details>;
}
