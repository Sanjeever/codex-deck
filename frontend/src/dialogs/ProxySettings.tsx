import {useState} from 'react';
import {Button, Dropdown} from '@heroui/react';
import {backend, type NetworkConfig} from '../bridge/api';
import {notify} from '../notifications/Notifications';
import type {RunOperation} from '../app/useDeckController';
import {Field} from './DialogFrame';

const modes: Record<NetworkConfig['mode'], string> = {environment: '默认代理', direct: '不使用代理', custom: '自定义 HTTP 代理'};

export function ProxySettings({config, busy, privacy, run}: {config: NetworkConfig; busy: string; privacy: boolean; run: RunOperation}) {
  const [draft, setDraft] = useState(() => ({...config}));
  const unsaved = JSON.stringify(draft) !== JSON.stringify(config);
  return <section aria-label="网络代理" className="space-y-3">
    <div className="flex items-center justify-between gap-4">
      <p className="text-sm">网络代理</p>
      <Dropdown>
        <Dropdown.Trigger aria-label="代理模式" isDisabled={!!busy} className="sort-trigger">{modes[draft.mode]} ▾</Dropdown.Trigger>
        <Dropdown.Popover><Dropdown.Menu aria-label="代理模式" onAction={key => setDraft({...draft, mode: String(key) as NetworkConfig['mode']})}>
          {Object.entries(modes).map(([mode, label]) => <Dropdown.Item id={mode} key={mode}>{label}</Dropdown.Item>)}
        </Dropdown.Menu></Dropdown.Popover>
      </Dropdown>
    </div>
    {draft.mode === 'environment' && <p className="text-xs text-muted">沿用启动时的代理环境变量。</p>}
    {draft.mode === 'custom' && <>
      <Field label="代理地址" type="url" value={draft.url} onChange={url => setDraft({...draft, url})} disabled={!!busy} />
      <p className="text-xs text-muted">例如 http://127.0.0.1:7897</p>
      <Field label="代理用户名" type={privacy ? 'password' : 'text'} value={draft.username} onChange={username => setDraft({...draft, username})} disabled={!!busy} />
      <Field label="代理密码" type="password" value={draft.password} onChange={password => setDraft({...draft, password})} disabled={!!busy} />
    </>}
    <div className="flex gap-2">
      <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={() => void run('正在测试代理连接', () => backend().TestNetwork(draft), () => notify('success', '连接测试成功'))}>测试连接</Button>
      <Button size="sm" variant="secondary" isDisabled={!!busy || !unsaved} onPress={() => void run('正在保存代理设置', () => backend().SaveNetwork(draft), saved => {setDraft(saved); notify('success', '代理设置已保存')})}>保存代理</Button>
    </div>
    <p className="text-xs text-muted">仅本机生效，不上传到 WebDAV。浏览器授权页使用浏览器代理。</p>
  </section>;
}
