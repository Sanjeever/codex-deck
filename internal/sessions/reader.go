package sessions

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Protocol checked against openai/codex 9167a0b2d4a74d44a4d388057d304682e47313c6:
// rollout/src/recorder.rs, history/src/rollout_payload.rs, protocol/src/{models,items,protocol}.rs.
// Only plaintext JSONL is supported. References to inherited history are reported,
// never followed: their paths and retention semantics belong to Codex.
const maxLineBytes = 16 << 20
const textChunkRunes = 8192

type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sourceRef struct {
	offset int64
	size   int
}
type indexedEvent struct {
	Event
	sources []sourceRef
}
type document struct {
	summary Summary
	events  []indexedEvent
	issues  []Issue
	turns   []Turn
	pending bool
	matches []Match
}

func walkRecords(ctx context.Context, f *os.File, visit func(record, sourceRef, int) error, problem func(int, string)) (bool, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	var offset int64
	line := 0
	data := make([]byte, 0, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		start := offset
		data = data[:0]
		tooLarge := false
		var err error
		for {
			var part []byte
			part, err = reader.ReadSlice('\n')
			offset += int64(len(part))
			if len(data)+len(part) > maxLineBytes {
				tooLarge = true
			}
			if !tooLarge {
				data = append(data, part...)
			}
			if !errors.Is(err, bufio.ErrBufferFull) {
				break
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		if len(data) == 0 && !tooLarge && errors.Is(err, io.EOF) {
			return false, nil
		}
		line++
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		if tooLarge {
			problem(line, "记录超过 16 MiB 读取上限，该行未参与展示或搜索")
		} else if len(bytes.TrimSpace(data)) > 0 {
			var entry record
			decodeErr := json.Unmarshal(data, &entry)
			if decodeErr != nil {
				// An unfinished JSON value at EOF is a live writer's pending record.
				if errors.Is(err, io.EOF) && !bytes.HasSuffix(data, []byte{'\n'}) && strings.Contains(decodeErr.Error(), "unexpected end") {
					return true, nil
				}
				problem(line, "JSON 记录损坏")
			} else if entry.Type == "" || len(entry.Payload) == 0 || bytes.Equal(entry.Payload, []byte("null")) {
				problem(line, "记录缺少 type 或 payload")
			} else if visitErr := visit(entry, sourceRef{start, len(data)}, line); visitErr != nil {
				return false, visitErr
			}
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
	}
}

func readDocument(ctx context.Context, f *os.File, file string) (*document, error) {
	return readRollout(ctx, f, file, true, "")
}

func readSummary(ctx context.Context, f *os.File, file string) (*document, error) {
	return readRollout(ctx, f, file, false, "")
}

func readMatches(ctx context.Context, f *os.File, file, search string) (*document, error) {
	return readRollout(ctx, f, file, false, strings.TrimSpace(search))
}

type toolCall struct {
	index   int
	name    string
	sources int
	offset  int64
	turn    int
	text    string
}

type indexedMatch struct {
	Match
	offset int64
}

type messageIdentity struct {
	hash   [32]byte
	source string
}

type rolloutBuilder struct {
	d            *document
	file         string
	index        bool
	userItems    bool
	calls        map[string]toolCall
	turnIDs      map[string]int
	turn         int
	activeTurnID string
	lastMessages map[string]messageIdentity
	search       string
	matches      map[int64]Match
}

func newRolloutBuilder(file string, index, userItems bool, search string) *rolloutBuilder {
	return &rolloutBuilder{
		d:    &document{summary: Summary{Models: []string{}}, issues: []Issue{}, events: []indexedEvent{}, turns: []Turn{}},
		file: file, index: index, userItems: userItems,
		calls: map[string]toolCall{}, turnIDs: map[string]int{}, lastMessages: map[string]messageIdentity{},
		search: search, matches: map[int64]Match{},
	}
}

func (b *rolloutBuilder) issue(line int, message string) {
	b.d.issues = append(b.d.issues, Issue{File: b.file, Line: line, Message: message})
}

// Keep both user-message interpretations only until the first canonical user
// event. This avoids a second file scan and keeps instruction items out of chat.
func readRollout(ctx context.Context, f *os.File, file string, index bool, search string) (*document, error) {
	canonical := newRolloutBuilder(file, index, false, search)
	fallback := newRolloutBuilder(file, index, true, search)
	pending, err := walkRecords(ctx, f, func(r record, ref sourceRef, line int) error {
		p := object(r.Payload)
		if p == nil {
			canonical.issue(line, "payload 必须是对象")
			if fallback != nil {
				fallback.issue(line, "payload 必须是对象")
			}
			return nil
		}
		if r.Type == "event_msg" && (str(p, "type") == "user_message" || (str(p, "type") == "item_completed" && str(object(p["item"]), "type") == "UserMessage")) {
			fallback = nil
		}
		kind, name, text, callID, status, output := normalizePayload(r, p)
		canonical.consume(r, p, ref, line, kind, name, text, callID, status, output)
		if fallback != nil {
			fallback.consume(r, p, ref, line, kind, name, text, callID, status, output)
		}
		return nil
	}, func(line int, message string) {
		canonical.issue(line, message)
		if fallback != nil {
			fallback.issue(line, message)
		}
	})
	if err != nil {
		return nil, err
	}
	b := canonical
	if fallback != nil {
		b = fallback
	}
	b.d.pending = pending
	if pending {
		b.issue(0, "末尾记录尚未写完；已展示此前完整记录，刷新可重新读取")
	}
	if b.d.summary.ID == "" {
		return nil, fmt.Errorf("缺少可识别的 session_meta.id")
	}
	b.d.summary.TurnCount = len(b.d.turns)
	if !index {
		b.d.turns = nil
	}
	matches := make([]indexedMatch, 0, len(b.matches))
	for offset, match := range b.matches {
		matches = append(matches, indexedMatch{match, offset})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].offset < matches[j].offset })
	b.d.matches = make([]Match, 0, len(matches))
	for _, match := range matches {
		b.d.matches = append(b.d.matches, match.Match)
	}
	return b.d, nil
}

func (b *rolloutBuilder) match(offset int64, turn int, name, text string) {
	if b.search == "" {
		return
	}
	if _, exists := b.matches[offset]; exists {
		return
	}
	index := strings.Index(strings.ToLower(text), strings.ToLower(b.search))
	if index < 0 && !strings.Contains(strings.ToLower(name), strings.ToLower(b.search)) {
		return
	}
	textOffset := 0
	if index >= 0 {
		textOffset = len([]rune(strings.ToLower(text)[:index]))
	}
	b.matches[offset] = Match{EventID: "line-" + strconv.FormatInt(offset, 10), Turn: turn, Snippet: excerpt(text, b.search, 180), TextOffset: textOffset}
}

func (b *rolloutBuilder) consume(r record, p map[string]json.RawMessage, ref sourceRef, line int, kind, name, text, callID, status string, output bool) {
	d := b.d
	timestamp := validTime(r.Timestamp)
	if r.Timestamp != "" && timestamp == "" {
		b.issue(line, "记录时间格式不可识别")
	}
	if newer(timestamp, d.summary.UpdatedAt) {
		d.summary.UpdatedAt = timestamp
	}
	switch r.Type {
	case "session_meta":
		if d.summary.ID == "" {
			d.summary.ID = str(p, "id")
			d.summary.Project = str(p, "cwd")
			d.summary.CreatedAt = validTime(str(p, "timestamp"))
			if len(p["history_base"]) > 0 && string(p["history_base"]) != "null" {
				b.issue(line, "此会话引用继承历史；当前只展示本文件记录")
			}
		}
		return
	case "turn_context":
		if model := str(p, "model"); model != "" && !contains(d.summary.Models, model) {
			d.summary.Models = append(d.summary.Models, model)
		}
		b.activeTurnID = str(p, "turn_id")
		return
	}
	if kind == "" {
		return
	}
	if r.Type == "response_item" && str(p, "role") == "user" && !b.userItems {
		return
	}
	if (kind == "user" || kind == "assistant") && text == "" {
		b.issue(line, "消息未记录可展示文本")
	}
	if r.Type == "event_msg" {
		if id := str(p, "turn_id"); id != "" {
			b.activeTurnID = id
		}
	}
	if r.Type == "event_msg" && str(p, "type") == "item_completed" && kind == "tool" && callID != "" {
		if _, exists := b.calls[callID]; exists {
			output = true
		}
	}
	if kind == "user" || kind == "assistant" {
		hash := sha256.Sum256([]byte(text))
		last := b.lastMessages[kind]
		if last.hash == hash && last.source != r.Type {
			return
		}
		b.lastMessages[kind] = messageIdentity{hash, r.Type}
	}
	if output && callID != "" {
		call, ok := b.calls[callID]
		if ok && call.sources >= 2 && r.Type == "response_item" {
			b.issue(line, "同一工具调用记录了额外结果，保留为独立事件")
		} else if ok {
			if call.sources < 2 && b.search != "" {
				delete(b.matches, call.offset)
				b.match(call.offset, call.turn, call.name, call.text+"\n\n"+text)
			}
			if b.index {
				e := &d.events[call.index]
				if status == "unknown" {
					status = shellResultStatus(call.name, text)
				}
				if call.sources < 2 {
					e.sources = append(e.sources, ref)
				}
				if status != "unknown" {
					e.Status = status
				}
			}
			call.sources = 2
			call.text = ""
			b.calls[callID] = call
			return
		}
		if !ok {
			b.issue(line, "工具结果缺少对应调用，保留独立结果")
		}
	}
	if kind == "user" {
		if number, ok := b.turnIDs[b.activeTurnID]; b.activeTurnID != "" && ok {
			b.turn = number
		} else {
			b.turn = len(d.turns) + 1
			if b.activeTurnID != "" {
				b.turnIDs[b.activeTurnID] = b.turn
			}
		}
		if d.summary.Title == "" {
			d.summary.Title = excerpt(text, "", 120)
		}
		b.lastMessages["assistant"] = messageIdentity{}
	}
	id := "line-" + strconv.FormatInt(ref.offset, 10)
	if b.turn > 0 && (len(d.turns) == 0 || d.turns[len(d.turns)-1].Number != b.turn) {
		d.turns = append(d.turns, Turn{Number: b.turn, EventID: id})
	}
	if kind == "tool" && !output {
		d.summary.ToolCount++
		if callID != "" {
			call := toolCall{index: len(d.events), name: name, sources: 1, offset: ref.offset, turn: b.turn}
			if b.search != "" {
				call.text = text
			}
			b.calls[callID] = call
		}
	}
	if kind == "unknown" {
		b.issue(line, "未支持的事件："+name)
	}
	b.match(ref.offset, b.turn, name, text)
	if b.index {
		d.events = append(d.events, indexedEvent{Event: Event{ID: id, Turn: b.turn, Kind: kind, Time: timestamp, Name: name, Status: status}, sources: []sourceRef{ref}})
	}
}

func object(raw json.RawMessage) map[string]json.RawMessage {
	var p map[string]json.RawMessage
	_ = json.Unmarshal(raw, &p)
	return p
}
func str(p map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(p[key], &s)
	return s
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func validTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func contentText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return string(raw)
	}
	parts := []string{}
	for _, item := range items {
		if text := str(item, "text"); text != "" {
			parts = append(parts, text)
		} else {
			parts = append(parts, "[非文本内容："+str(item, "type")+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// normalize extracts only persisted text. Encrypted reasoning and model-only
// instructions are never presented as a user request or invisible reasoning.
func normalize(r record) (kind, name, text, callID, status string, output bool) {
	return normalizePayload(r, object(r.Payload))
}

func normalizePayload(r record, p map[string]json.RawMessage) (kind, name, text, callID, status string, output bool) {
	typ := str(p, "type")
	status = "unknown"
	if r.Type == "compacted" {
		return "notice", "上下文压缩", str(p, "message"), "", "", false
	}
	if r.Type == "response_item" {
		switch typ {
		case "message":
			role := str(p, "role")
			if role != "user" && role != "assistant" {
				return
			}
			return role, "", contentText(p["content"]), "", "", false
		case "reasoning":
			text = contentText(p["summary"])
			if text == "" {
				return
			}
			return "notice", "已记录的推理摘要", text, "", "", false
		case "function_call", "custom_tool_call", "local_shell_call":
			text = str(p, "arguments")
			if typ == "custom_tool_call" {
				text = str(p, "input")
			}
			if typ == "local_shell_call" {
				text = string(p["action"])
			}
			return "tool", str(p, "name"), text, str(p, "call_id"), status, false
		case "function_call_output", "custom_tool_call_output":
			text = contentText(p["output"])
			return "tool", str(p, "name"), text, str(p, "call_id"), resultStatus(text), true
		case "compaction", "context_compaction":
			return "notice", "上下文压缩", "记录包含压缩标记", "", "", false
		}
	}
	if r.Type == "event_msg" {
		switch typ {
		case "user_message":
			return "user", "", str(p, "message"), "", "", false
		case "agent_message":
			return "assistant", "", str(p, "message"), "", "", false
		case "agent_reasoning":
			return "notice", "已记录的推理摘要", str(p, "text"), "", "", false
		case "agent_reasoning_raw_content", "token_count", "task_started", "task_complete", "turn_started", "turn_complete", "context_compacted":
			return
		case "turn_aborted":
			return "notice", "轮次中断", str(p, "reason"), "", "error", false
		case "error", "warning":
			return "notice", typ, str(p, "message"), "", "error", false
		case "exec_command_end":
			return "tool", "执行结果", str(p, "aggregated_output"), str(p, "call_id"), exitStatus(p["exit_code"]), true
		case "patch_apply_end":
			var success *bool
			_ = json.Unmarshal(p["success"], &success)
			if success != nil {
				if *success {
					status = "success"
				} else {
					status = "error"
				}
			}
			return "tool", "apply_patch", str(p, "stdout") + str(p, "stderr"), str(p, "call_id"), status, true
		case "mcp_tool_call_end":
			result := object(p["result"])
			return "tool", "MCP 结果", string(p["result"]), str(p, "call_id"), resultStatus(string(result["Ok"])), true
		case "item_completed":
			item := object(p["item"])
			name = str(item, "type")
			callID = str(item, "id")
			switch name {
			case "UserMessage":
				return "user", "", contentText(item["content"]), "", "", false
			case "AgentMessage":
				return "assistant", "", contentText(item["content"]), "", "", false
			case "CommandExecution":
				return "tool", "执行命令", contentText(item["command"]) + "\n\n" + str(item, "aggregated_output"), callID, exitStatus(item["exit_code"]), false
			case "FunctionCallOutput":
				return "tool", str(item, "name"), contentText(item["output"]), callID, resultStatus(contentText(item["output"])), true
			case "Plan":
				return "notice", "计划", string(p["item"]), "", "", false
			}
		}
	}
	return "unknown", r.Type + "/" + typ, string(r.Payload), "", "unknown", false
}

func exitStatus(raw json.RawMessage) string {
	var code *int
	if json.Unmarshal(raw, &code) != nil || code == nil {
		return "unknown"
	}
	if *code == 0 {
		return "success"
	}
	return "error"
}
func resultStatus(text string) string {
	// Shell tools persist a JSON wrapper with metadata.exit_code; generic tool
	// output strings have no reliable success flag and remain unknown.
	p := object(json.RawMessage(text))
	if metadata := object(p["metadata"]); metadata != nil {
		return exitStatus(metadata["exit_code"])
	}
	var failed *bool
	_ = json.Unmarshal(p["isError"], &failed)
	if failed != nil {
		if *failed {
			return "error"
		}
		return "success"
	}
	return "unknown"
}

func shellResultStatus(name, text string) string {
	if name != "exec_command" && name != "write_stdin" {
		return "unknown"
	}
	// This is Codex's response_header in core/src/tools/context.rs, not a
	// heuristic over command stdout. Never inspect anything after Output:.
	header, _, found := strings.Cut(text, "\nOutput:\n")
	if !found || !strings.Contains(header, "Wall time: ") {
		return "unknown"
	}
	for _, line := range strings.Split(header, "\n") {
		if value, ok := strings.CutPrefix(line, "Process exited with code "); ok {
			code, err := strconv.Atoi(value)
			if err != nil {
				return "unknown"
			}
			if code == 0 {
				return "success"
			}
			return "error"
		}
	}
	return "unknown"
}

func eventText(f *os.File, e indexedEvent) (string, error) {
	parts := make([]string, 0, len(e.sources))
	for _, ref := range e.sources {
		data := make([]byte, ref.size)
		if _, err := f.ReadAt(data, ref.offset); err != nil {
			return "", err
		}
		var r record
		if err := json.Unmarshal(data, &r); err != nil {
			return "", fmt.Errorf("记录已变化，请刷新会话")
		}
		_, _, text, _, _, _ := normalize(r)
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n"), nil
}

func excerpt(text, search string, limit int) string {
	runes := []rune(text)
	start := 0
	if search != "" {
		lower := strings.ToLower(text)
		if index := strings.Index(lower, strings.ToLower(search)); index >= 0 {
			start = len([]rune(lower[:index])) - 40
			if start < 0 {
				start = 0
			}
		}
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
	}
	value := string(runes[start:end])
	if start > 0 {
		value = "…" + value
	}
	if end < len(runes) {
		value += "…"
	}
	return value
}
