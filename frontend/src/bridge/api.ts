export interface Preferences {privacy: boolean; sort: string; low_quota_notify: boolean; low_quota_threshold: number; disable_update_check?: boolean}
export interface BuildInfo {version: string; commit: string; development: boolean}
export interface UpdateState {phase: string; error: string; version: string; notes: string; url: string; checked_at: string | null; downloaded: number; total: number}
export interface Settings {webdav_url: string; webdav_username: string; webdav_password: string}
export interface NetworkConfig {mode: 'environment' | 'direct' | 'custom'; url: string; username: string; password: string}
export interface CLIProxyVariable {key: string; value: string}
export interface CLIProxyState {path: string; variables: CLIProxyVariable[]}
export interface CLIProxyChange {url: string; remove: boolean; expected: CLIProxyVariable[]}
export interface Configuration {network: NetworkConfig; settings: Settings; preferences: {low_quota_notify: boolean; low_quota_threshold: number; disable_update_check: boolean}}
export interface ConfigurationSaveResult {configuration: Configuration | null; section: string; error: string}
export interface QuotaWindow {remaining_percent: number; reset_at: string | null}
export interface Usage {plan: string; five_hour: QuotaWindow | null; weekly: QuotaWindow | null; reset_credits: number | null; queried_at: string}
export interface Credit {id: string; reset_type: string; status: string; granted_at: string; expires_at: string | null; title: string | null; description: string | null}
export interface Credits {available_count: number; credits: Credit[] | null; queried_at: string}
export interface Account {id: string; account_id: string; email: string; alias: string; plan: string; current: boolean; auth_error: string; usage: Usage | null; credits: Credits | null; usage_error: string; credits_error: string; pending_reset: ResetRequest | null}
export interface State {network: NetworkConfig; build_info: BuildInfo; update: UpdateState; desktop_error: string; preferences: Preferences; accounts: Account[]; settings: Settings; cli_store: string; cli_error: string; backup: {at: string | null; error: string; busy: boolean; conflict: boolean}; next_refresh_at: string | null; auto_refreshing: boolean}
export interface RestorePreview {id: string; accounts: number; webdav_url: string; modified_at: string | null; added: PreviewAccount[]; removed: PreviewAccount[]; removes_current: boolean}
export interface PreviewAccount {email: string; alias: string; account_id: string}
export interface AuthImportPreview extends PreviewAccount {id: string; existing: boolean; unchanged: boolean}
export interface ResetRequest {id: string; credit_id: string; started: boolean; result: ConsumeResult | null}
export interface ConsumeResult {code: string; windows_reset: number; refresh_error: string}
interface Backend {
  ListSessions(query: SessionListQuery): Promise<SessionListPage>;
  CancelSessionRequest(scope: 'list' | 'detail' | 'find', requestID: number): Promise<void>;
  ReadSession(query: SessionReadQuery): Promise<SessionReadPage>;
  FindSessionMatches(query: {id: string; search: string; cursor: string; request_id?: number}): Promise<SessionMatchPage>;
  ReadSessionText(query: {id: string; event_id: string; revision: string; offset: number}): Promise<SessionTextPage>;
  SaveConfiguration(config: Configuration): Promise<ConfigurationSaveResult>;
  SaveNetwork(config: NetworkConfig): Promise<NetworkConfig>;
  TestNetwork(config: NetworkConfig): Promise<void>;
  GetCLIProxy(): Promise<CLIProxyState>;
  SaveCLIProxy(change: CLIProxyChange): Promise<CLIProxyState>;
  TestCLIProxy(address: string): Promise<void>;
  ImportAuth(): Promise<AuthImportPreview | null>;
  ConfirmAuthImport(id: string): Promise<void>;
  CancelAuthImport(): Promise<void>;
  ExportAuth(id: string): Promise<boolean>;
  CheckUpdate(): Promise<void>;
  InstallUpdate(): Promise<void>;
  CancelUpdate(): Promise<void>;
  GetAutoStart(): Promise<boolean>;
  SetAutoStart(enabled: boolean): Promise<boolean>;
  GetState(): Promise<State>;
  RefreshAll(): Promise<State>;
  SetAutomaticPaused(paused: boolean): Promise<void>;
  WakeAutomatic(): Promise<void>;
  RefreshAccount(id: string): Promise<void>;
  RefreshCredits(id: string): Promise<void>;
  Rename(id: string, alias: string): Promise<void>;
  Delete(id: string): Promise<void>;
  Switch(id: string, enableFile: boolean): Promise<void>;
  BeginLogin(target: string): Promise<{id: string}>;
  WaitLogin(id: string): Promise<void>;
  CancelLogin(id: string): Promise<void>;
  SaveSettings(settings: Settings): Promise<void>;
  SavePreferences(preferences: Preferences): Promise<void>;
  Backup(): Promise<void>;
  OverwriteBackup(): Promise<void>;
  PrepareRestore(): Promise<RestorePreview>;
  CancelRestore(): Promise<void>;
  Restore(id: string): Promise<void>;
  PrepareConsume(id: string, creditID: string): Promise<ResetRequest>;
  Consume(id: string, requestID: string): Promise<ConsumeResult>;
}
declare global {interface Window {runtime?: {EventsOnMultiple(event: string, callback: (state: State) => void, count: number): () => void}; go: {main: {App: Backend}}}}
export function backend(): Backend {
  if (!window.go?.main?.App) throw new Error('请通过 Wails 启动程序');
  return window.go.main.App;
}

export interface SessionIssue {file: string; line?: number; message: string}
export interface SessionSummary {id: string; title: string; project: string; created_at?: string; updated_at?: string; models: string[]; turn_count: number; tool_count: number; snippet?: string; match_event_id?: string; match_text_offset?: number}
export interface SessionEvent {id: string; turn: number; kind: string; time?: string; name?: string; status?: string; text: string; text_cursor: number}
export interface SessionTurn {number: number; event_id: string}
export interface SessionListQuery {search: string; project: string; date: string; cursor: string; request_id?: number}
export interface SessionListPage {sessions: SessionSummary[]; projects: string[]; dates: string[]; source: string; missing: boolean; next_cursor: string; issues: SessionIssue[]}
export interface SessionReadQuery {id: string; cursor: string; event_id: string; filter: string; request_id?: number}
export interface SessionReadPage {session: SessionSummary; revision: string; turns: SessionTurn[]; events: SessionEvent[]; next_cursor: string; issues: SessionIssue[]}
export interface SessionMatch {event_id: string; turn: number; snippet: string; text_offset: number}
export interface SessionMatchPage {matches: SessionMatch[]; next_cursor: string; issues: SessionIssue[]}
export interface SessionTextPage {text: string; next_offset: number}
