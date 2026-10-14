import {useState} from 'react';
import {Button} from '@heroui/react';
import {backend, type NetworkConfig} from '../bridge/api';
import type {RunOperation} from '../app/useDeckController';
import type {useCLIProxyDraft} from './useCLIProxyDraft';
import {Field} from './DialogFrame';

export function CLIProxySettings({proxy, deckNetwork, busy, privacy, safeText, run}: {proxy: ReturnType<typeof useCLIProxyDraft>; deckNetwork: NetworkConfig; busy: string; privacy: boolean; safeText: (text: string) => string; run: RunOperation}) {
  const [tested, setTested] = useState<{url: string; error: string} | null>(null);
  const reuse = () => {
    const address = new URL(deckNetwork.url.trim());
    address.username = encodeURIComponent(deckNetwork.username);
    address.password = encodeURIComponent(deckNetwork.password);
    proxy.change({url: address.toString(), remove: false}); setTested(null);
  };
  const test = () => void run('正在测试 CLI 代理', async () => {
    try {await backend().TestCLIProxy(proxy.draft.url); setTested({url: proxy.draft.url, error: ''})}
    catch (error) {setTested({url: proxy.draft.url, error: error instanceof Error ? error.message : String(error)}); throw error}
  });
  return <section aria-label="Codex CLI 代理" className="space-y-4">
    <h3 className="text-sm font-medium">Codex CLI 代理</h3>
    {proxy.error && <p role="alert" className="text-xs text-danger break-words">{safeText(proxy.error)}</p>}
    {!proxy.state ? <Button size="sm" variant="ghost" isDisabled={!!busy} onPress={proxy.read}>读取 CLI 代理</Button> : <>
      <details className="text-xs text-muted">
        <summary className="cursor-pointer">配置文件</summary><p className="mt-2 break-all">{safeText(proxy.state.path)}</p>
      </details>
      {proxy.state.variables.length === 0 ? <p className="text-xs text-muted">文件中未配置 HTTP/HTTPS 代理</p> : <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-xs text-muted">
        {proxy.state.variables.map((variable, index) => <div key={index} className="contents"><dt>{variable.key}</dt><dd className="break-all">{privacy || variable.value.includes('@') ? '••••••••' : safeText(variable.value) || '空值'}</dd></div>)}
      </dl>}
      {proxy.draft.remove ? <p role="status" className="text-sm">保存后移除文件中的 HTTP/HTTPS 代理字段。</p> : <Field label="CLI 代理地址" type={privacy || proxy.draft.url.includes('@') ? 'password' : 'url'} value={proxy.draft.url} onChange={url => {proxy.change({url, remove: false}); setTested(null)}} disabled={!!busy} />}
      {!proxy.draft.remove && <p className="text-xs text-muted">保存时将 HTTP 与 HTTPS 统一设为此地址。含认证信息的地址使用遮罩。</p>}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" isDisabled={!!busy || deckNetwork.mode !== 'custom' || !deckNetwork.url.trim() || !URL.canParse(deckNetwork.url.trim())} onPress={reuse}>使用 Deck 代理地址</Button>
        <Button size="sm" variant="ghost" isDisabled={!!busy || proxy.draft.remove || !proxy.draft.url.trim()} onPress={test}>测试 CLI 代理</Button>
        <Button size="sm" variant="ghost" isDisabled={!!busy || proxy.draft.remove || proxy.state.variables.length === 0} onPress={() => {proxy.change({url: '', remove: true}); setTested(null)}}>移除代理配置</Button>
        <Button size="sm" variant="ghost" isDisabled={!!busy || proxy.dirty} onPress={proxy.read}>重新读取</Button>
        {proxy.dirty && <Button size="sm" variant="ghost" isDisabled={!!busy} onPress={() => {proxy.undo(); setTested(null)}}>撤销 CLI 更改</Button>}
      </div>
      {tested?.url === proxy.draft.url && !proxy.draft.remove && <p role="status" className={`text-xs ${tested.error ? 'text-danger' : 'text-muted'}`}>{tested.error ? safeText(tested.error) : '通过此代理访问 GitHub 成功，未验证 Codex CLI 连接。'}</p>}
      {proxy.applied && <p role="status" className="text-xs text-muted">CLI 代理配置已保存，重新启动 Codex CLI 生效。</p>}
    </>}
    <p className="text-xs text-muted">仅修改 CLI 环境文件，不参与 WebDAV 备份。移除后仍可能使用启动环境中的代理或其他代理变量。</p>
  </section>;
}
