import {useState} from 'react';
import {Button, Label, Radio, RadioGroup} from '@heroui/react';
import {backend, type NetworkConfig} from '../bridge/api';
import type {RunOperation} from '../app/useDeckController';
import {Field} from './DialogFrame';

const modes: Record<NetworkConfig['mode'], string> = {environment: '跟随环境变量', direct: '不使用代理', custom: '自定义代理'};

export function ProxySettings({draft, onChange, busy, privacy, run, unsaved}: {draft: NetworkConfig; onChange: (config: NetworkConfig) => void; busy: string; privacy: boolean; run: RunOperation; unsaved: boolean}) {
  const [tested, setTested] = useState<{config: string; error: string} | null>(null);
  const [authOpen, setAuthOpen] = useState(!!draft.username || !!draft.password);
  const configKey = JSON.stringify(draft);
  const change = (config: NetworkConfig) => {setTested(null); onChange(config)};
  const test = () => void run('正在测试代理连接', () => backend().TestNetwork(draft), () => setTested({config: configKey, error: ''}), error => setTested({config: configKey, error}));
  return <section aria-label="Codex Deck 代理" className="space-y-5">
    <h3 className="text-sm font-medium">Codex Deck 代理</h3>
    <RadioGroup aria-label="代理模式" orientation="horizontal" value={draft.mode} isDisabled={!!busy} onChange={mode => change({...draft, mode: mode as NetworkConfig['mode']})} className="flex flex-wrap gap-x-5 gap-y-3">
      {Object.entries(modes).map(([mode, label]) => <Radio key={mode} value={mode}>
        <Radio.Content><Radio.Control><Radio.Indicator /></Radio.Control><Label>{label}</Label></Radio.Content>
      </Radio>)}
    </RadioGroup>
    {draft.mode === 'custom' && <>
      <div className="space-y-2">
        <Field label="代理地址" type="url" placeholder="http://127.0.0.1:7897" value={draft.url} onChange={url => change({...draft, url})} disabled={!!busy} />
      </div>
      <details open={authOpen} onToggle={event => setAuthOpen(event.currentTarget.open)}>
        <summary className="cursor-pointer text-sm text-foreground">代理认证（可选）</summary>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <Field label="代理用户名" type={privacy ? 'password' : 'text'} value={draft.username} onChange={username => change({...draft, username})} disabled={!!busy} />
          <Field label="代理密码" type="password" value={draft.password} onChange={password => change({...draft, password})} disabled={!!busy} />
        </div>
      </details>
    </>}
    <div className="flex flex-wrap items-center gap-3">
      <Button size="sm" variant="secondary" isDisabled={!!busy} onPress={test}>测试连接</Button>
      {tested?.config === configKey && <p role={tested.error ? 'alert' : 'status'} className={`text-xs ${tested.error ? 'text-danger' : 'text-muted'}`}>{tested.error || `连接成功${unsaved ? ' · 尚未保存' : ''}`}</p>}
    </div>
  </section>;
}
