import type {ReactNode} from 'react';
import {Modal} from '@heroui/react';
import type {Account, AuthImportPreview, ResetRequest, RestorePreview, State} from '../bridge/api';
import {privateText} from '../accounts/accounts';
import type {RunOperation} from '../app/useDeckController';
import {AccountDialogs} from './AccountDialogs';
import {BackupDialogs} from './BackupDialogs';
import {CreditDialogs} from './CreditDialogs';
import {SettingsDialog} from './SettingsDialog';
import {UpdateDialog} from './UpdateDialog';

export type Dialog =
  | {kind: 'settings'}
  | {kind: 'update'}
  | {kind: 'overwrite'}
  | {kind: 'login'; id: string}
  | {kind: 'import'; preview: AuthImportPreview}
  | {kind: 'export'; account: Account}
  | {kind: 'alias'; account: Account}
  | {kind: 'delete'; account: Account}
  | {kind: 'switch'; account: Account}
  | {kind: 'credits'; account: Account}
  | {kind: 'consume'; account: Account; request: ResetRequest}
  | {kind: 'restore'; preview: RestorePreview};

export interface DialogProps {
  state: State;
  busy: string;
  run: RunOperation;
  onClose: () => Promise<void>;
  setDialog: (dialog: Dialog | null) => void;
}

interface DialogHostProps extends DialogProps {
  dialog: Dialog | null;
  useCredit: (account: Account, creditID?: string) => void;
}

export function DialogHost({dialog, useCredit, ...props}: DialogHostProps) {
  let content: ReactNode = null;
  switch (dialog?.kind) {
    case 'settings':
      return <SettingsDialog {...props} safeText={value => privateText(value, props.state.preferences.privacy, props.state.accounts)} onOverwrite={() => props.setDialog({kind: 'overwrite'})} onRestore={preview => props.setDialog({kind: 'restore', preview})} onUpdate={() => props.setDialog({kind: 'update'})} />;
    case 'update': content = <UpdateDialog {...props} />; break;
    case 'login': case 'alias': case 'delete': case 'switch': case 'import': case 'export':
      content = <AccountDialogs {...props} dialog={dialog} />;
      break;
    case 'overwrite': case 'restore':
      content = <BackupDialogs {...props} dialog={dialog} />;
      break;
    case 'consume': case 'credits':
      content = <CreditDialogs {...props} dialog={dialog} useCredit={useCredit} />;
      break;
  }
  const installing = dialog?.kind === 'update' && !!props.state.update.phase;
  return <Modal.Backdrop isOpen={!!dialog} onOpenChange={open => {if (!open) void props.onClose()}} isDismissable={!props.busy && !installing} isKeyboardDismissDisabled={installing || (!!props.busy && dialog?.kind !== 'login')}>
    <Modal.Container size={dialog?.kind === 'credits' ? 'lg' : 'sm'} placement="center">{content}</Modal.Container>
  </Modal.Backdrop>;
}
