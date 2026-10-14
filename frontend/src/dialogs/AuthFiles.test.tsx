import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {toast} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import App from '../app/App';
import {createAccount, createState} from '../test/fixtures';

afterEach(() => {cleanup(); toast.clear(); vi.unstubAllGlobals()});

async function press(name: string, role: 'button' | 'menuitem' = 'button') {
  await act(async () => {fireEvent.click(screen.getByRole(role, {name}))});
}

async function chooseImport() {await press('添加账号方式'); await press('从 auth.json 导入', 'menuitem')}

it.each([false, true])('导入新账号直接更新列表，取消文件选择保留原列表，取消=%s', async canceled => {
  let state = createState();
  const preview = {id: 'import-preview', email: 'import@example.test', account_id: 'workspace', alias: '', existing: false};
  const ImportAuth = vi.fn(async () => {
    if (canceled) return null;
    state = {...state, accounts: [createAccount({email: preview.email})]};
    return preview;
  });
  const ConfirmAuthImport = vi.fn();
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), ImportAuth, ConfirmAuthImport}}});
  render(<App />); await act(async () => {});
  await chooseImport();
  expect(ImportAuth).toHaveBeenCalledTimes(1);
  expect(ConfirmAuthImport).not.toHaveBeenCalled();
  expect(!!screen.queryByText(preview.email)).toBe(!canceled);
  expect(screen.queryByRole('alertdialog')).toBeNull();
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('相同凭据导入直接结束，不要求确认或显示成功通知', async () => {
  const account = createAccount(); const state = createState({accounts: [account]});
  const preview = {id: 'unchanged-preview', email: account.email, account_id: account.account_id, alias: '', existing: true, unchanged: true};
  const ConfirmAuthImport = vi.fn();
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), ImportAuth: vi.fn(async () => preview), ConfirmAuthImport}}});
  render(<App />); await act(async () => {});
  await chooseImport();
  expect(ConfirmAuthImport).not.toHaveBeenCalled();
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.queryByText('账号凭据已更新')).toBeNull();
  expect(screen.getByText(account.email)).toBeTruthy();
});

it('重复账号需要确认，失败保留确认界面，隐私模式隐藏身份', async () => {
  const account = createAccount({email: 'private@example.test', account_id: 'workspace-secret'});
  const state = createState({accounts: [account]}); state.preferences.privacy = true;
  const preview = {id: 'private-preview', email: account.email, account_id: account.account_id, alias: '', existing: true};
  const ConfirmAuthImport = vi.fn().mockRejectedValueOnce(new Error('导入凭据早于已保存凭据')).mockResolvedValue(undefined);
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), ImportAuth: vi.fn(async () => preview), ConfirmAuthImport}}});
  render(<App />); await act(async () => {});
  await chooseImport();
  expect(screen.getByRole('heading', {name: '更新账号凭据'})).toBeTruthy();
  expect(ConfirmAuthImport).not.toHaveBeenCalled();
  expect(document.body.innerHTML).not.toContain(account.email);
  expect(document.body.innerHTML).not.toContain(account.account_id);
  await press('确认更新');
  expect(screen.getByRole('dialog')).toBeTruthy();
  expect(screen.getByText('导入凭据早于已保存凭据')).toBeTruthy();
  await press('确认更新');
  expect(ConfirmAuthImport).toHaveBeenLastCalledWith(preview.id);
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.queryByText('账号凭据已更新')).toBeNull();
});

it('关闭重复账号确认时清理待导入凭据', async () => {
  const state = createState();
  const preview = {id: 'preview', email: 'import@example.test', account_id: 'workspace', alias: '', existing: true};
  const CancelAuthImport = vi.fn(async () => {}); const ConfirmAuthImport = vi.fn();
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), ImportAuth: vi.fn(async () => preview), CancelAuthImport, ConfirmAuthImport}}});
  render(<App />); await act(async () => {});
  await chooseImport(); await press('取消');
  expect(CancelAuthImport).toHaveBeenCalledTimes(1);
  expect(ConfirmAuthImport).not.toHaveBeenCalled();
  expect(screen.queryByRole('dialog')).toBeNull();
});

it('导出先提示凭据风险，保存取消与失败保留对话框，成功后关闭', async () => {
  const account = createAccount(); const state = createState({accounts: [account]});
  const ExportAuth = vi.fn().mockResolvedValueOnce(false).mockRejectedValueOnce(new Error('目录不可写')).mockResolvedValue(true);
  vi.stubGlobal('go', {main: {App: {SetAutomaticPaused: vi.fn(async () => {}), GetState: vi.fn(async () => state), ExportAuth}}});
  render(<App />); await act(async () => {});
  await press('account@example.test 操作'); await press('导出 auth.json', 'menuitem');
  expect(screen.getByText('文件包含登录凭据，请勿公开分享。')).toBeTruthy();
  expect(ExportAuth).not.toHaveBeenCalled();
  await press('选择保存位置');
  expect(screen.getByRole('dialog')).toBeTruthy(); expect(screen.queryByText('账号已导出')).toBeNull();
  await press('选择保存位置');
  expect(screen.getByRole('dialog')).toBeTruthy(); expect(screen.getByText('目录不可写')).toBeTruthy();
  await press('选择保存位置');
  expect(ExportAuth).toHaveBeenLastCalledWith(account.id);
  expect(screen.queryByRole('dialog')).toBeNull(); expect(screen.queryByText('账号已导出')).toBeNull();
});
