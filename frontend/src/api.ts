export interface Settings {webdav_url: string; webdav_username: string; webdav_password: string}
export interface QuotaWindow {remaining_percent: number; reset_at: string | null}
export interface Usage {plan: string; five_hour: QuotaWindow | null; weekly: QuotaWindow | null; reset_credits: number | null; queried_at: string}
export interface Credit {id: string; reset_type: string; status: string; granted_at: string; expires_at: string | null; title: string | null; description: string | null}
export interface Credits {available_count: number; credits: Credit[] | null; queried_at: string}
export interface Account {id: string; account_id: string; email: string; alias: string; plan: string; current: boolean; auth_error: string; usage: Usage | null; credits: Credits | null; usage_error: string; credits_error: string; pending_reset: ResetRequest | null}
export interface State {accounts: Account[]; settings: Settings; cli_store: string; cli_error: string; backup: {at: string | null; error: string; busy: boolean; conflict: boolean}; next_refresh_at: string | null}
export interface RestorePreview {id: string; accounts: number; webdav_url: string; modified_at: string | null; added: PreviewAccount[]; removed: PreviewAccount[]}
export interface PreviewAccount {email: string; alias: string; account_id: string}
export interface ResetRequest {id: string; credit_id: string; started: boolean; result: ConsumeResult | null}
export interface ConsumeResult {code: string; windows_reset: number; refresh_error: string}
interface Backend {
  GetState(): Promise<State>;
  RefreshAll(): Promise<State>;
  RefreshAutomatic(): Promise<State>;
  CancelAutomatic(): Promise<void>;
  RefreshAccount(id: string): Promise<void>;
  RefreshCredits(id: string): Promise<void>;
  Rename(id: string, alias: string): Promise<void>;
  Delete(id: string): Promise<void>;
  Switch(id: string, enableFile: boolean): Promise<void>;
  BeginLogin(target: string): Promise<{id: string}>;
  WaitLogin(id: string): Promise<void>;
  CancelLogin(id: string): Promise<void>;
  SaveSettings(settings: Settings): Promise<void>;
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
