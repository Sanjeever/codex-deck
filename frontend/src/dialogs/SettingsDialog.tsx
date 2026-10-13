import {useState} from 'react';
import {Button, Dropdown, Label, Modal, Spinner, Switch, Tabs} from '@heroui/react';
import {backend, type RestorePreview, type State} from '../bridge/api';
import {dateTime} from '../accounts/accounts';
import {notify} from '../notifications/Notifications';
import {Field} from './DialogFrame';
import type {RunOperation} from '../app/useDeckController';
import {ProxySettings} from './ProxySettings';
import {useSettingsDraft, type SettingsTab} from './useSettingsDraft';

interface SettingsDialogProps {
  state: Pick<State, 'network' | 'settings' | 'preferences' | 'backup' | 'build_info' | 'update'>;
  busy: string;
  run: RunOperation;
  safeText: (value: string) => string;
  onClose: () => Promise<void>;
  onOverwrite: () => void;
  onRestore: (preview: RestorePreview) => void;
  onUpdate: () => void;
}

const tabs: Record<SettingsTab, string> = {general: '常规', network: '网络', backup: '备份', about: '关于'};

export function SettingsDialog({state, busy, run, safeText, onClose, onOverwrite, onRestore, onUpdate}: SettingsDialogProps) {
  const {draft, setDraft, tab, setTab, autoStart, setAutoStart, readAutoStart, error, dirty, unsaved, save} = useSettingsDraft(state, run);
  const [leave, setLeave] = useState<(() => void) | null>(null);
  const requestLeave = (action: () => void) => {if (unsaved) setLeave(() => action); else action()};
  const close = () => {if (busy) return; if (leave) setLeave(null); else requestLeave(() => void onClose())};
  const preferences = draft.preferences;
  const changePreferences = (patch: Partial<typeof preferences>) => setDraft({...draft, preferences: {...preferences, ...patch}});
  const settings = draft.settings;
  const backupBlocked = dirty.network || dirty.backup;

  return <Modal.Backdrop isOpen onOpenChange={open => {if (!open) close()}} isDismissable={!busy} isKeyboardDismissDisabled={!!busy}>
    <Modal.Container size="lg" scroll="inside" placement="center">
      {leave ? <Modal.Dialog>
        <Modal.Header><Modal.Heading>放弃未保存更改？</Modal.Heading></Modal.Header>
        <Modal.Body><p>设置尚未保存。放弃后将丢失本次修改。</p></Modal.Body>
        <Modal.Footer>
          <Button variant="tertiary" onPress={() => setLeave(null)}>继续编辑</Button>
          <Button variant="danger" onPress={() => {const action = leave; setLeave(null); action()}}>放弃更改</Button>
        </Modal.Footer>
      </Modal.Dialog> : <Modal.Dialog className="settings-dialog">
        <Modal.Header><Modal.Heading>设置</Modal.Heading></Modal.Header>
        <Tabs selectedKey={tab} onSelectionChange={key => setTab(key as SettingsTab)} variant="secondary" className="settings-tabs">
          <Tabs.ListContainer className="shrink-0">
            <Tabs.List aria-label="设置分类" className="w-full">
              {Object.entries(tabs).map(([key, label]) => <Tabs.Tab key={key} id={key} aria-label={label} className="flex-1 gap-2">
                {label}{dirty[key as SettingsTab] && <span aria-label="有未保存更改" className="size-1.5 rounded-full bg-current" />}<Tabs.Indicator />
              </Tabs.Tab>)}
            </Tabs.List>
          </Tabs.ListContainer>
          <Modal.Body className="settings-content">
            {error && (!error.section || error.section === tab) && <p role="alert" className="mb-4 text-sm text-danger break-words">{safeText(error.text)}</p>}
            <Tabs.Panel id="general" className="space-y-6">
              <div className="space-y-2">
                <Switch isSelected={autoStart === true} isDisabled={!!busy || autoStart === null} onChange={setAutoStart}>
                  <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>开机自启动</Label></Switch.Content>
                </Switch>
                {autoStart === null && !busy && <Button variant="ghost" size="sm" onPress={readAutoStart}>重试读取</Button>}
              </div>
              <div className="space-y-3">
                <Switch isSelected={preferences.low_quota_notify} isDisabled={!!busy} onChange={low_quota_notify => changePreferences({low_quota_notify})}>
                  <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>低额度通知</Label></Switch.Content>
                </Switch>
                {preferences.low_quota_notify && <div className="flex items-center justify-between gap-4">
                  <span className="text-sm">通知阈值</span>
                  <Dropdown>
                    <Dropdown.Trigger isDisabled={!!busy} className="sort-trigger" aria-label="通知阈值">{preferences.low_quota_threshold || 10}% ▾</Dropdown.Trigger>
                    <Dropdown.Popover><Dropdown.Menu aria-label="通知阈值" onAction={key => changePreferences({low_quota_threshold: Number(key)})}>
                      {[5, 10, 20].map(value => <Dropdown.Item key={value} id={String(value)}>{value}%</Dropdown.Item>)}
                    </Dropdown.Menu></Dropdown.Popover>
                  </Dropdown>
                </div>}
              </div>
            </Tabs.Panel>
            <Tabs.Panel id="network">
              <ProxySettings draft={draft.network} onChange={network => setDraft({...draft, network})} busy={busy} privacy={state.preferences.privacy} run={run} unsaved={dirty.network} />
            </Tabs.Panel>
            <Tabs.Panel id="backup" className="space-y-6">
              <div className="space-y-4">
                <Field label="WebDAV 目录地址" type="url" value={settings.webdav_url} onChange={webdav_url => setDraft({...draft, settings: {...settings, webdav_url}})} disabled={!!busy} />
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label="用户名" type={state.preferences.privacy ? 'password' : 'text'} value={settings.webdav_username} onChange={webdav_username => setDraft({...draft, settings: {...settings, webdav_username}})} disabled={!!busy} />
                  <Field label="密码" type="password" value={settings.webdav_password} onChange={webdav_password => setDraft({...draft, settings: {...settings, webdav_password}})} disabled={!!busy} />
                </div>
              </div>
              <div className="space-y-3">
                {state.backup.busy && <p role="status" className="text-xs text-muted">正在备份</p>}
                {state.backup.error ? <p className="text-xs text-danger break-words">{state.backup.conflict ? '备份冲突' : '备份失败'} · {safeText(state.backup.error)}</p> : <p className="text-xs text-muted">{state.backup.at ? `上次备份 · ${dateTime(state.backup.at)}` : '暂无备份记录'}</p>}
                {backupBlocked && <p className="text-xs text-muted">请先保存网络与备份设置。</p>}
                <div className="flex flex-wrap gap-2">
                  {state.backup.conflict && <Button variant="danger" size="sm" isDisabled={!!busy || backupBlocked} onPress={() => requestLeave(onOverwrite)}>覆盖远端</Button>}
                  <Button variant="secondary" size="sm" isDisabled={!!busy || !state.settings.webdav_url || backupBlocked} onPress={() => void run('正在备份', () => backend().Backup(), () => notify('success', '备份成功'))}>立即备份</Button>
                  <Button variant="secondary" size="sm" isDisabled={!!busy || !state.settings.webdav_url || backupBlocked} onPress={() => requestLeave(() => void run('正在下载备份', () => backend().PrepareRestore(), onRestore))}>恢复备份</Button>
                </div>
              </div>
            </Tabs.Panel>
            <Tabs.Panel id="about" className="space-y-6">
              <div className="space-y-3">
                <p className="text-base font-medium">Codex Deck v{state.build_info.version}{state.build_info.development && ' · 开发版'}</p>
                {state.build_info.commit && <details>
                  <summary className="cursor-pointer text-sm text-muted">构建信息</summary>
                  <Button variant="ghost" size="sm" className="mt-2" onPress={() => void navigator.clipboard.writeText(state.build_info.commit).then(() => notify('success', '提交号已复制'), error => notify('error', '复制失败', String(error)))}>提交 {state.build_info.commit.slice(0, 8)} · 复制</Button>
                </details>}
              </div>
              <Switch isSelected={!preferences.disable_update_check} isDisabled={!!busy || state.build_info.development} onChange={value => changePreferences({disable_update_check: !value})}>
                <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control><Label>自动检查更新</Label></Switch.Content>
              </Switch>
              <div className="space-y-3">
                {dirty.network && <p className="text-xs text-muted">请先保存网络设置。</p>}
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="secondary" isDisabled={!!busy || !!state.update.phase || dirty.network} onPress={() => void run('正在检查更新', () => backend().CheckUpdate(), () => {if (!state.update.version) notify('info', '检查更新完成')})}>检查更新</Button>
                  {state.update.version && <Button size="sm" variant="secondary" isDisabled={!!busy || dirty.network} onPress={() => requestLeave(onUpdate)}>新版本 v{state.update.version}</Button>}
                </div>
                {state.update.error && <p className="text-xs text-danger break-words">{safeText(state.update.error)}</p>}
                {state.update.checked_at && <p className="text-xs text-muted">上次检查 · {dateTime(state.update.checked_at)}</p>}
              </div>
            </Tabs.Panel>
            {busy && <p role="status" className="mt-4 flex items-center gap-2 text-xs text-muted"><Spinner size="sm" />{busy}</p>}
          </Modal.Body>
        </Tabs>
        <Modal.Footer className="items-center">
          <span className="mr-auto text-xs text-muted">{unsaved ? '有未保存更改' : ''}</span>
          <Button variant="tertiary" isDisabled={!!busy} onPress={close}>关闭</Button>
          <Button isDisabled={!!busy || !unsaved} onPress={save}>保存更改</Button>
        </Modal.Footer>
      </Modal.Dialog>}
    </Modal.Container>
  </Modal.Backdrop>;
}
