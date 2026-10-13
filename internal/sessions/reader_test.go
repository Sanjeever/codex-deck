package sessions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestRawAndCompletedToolAreShownOnce(t *testing.T) {
	data := meta("s", "/p") + user("request") + response("function_call", map[string]any{"name": "exec_command", "call_id": "c", "arguments": "{}"}) + response("function_call_output", map[string]any{"call_id": "c", "output": "Wall time: 1 seconds\nProcess exited with code 1\nOutput:\nfailed"}) + row("event_msg", map[string]any{"type": "item_completed", "turn_id": "t", "item": map[string]any{"type": "CommandExecution", "id": "c", "command": []string{"go", "test"}, "aggregated_output": "failed", "exit_code": 1}})
	d, f := parseFixture(t, data)
	if len(d.events) != 2 || d.summary.ToolCount != 1 || d.events[1].Status != "error" {
		t.Fatalf("duplicate completion: %+v", d)
	}
	text, err := eventText(f, d.events[1])
	if err != nil || strings.Count(text, "failed") != 1 {
		t.Fatalf("duplicate output: %q %v", text, err)
	}
}

func TestSinglePassUserSourceSelection(t *testing.T) {
	instructions := response("message", map[string]any{"role": "user", "content": "环境说明"})
	answer := response("message", map[string]any{"role": "assistant", "content": "回复"})
	data := meta("s", "/p") + instructions + answer + user("实际请求") + answer
	d, _ := parseFixture(t, data)
	if d.summary.Title != "实际请求" || len(d.events) != 3 || d.events[0].Turn != 0 || d.events[1].Kind != "user" || d.events[2].Turn != 1 {
		t.Fatalf("late canonical user did not discard instruction items: %+v", d)
	}
	d, _ = parseFixture(t, meta("s", "/p")+instructions+answer)
	if d.summary.Title != "环境说明" || len(d.events) != 2 || d.events[0].Turn != 1 {
		t.Fatalf("response-only rollout lost its user: %+v", d)
	}
}

func TestSummaryMatchesDetailWithoutEventIndex(t *testing.T) {
	data := meta("s", "/p") + user("请求") +
		response("function_call", map[string]any{"name": "exec_command", "call_id": "c", "arguments": "{}"}) +
		response("function_call_output", map[string]any{"call_id": "c", "output": "result"}) +
		response("function_call_output", map[string]any{"call_id": "c", "output": "extra"}) + "{broken}\n"
	d, f := parseFixture(t, data)
	summary, err := readSummary(context.Background(), f, "rollout-test.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summary.summary, d.summary) || !reflect.DeepEqual(summary.issues, d.issues) || len(summary.events) != 0 || len(summary.turns) != 0 {
		t.Fatalf("summary diverged or retained detail: %+v", summary)
	}
}

func TestStreamingMatchesKeepTimelineOrderAndMergedOffsets(t *testing.T) {
	data := meta("s", "/p") + response("message", map[string]any{"role": "user", "content": "needle instructions"}) + user("实际请求") +
		response("function_call", map[string]any{"name": "needle_tool", "call_id": "c", "arguments": "参数"}) +
		response("message", map[string]any{"role": "assistant", "content": "needle reply"}) +
		response("function_call_output", map[string]any{"call_id": "c", "output": "输出 needle"})
	d, f := parseFixture(t, data)
	matches, err := readMatches(context.Background(), f, "rollout-test.jsonl", "needle")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches.matches) != 2 || matches.matches[0].EventID != d.events[1].ID || matches.matches[1].EventID != d.events[2].ID {
		t.Fatalf("streaming search lost source selection or timeline order: %+v", matches.matches)
	}
	text, err := eventText(f, d.events[1])
	if err != nil {
		t.Fatal(err)
	}
	match := matches.matches[0]
	if !strings.HasPrefix(string([]rune(text)[match.TextOffset:]), "needle") || match.Snippet != excerpt(text, "needle", 180) {
		t.Fatalf("merged tool match points to a different position: %+v in %q", match, text)
	}
	if len(matches.events) != 0 {
		t.Fatal("search retained a detail event index")
	}
}

func BenchmarkRollout(b *testing.B) {
	path := filepath.Join(b.TempDir(), "rollout.jsonl")
	data := meta("s", "/p") + user("request")
	for i := 0; i < 100; i++ {
		data += response("function_call", map[string]any{"name": "exec_command", "call_id": string(rune(i + 100)), "arguments": "{}"}) +
			response("function_call_output", map[string]any{"call_id": string(rune(i + 100)), "output": strings.Repeat("output ", 6000)})
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		b.Fatal(err)
	}
	for _, index := range []bool{false, true} {
		name := "summary"
		if index {
			name = "detail"
		}
		b.Run(name, func(b *testing.B) {
			f, err := os.Open(path)
			if err != nil {
				b.Fatal(err)
			}
			defer f.Close()
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := readRollout(context.Background(), f, "rollout.jsonl", index, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
