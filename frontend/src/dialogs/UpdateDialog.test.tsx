import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {Modal} from '@heroui/react';
import {afterEach, expect, it, vi} from 'vitest';
import {createState} from '../test/fixtures';
import type {RunOperation} from '../app/useDeckController';
import {UpdateDialog} from './UpdateDialog';

afterEach(cleanup);

function showUpdate(notes: string) {
  const state = createState({build_info: {version: '1.3.2', commit: '', development: false}});
  state.update = {...state.update, version: '1.4.0', notes, url: 'https://github.com/Sanjeever/codex-deck/releases/tag/v1.4.0'};
  const install = vi.fn(async () => {});
  vi.stubGlobal('go', {main: {App: {InstallUpdate: install}}});
  const run: RunOperation = async (_label, action) => {await action()};
  const view = render(<Modal.Backdrop isOpen><Modal.Container><UpdateDialog state={state} busy="" run={run} onClose={async () => {}} setDialog={vi.fn()} /></Modal.Container></Modal.Backdrop>);
  return {...view, install};
}

it('默认显示 Markdown 标题、列表、强调和外部链接，安装仍需点击确认', async () => {
  const {install} = showUpdate('## 新增\n\n- 支持 **托盘额度**。\n- [发布说明](https://github.com/Sanjeever/codex-deck/releases/tag/v1.4.0)\n\n## 修复\n\n1. 修复窗口调度。\n');
  await screen.findByRole('heading', {name: '新增'});
  const notes = within(screen.getByRole('region', {name: '更新日志'}));
  expect(notes.getByRole('heading', {name: '新增'})).toBeTruthy();
  expect(notes.getAllByRole('listitem')).toHaveLength(3);
  expect(notes.getByText('托盘额度').tagName).toBe('STRONG');
  const link = notes.getByRole('link', {name: '发布说明'});
  expect(link.getAttribute('target')).toBe('_blank');
  expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  expect(screen.getByRole('region', {name: '更新日志'}).closest('details')).toBeNull();
  expect(install).not.toHaveBeenCalled();
  await act(async () => {fireEvent.click(screen.getByRole('button', {name: '下载并安装'}))});
  expect(install).toHaveBeenCalledTimes(1);
});

it.each(['', ' \n\t'])('空正文显示未提供日志，保留发布入口与安装操作', notes => {
  showUpdate(notes);
  expect(screen.getByText('此版本未提供更新日志')).toBeTruthy();
  expect(screen.getByRole('link', {name: '查看发布说明与手动下载'}).getAttribute('href')).toContain('/releases/tag/v1.4.0');
  expect((screen.getByRole('button', {name: '下载并安装'}) as HTMLButtonElement).disabled).toBe(false);
});

it('忽略原始 HTML，Markdown 链接不保留 javascript 协议', async () => {
  showUpdate('## 更新\n\n安全正文\n\n<script>alert(1)</script>\n\n<img src="https://example.test/secret" onerror="alert(1)">\n\n[危险链接](javascript:alert%281%29)');
  await screen.findByText('安全正文');
  const notes = screen.getByRole('region', {name: '更新日志'});
  expect(within(notes).getByText('安全正文')).toBeTruthy();
  expect(notes.querySelector('script')).toBeNull();
  expect(notes.querySelector('img')).toBeNull();
  expect(notes.querySelector('a')?.getAttribute('href')).not.toContain('javascript:');
});
