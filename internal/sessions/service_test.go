package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFixtureService(t *testing.T) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	return NewService(context.Background(), home), filepath.Join(home, "sessions")
}

func TestListSearchAndPagination(t *testing.T) {
	s, root := newFixtureService(t)
	for i := 0; i < listPageSize+3; i++ {
		data := meta(fmt.Sprint(i), "/project") + user(fmt.Sprintf("request %d", i))
		if i == listPageSize+2 {
			data += response("message", map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "deep search needle"}}})
		}
		fixture(t, filepath.Join(root, fmt.Sprintf("2026/10/13/rollout-%03d.jsonl", i)), data)
	}
	page, err := s.List(ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Sessions) != listPageSize || page.NextCursor == "" || page.Source != root || len(page.Projects) != 1 || len(page.Dates) != 1 {
		t.Fatalf("page: %+v", page)
	}
	next, err := s.List(ListQuery{Cursor: page.NextCursor})
	if err != nil || len(next.Sessions) != 3 {
		t.Fatalf("next: %+v %v", next, err)
	}
	matched, err := s.List(ListQuery{Search: "needle", Project: "/project", Date: "2026-10-13"})
	if err != nil || len(matched.Sessions) != 1 || matched.Sessions[0].MatchEventID == "" || !strings.Contains(matched.Sessions[0].Snippet, "needle") {
		t.Fatalf("content search: %+v %v", matched, err)
	}
	none, err := s.List(ListQuery{Project: "/other"})
	if err != nil || len(none.Sessions) != 0 {
		t.Fatalf("filter: %+v %v", none, err)
	}
}

func TestDetailSearchBeyondPageAndLongOutput(t *testing.T) {
	s, root := newFixtureService(t)
	data := meta("s", "/p")
	for i := 0; i < eventPageSize+5; i++ {
		data += user(fmt.Sprint(i))
	}
	long := strings.Repeat("输出", textChunkRunes) + "最终命中"
	data += response("function_call", map[string]any{"name": "tool", "call_id": "long", "arguments": "{}"}) + response("function_call_output", map[string]any{"call_id": "long", "output": long})
	fixture(t, filepath.Join(root, "rollout-s.jsonl"), data)
	list, err := s.List(ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	id := list.Sessions[0].ID
	page, err := s.Read(ReadQuery{ID: id})
	if err != nil || len(page.Events) != eventPageSize || page.NextCursor == "" {
		t.Fatalf("first: %+v %v", page, err)
	}
	matches, err := s.Find(MatchQuery{ID: id, Search: "最终命中"})
	if err != nil || len(matches.Matches) != 1 || matches.Matches[0].TextOffset < textChunkRunes {
		t.Fatalf("full search: %+v %v", matches, err)
	}
	target, err := s.Read(ReadQuery{ID: id, EventID: matches.Matches[0].EventID})
	if err != nil || len(target.Events) != 1 || target.Events[0].TextCursor == 0 {
		t.Fatalf("target: %+v %v", target, err)
	}
	text, err := s.ReadText(TextQuery{ID: id, EventID: target.Events[0].ID, Revision: target.Revision, Offset: matches.Matches[0].TextOffset})
	if err != nil || !strings.Contains(text.Text, "最终命中") {
		t.Fatalf("target chunk: %+v %v", text, err)
	}
	next, err := s.ReadText(TextQuery{ID: id, EventID: target.Events[0].ID, Revision: target.Revision, Offset: target.Events[0].TextCursor})
	if err != nil || len([]rune(next.Text)) > textChunkRunes {
		t.Fatalf("text bound: %+v %v", next, err)
	}
	if _, err = s.ReadText(TextQuery{ID: id, EventID: target.Events[0].ID, Revision: target.Revision, Offset: -1}); err == nil {
		t.Fatal("negative offset accepted")
	}
}

func TestMissingDamagedDeletedAndChangedFiles(t *testing.T) {
	s, root := newFixtureService(t)
	missing, err := s.List(ListQuery{})
	if err != nil || !missing.Missing {
		t.Fatalf("missing: %+v %v", missing, err)
	}
	path := filepath.Join(root, "rollout-s.jsonl")
	fixture(t, path, meta("s", "/p")+user("old"))
	fixture(t, filepath.Join(root, "broken.jsonl"), "bad\n")
	fixture(t, filepath.Join(root, "compressed.jsonl.zst"), "compressed")
	list, err := s.List(ListQuery{})
	if err != nil || len(list.Sessions) != 1 || len(list.Issues) != 2 {
		t.Fatalf("partial: %+v %v", list, err)
	}
	id := list.Sessions[0].ID
	page, err := s.Read(ReadQuery{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	fixture(t, path, meta("s", "/p")+user("changed content"))
	matches, err := s.Find(MatchQuery{ID: id, Search: "changed"})
	if err != nil || len(matches.Matches) != 1 {
		t.Fatalf("invalidation: %+v %v", matches, err)
	}
	if _, err = s.ReadText(TextQuery{ID: id, EventID: page.Events[0].ID, Revision: page.Revision}); err == nil {
		t.Fatal("old snapshot accepted")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ReadQuery{ID: id}); err == nil {
		t.Fatal("deleted file accepted")
	}
}

func TestPathAndLinkBoundary(t *testing.T) {
	s, root := newFixtureService(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	fixture(t, outside, meta("outside", "/p")+user("private"))
	fixture(t, filepath.Join(root, "rollout-good.jsonl"), meta("good", "/p"))
	if f, err := s.secureOpen(outside); err == nil {
		f.Close()
		t.Fatal("outside file accepted")
	}
	if _, err := s.Read(ReadQuery{ID: "../outside.jsonl"}); err == nil {
		t.Fatal("path used as ID")
	}
	link := filepath.Join(root, "linked.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("当前系统不允许创建符号链接：%v", err)
	}
	list, err := s.List(ListQuery{})
	if err != nil || len(list.Sessions) != 1 || len(list.Issues) != 1 {
		t.Fatalf("link scan: %+v %v", list, err)
	}
	if f, err := s.secureOpen(link); err == nil {
		f.Close()
		t.Fatal("link accepted")
	}
}

func TestNewestOrderAndInvalidCursor(t *testing.T) {
	s, root := newFixtureService(t)
	fixture(t, filepath.Join(root, "a.jsonl"), strings.ReplaceAll(meta("a", "/p")+user("a"), "08:00:00Z", "08:00:00.1Z"))
	fixture(t, filepath.Join(root, "b.jsonl"), meta("b", "/p")+user("b"))
	list, err := s.List(ListQuery{})
	if err != nil || list.Sessions[0].Title != "a" {
		t.Fatalf("time sorting: %+v %v", list, err)
	}
	if _, err = s.List(ListQuery{Cursor: "arbitrary"}); err == nil {
		t.Fatal("invalid cursor accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed := NewService(ctx, t.TempDir())
	if _, err = closed.List(ListQuery{}); err != context.Canceled {
		t.Fatal(err)
	}
}
