import type {Account, State} from '../bridge/api';

export function createState(overrides: Partial<State> = {}): State {
  return {
    build_info: {version: '1.0.0', commit: '', development: true},
    update: {phase: '', error: '', version: '', notes: '', url: '', checked_at: null, downloaded: 0, total: 0},
    desktop_error: '',
    preferences: {privacy: false, sort: '', low_quota_notify: false, low_quota_threshold: 10},
    accounts: [],
    settings: {webdav_url: '', webdav_username: '', webdav_password: ''},
    cli_store: 'file',
    cli_error: '',
    backup: {at: null, error: '', busy: false, conflict: false},
    next_refresh_at: null,
    auto_refreshing: false,
    ...overrides,
  };
}

export function createAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 'account',
    account_id: 'workspace',
    email: 'account@example.test',
    alias: '',
    plan: '',
    current: false,
    auth_error: '',
    usage: null,
    credits: null,
    usage_error: '',
    credits_error: '',
    pending_reset: null,
    ...overrides,
  };
}
