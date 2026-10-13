package sessions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func row(typ string, payload any) string {
	data, _ := json.Marshal(map[string]any{"timestamp": "2026-10-13T08:00:00Z", "type": typ, "payload": payload})
	return string(data) + "\n"
}
func meta(id, project string) string {
	return row("session_meta", map[string]any{"id": id, "cwd": project, "timestamp": "2026-10-13T07:00:00Z"})
}
func user(text string) string {
	return row("event_msg", map[string]any{"type": "user_message", "message": text})
}
func response(typ string, fields map[string]any) string {
	fields["type"] = typ
	return row("response_item", fields)
}
func fixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func parseFixture(t *testing.T, data string) (*document, *os.File) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout-test.jsonl")
	fixture(t, path, data)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	d, err := readDocument(context.Background(), f, "rollout-test.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return d, f
}

func TestConversationAndTools(t *testing.T) {
	data := meta("session", "D:/project") + response("message", map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "environment instructions"}}}) +
		row("turn_context", map[string]any{"turn_id": "t1", "model": "actual-model"}) + user("修复测试") +
		response("function_call", map[string]any{"name": "shell", "call_id": "c1", "arguments": "{\"command\":\"go test\"}"}) +
		response("function_call_output", map[string]any{"call_id": "c1", "output": "{\"output\":\"FAIL\",\"metadata\":{\"exit_code\":1}}"}) +
		response("message", map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "已修复"}}}) +
		row("event_msg", map[string]any{"type": "agent_message", "message": "已修复"}) + user("再检查") +
		response("custom_tool_call", map[string]any{"name": "apply_patch", "call_id": "c2", "input": "patch"}) +
		response("custom_tool_call_output", map[string]any{"call_id": "c2", "output": "done"})
	d, f := parseFixture(t, data)
	if d.summary.Title != "修复测试" || d.summary.TurnCount != 1 || d.summary.ToolCount != 2 || len(d.events) != 5 {
		t.Fatalf("unexpected summary/events: %+v, %+v", d.summary, d.events)
	}
	if d.events[1].Status != "error" || d.events[4].Status != "unknown" {
		t.Fatalf("wrong statuses: %+v", d.events)
	}
	text, err := eventText(f, d.events[1])
	if err != nil || !strings.Contains(text, "go test") || !strings.Contains(text, "FAIL") {
		t.Fatalf("missing associated output: %q %v", text, err)
	}
	if len(d.issues) != 0 {
		t.Fatalf("unexpected issues: %+v", d.issues)
	}
}

func TestRepeatedUserRequestsRemainSeparateTurns(t *testing.T) {
	d, _ := parseFixture(t, meta("s", "/project")+user("继续")+user("继续"))
	if len(d.events) != 2 || d.summary.TurnCount != 2 {
		t.Fatalf("repeated requests lost: %+v", d)
	}
}

func TestPartialAndDamagedRecords(t *testing.T) {
	d, _ := parseFixture(t, meta("s", "/project")+"{broken}\n"+user("可读")+`{"type":"response_item","payload":`)
	if !d.pending || len(d.events) != 1 || len(d.issues) != 2 || d.issues[0].Line != 2 {
		t.Fatalf("incorrect partial issues: %+v", d)
	}
	d, _ = parseFixture(t, meta("s", "/project")+`{"type":"response_item","payload":`+"\n")
	if d.pending || len(d.issues) != 1 {
		t.Fatalf("terminated damage treated as live tail: %+v", d)
	}
}

func TestUnknownEncryptedAndCompactionRecords(t *testing.T) {
	d, f := parseFixture(t, meta("s", "/p")+user("test")+response("reasoning", map[string]any{"summary": []any{}, "encrypted_content": "secret"})+row("compacted", map[string]any{"message": "实际压缩摘要"})+row("future_record", map[string]any{"value": "visible"}))
	if len(d.events) != 3 || d.events[1].Kind != "notice" || d.events[2].Kind != "unknown" || len(d.issues) != 1 {
		t.Fatalf("unknown/compaction lost: %+v", d)
	}
	text, _ := eventText(f, d.events[1])
	if text != "实际压缩摘要" {
		t.Fatal(text)
	}
}

func TestPaginatedTurnItems(t *testing.T) {
	data := meta("s", "/p") + row("event_msg", map[string]any{"type": "item_completed", "turn_id": "t", "item": map[string]any{"type": "UserMessage", "id": "u", "content": []any{map[string]any{"type": "text", "text": "request"}}}}) + row("event_msg", map[string]any{"type": "item_completed", "turn_id": "t", "item": map[string]any{"type": "CommandExecution", "id": "c", "command": []string{"go", "test"}, "aggregated_output": "failed", "exit_code": 2}})
	d, _ := parseFixture(t, data)
	if d.summary.TurnCount != 1 || d.events[1].Status != "error" {
		t.Fatalf("turn items: %+v", d)
	}
}

func TestReadLimitAndCancellation(t *testing.T) {
	d, _ := parseFixture(t, meta("s", "/p")+strings.Repeat("x", maxLineBytes+1)+"\n"+user("after limit"))
	if len(d.events) != 1 || len(d.issues) != 1 || !strings.Contains(d.issues[0].Message, "读取上限") {
		t.Fatalf("limit hidden: %+v", d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f, err := os.Open(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		f.Close()
		t.Fatal("unexpected file")
	}
	_, file := parseFixture(t, meta("s", "/p"))
	if _, err := readDocument(ctx, file, "test"); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestUnifiedExecStatusUsesRecordedHeaderOnly(t *testing.T) {
	data := meta("s", "/p") + user("request") + response("function_call", map[string]any{"name": "exec_command", "call_id": "c", "arguments": "{}"}) + response("function_call_output", map[string]any{"call_id": "c", "output": "Chunk ID: abc\nWall time: 1.25 seconds\nProcess exited with code 2\nOutput:\nfailed"})
	d, _ := parseFixture(t, data)
	if d.events[1].Status != "error" {
		t.Fatal(d.events[1].Status)
	}
	if shellResultStatus("exec_command", "Wall time: 1 seconds\nOutput:\nProcess exited with code 1") != "unknown" {
		t.Fatal("stdout inferred as status")
	}
	if shellResultStatus("unrecognized", "Wall time: 1 seconds\nProcess exited with code 0\nOutput:\ntext") != "unknown" {
		t.Fatal("unknown tool inferred as success")
	}
}
