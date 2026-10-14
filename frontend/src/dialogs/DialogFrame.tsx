import type {ReactNode} from 'react';
import {Button, Input, Label, Modal, Spinner, TextField} from '@heroui/react';

interface DialogFrameProps {
  title: string;
  busy: string;
  onClose: () => Promise<void>;
  children: ReactNode;
  actions?: ReactNode;
  closeLabel?: string;
}

export function DialogFrame({title, busy, onClose, children, actions, closeLabel = '取消'}: DialogFrameProps) {
  return <Modal.Dialog>
    <Modal.Header><Modal.Heading>{title}</Modal.Heading></Modal.Header>
    <Modal.Body className="space-y-4">
      {children}
      {busy && <p role="status" className="flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
    </Modal.Body>
    <Modal.Footer>
      <Button variant="tertiary" isDisabled={!!busy} onPress={() => void onClose()}>{closeLabel}</Button>
      {actions}
    </Modal.Footer>
  </Modal.Dialog>;
}

export function Field({label, value, onChange, type = 'text', disabled = false, placeholder}: {label: string; value: string; onChange: (value: string) => void; type?: 'text' | 'password' | 'url'; disabled?: boolean; placeholder?: string}) {
  return <TextField value={value} onChange={onChange} isDisabled={disabled} className="w-full">
    <Label>{label}</Label><Input type={type} placeholder={placeholder} autoComplete="off" spellCheck={false} />
  </TextField>;
}
