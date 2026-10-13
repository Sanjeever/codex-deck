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
}

func walkRecords(ctx context.Context, f *os.File, visit func(record, sourceRef, int) error, problem func(int, string)) (bool, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	var offset int64
	line := 0
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		start := offset
		data := make([]byte, 0, 4096)
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
	d := &document{summary: Summary{Models: []string{}}, issues: []Issue{}, events: []indexedEvent{}, turns: []Turn{}}
	issue := func(line int, message string) {
		d.issues = append(d.issues, Issue{File: file, Line: line, Message: message})
	}
	// Canonical user events omit the model-visible environment and instruction messages.
	hasUserEvents := false
	_, err := walkRecords(ctx, f, func(r record, _ sourceRef, _ int) error {
		p := object(r.Payload)
		if r.Type == "event_msg" && (str(p, "type") == "user_message" || (str(p, "type") == "item_completed" && str(object(p["item"]), "type") == "UserMessage")) {
			hasUserEvents = true
		}
		return nil
	}, func(int, string) {})
	if err != nil {
		return nil, err
	}
	calls := map[string]int{}
	turnIDs := map[string]int{}
	turn := 0
	activeTurnID := ""
	lastMessages := map[string]struct {
		hash   [32]byte
		source string
	}{}
	pending, err := walkRecords(ctx, f, func(r record, ref sourceRef, line int) error {
		p := object(r.Payload)
		if p == nil {
			issue(line, "payload 必须是对象")
			return nil
		}
		timestamp := validTime(r.Timestamp)
		if r.Timestamp != "" && timestamp == "" {
			issue(line, "记录时间格式不可识别")
		}
		if timestamp > d.summary.UpdatedAt {
			d.summary.UpdatedAt = timestamp
		}
		switch r.Type {
		case "session_meta":
			if d.summary.ID == "" {
				d.summary.ID = str(p, "id")
				d.summary.Project = str(p, "cwd")
				d.summary.CreatedAt = validTime(str(p, "timestamp"))
				if len(p["history_base"]) > 0 && string(p["history_base"]) != "null" {
					issue(line, "此会话引用继承历史；当前只展示本文件记录")
				}
			}
			return nil
		case "turn_context":
			if model := str(p, "model"); model != "" && !contains(d.summary.Models, model) {
				d.summary.Models = append(d.summary.Models, model)
			}
			activeTurnID = str(p, "turn_id")
			return nil
		}
		kind, name, text, callID, status, output := normalize(r)
		if kind == "" {
			return nil
		}
		if r.Type == "response_item" && str(p, "role") == "user" && hasUserEvents {
			return nil
		}
		if r.Type == "event_msg" {
			if id := str(p, "turn_id"); id != "" {
				activeTurnID = id
			}
		}
		if kind == "user" || kind == "assistant" {
			hash := sha256.Sum256([]byte(text))
			last := lastMessages[kind]
			if last.hash == hash && last.source != r.Type {
				return nil
			}
			lastMessages[kind] = struct {
				hash   [32]byte
				source string
			}{hash, r.Type}
		}
		if output && callID != "" {
			if index, ok := calls[callID]; ok {
				e := &d.events[index]
				e.sources = append(e.sources, ref)
				if status != "unknown" {
					e.Status = status
				}
				delete(calls, callID)
				return nil
			}
			issue(line, "工具结果缺少对应调用，保留独立结果")
		}
		if kind == "user" {
			if number, ok := turnIDs[activeTurnID]; activeTurnID != "" && ok {
				turn = number
			} else {
				turn = len(d.turns) + 1
				if activeTurnID != "" {
					turnIDs[activeTurnID] = turn
				}
			}
			if d.summary.Title == "" {
				d.summary.Title = excerpt(text, "", 120)
			}
			lastMessages["assistant"] = struct {
				hash   [32]byte
				source string
			}{}
		}
		id := "line-" + strconv.FormatInt(ref.offset, 10)
		if turn > 0 && (len(d.turns) == 0 || d.turns[len(d.turns)-1].Number != turn) {
			d.turns = append(d.turns, Turn{Number: turn, EventID: id})
		}
		e := indexedEvent{Event: Event{ID: id, Turn: turn, Kind: kind, Time: timestamp, Name: name, Status: status}, sources: []sourceRef{ref}}
		if kind == "tool" && !output {
			d.summary.ToolCount++
			if callID != "" {
				calls[callID] = len(d.events)
			}
		}
		if kind == "unknown" {
			issue(line, "未支持的事件："+name)
		}
		d.events = append(d.events, e)
		return nil
	}, issue)
	if err != nil {
		return nil, err
	}
	d.pending = pending
	if pending {
		issue(0, "末尾记录尚未写完；已展示此前完整记录，刷新可重新读取")
	}
	if d.summary.ID == "" {
		return nil, fmt.Errorf("缺少可识别的 session_meta.id")
	}
	d.summary.TurnCount = len(d.turns)
	return d, nil
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
	p := object(r.Payload)
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
		if index := strings.Index(strings.ToLower(text), strings.ToLower(search)); index >= 0 {
			start = len([]rune(text[:index])) - 40
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
