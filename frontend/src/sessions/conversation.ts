import {backend} from '../bridge/api';

// Read independently of the displayed filter, event pages and text fragments.
export async function readConversation(id: string, revision: string, turn: number | null, eventID: string, current: () => boolean) {
  const parts: string[] = [];
  let cursor = '';
  let lastTurn = -1;
  do {
    const page = await backend().ReadSession({id, filter: 'chat', cursor, event_id: cursor ? '' : eventID});
    if (!current()) return null;
    if (page.revision !== revision) throw new Error('会话已变化，请刷新后复制');
    if (!parts.length) {
      if (turn === null) parts.push(`# ${page.session.title || '会话对话'}`);
      for (const issue of page.issues) parts.push(`> 读取提示：${issue.file}${issue.line ? `:${issue.line}` : ''} · ${issue.message}`);
    }
    for (const event of page.events) {
      if (turn !== null && event.turn !== turn) return parts.join('\n\n');
      if (event.turn !== lastTurn) {
        parts.push(`## ${event.turn ? `第 ${event.turn} 轮` : '轮次前记录'}`);
        lastTurn = event.turn;
      }
      let text = event.text;
      let offset = event.text_cursor;
      while (offset > 0) {
        const fragment = await backend().ReadSessionText({id, event_id: event.id, revision, offset});
        if (!current()) return null;
        text += fragment.text;
        offset = fragment.next_offset;
      }
      parts.push(`### ${event.kind === 'user' ? '你' : 'Codex'}\n\n${text}`);
    }
    cursor = page.next_cursor;
  } while (cursor);
  return parts.join('\n\n');
}
