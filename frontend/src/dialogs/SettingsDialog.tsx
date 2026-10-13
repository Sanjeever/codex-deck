import {useEffect, useState} from 'react';
import {Button, Dropdown, Label, Switch} from '@heroui/react';
import {backend, type RestorePreview, type State} from '../bridge/api';
import {dateTime} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import {DialogFrame, Field, type RunOperation} from './DialogFrame';

interface SettingsDialogProps {
  state: Pick<State, 'settings' | 'preferences' | 'backup'>;
  busy: string;
  run: RunOperation;
  safeText: (value: string) => string;
  onClose: () => Promise<void>;
  onOverwrite: () => void;
  onRestore: (preview: RestorePreview) => void;
}

export function SettingsDialog({state, busy, run, safeText, onClose, onOverwrite, onRestore}: SettingsDialogProps) {
  const [settings, setSettings] = useState(() => ({...state.settings}));
  const [autoStart, setAutoStart] = useState<boolean | null>(null);
  const readAutoStart = () => void run('正在读取自启动设置', () => backend().GetAutoStart(), setAutoStart);
  useEffect(() => {void run('正在读取自启动设置', () => backend().GetAutoStart(), setAutoStart)}, [run]);
  const unsaved = JSON.stringify(settings) !== JSON.stringify(state.settings);

  return <DialogFrame title="设置" busy={busy} onClose={onClose} closeLabel="关闭" actions={
    <Button isDisabled={!!busy} onPress={() => void run('正在保存设置', () => backend().SaveSettings(settings), () => notify('success', '设置已保存'))}>保存</Button>
  }>
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-4">
        <Switch isSelected={autoStart === true} isDisabled={!!busy || autoStart === null} onChange={value => void run('正在修改自启动设置', () => backend().SetAutoStart(value), enabled => {
          setAutoStart(enabled);
          notify('success', enabled ? '已开启开机自启动' : '已关闭开机自启动');
        })}>
          <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>开机自启动</Label></Switch.Content>
        </Switch>
        {autoStart === null && !busy && <Button variant="ghost" size="sm" onPress={readAutoStart}>重试读取</Button>}
      </div>
      <div className="flex items-center justify-between gap-4">
        <Switch isSelected={state.preferences.low_quota_notify} isDisabled={!!busy} onChange={value => void run('正在保存通知设置', () => backend().SavePreferences({...state.preferences, low_quota_notify: value}))}>
          <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>低额度通知</Label></Switch.Content>
        </Switch>
        <Dropdown>
          <Dropdown.Trigger isDisabled={!!busy || !state.preferences.low_quota_notify} className="sort-trigger" aria-label="通知阈值">{state.preferences.low_quota_threshold || 10}% ▾</Dropdown.Trigger>
          <Dropdown.Popover><Dropdown.Menu aria-label="通知阈值" onAction={key => void run('正在保存通知设置', () => backend().SavePreferences({...state.preferences, low_quota_threshold: Number(key)}))}>
            {[5, 10, 20].map(value => <Dropdown.Item key={value} id={String(value)}>{value}%</Dropdown.Item>)}
          </Dropdown.Menu></Dropdown.Popover>
        </Dropdown>
      </div>
      <Field label="WebDAV 目录地址" type="url" value={settings.webdav_url} onChange={value => setSettings({...settings, webdav_url: value})} disabled={!!busy} />
      <Field label="用户名" type={state.preferences.privacy ? 'password' : 'text'} value={settings.webdav_username} onChange={value => setSettings({...settings, webdav_username: value})} disabled={!!busy} />
      <Field label="密码" type="password" value={settings.webdav_password} onChange={value => setSettings({...settings, webdav_password: value})} disabled={!!busy} />
      {state.backup.busy && <p role="status" className="text-xs text-muted">正在备份</p>}
      {state.backup.error ? <p className="text-xs text-danger" title={safeText(state.backup.error)}>{state.backup.conflict ? '备份冲突' : '备份失败'}</p> : state.backup.at && <p className="text-xs text-muted">上次备份 · {dateTime(state.backup.at)}</p>}
      <div className="flex gap-2 pt-1">
        {state.backup.conflict && <Button variant="danger" size="sm" isDisabled={!!busy || unsaved} onPress={onOverwrite}>覆盖远端</Button>}
        <Button variant="secondary" size="sm" isDisabled={!!busy || !state.settings.webdav_url || unsaved} onPress={() => void run('正在备份', () => backend().Backup(), () => notify('success', '备份成功'))}>备份</Button>
        <Button variant="secondary" size="sm" isDisabled={!!busy || !state.settings.webdav_url || unsaved} onPress={() => void run('正在下载备份', () => backend().PrepareRestore(), onRestore)}>恢复</Button>
      </div>
    </div>
  </DialogFrame>;
}
