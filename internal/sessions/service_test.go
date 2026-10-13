package sessions

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	searchList, err := s.List(ListQuery{Search: "最终命中"})
	if err != nil || len(searchList.Sessions) != 1 || searchList.Sessions[0].MatchTextOffset < textChunkRunes {
		t.Fatalf("list match position: %+v %v", searchList, err)
	}
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

func TestCanceledAndSupersededRequestsStayIndependent(t *testing.T) {
	s, root := newFixtureService(t)
	fixture(t, filepath.Join(root, "s.jsonl"), meta("s", "/p")+user("request"))
	list, finishList := s.beginRequest("list", 1)
	defer finishList()
	detail, finishDetail := s.beginRequest("detail", 1)
	defer finishDetail()
	newList, finishNewList := s.beginRequest("list", 2)
	defer finishNewList()
	if list.Err() != context.Canceled || detail.Err() != nil || newList.Err() != nil {
		t.Fatal("superseding a search canceled a detail request")
	}
	if err := s.CancelRequest("list", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ListQuery{RequestID: 3}); err != context.Canceled {
		t.Fatalf("a late canceled bridge call still ran: %v", err)
	}
	page, err := s.List(ListQuery{RequestID: 4})
	if err != nil || len(page.Sessions) != 1 {
		t.Fatalf("new request did not recover: %+v %v", page, err)
	}
	if err := s.CancelRequest("list", 2); err != nil || detail.Err() != nil {
		t.Fatal("late cancellation affected another scope")
	}
}

func TestSearchPagesReuseResultsAndInvalidateOnChanges(t *testing.T) {
	s, root := newFixtureService(t)
	for i := 0; i < listPageSize+1; i++ {
		fixture(t, filepath.Join(root, fmt.Sprintf("%d.jsonl", i)), meta(fmt.Sprint(i), "/p")+user("needle"))
	}
	page, err := s.List(ListQuery{Search: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	cached := s.listCache
	next, err := s.List(ListQuery{Search: "needle", Cursor: page.NextCursor})
	if err != nil || len(next.Sessions) != 1 || s.listCache != cached {
		t.Fatalf("search pagination did not reuse the snapshot: %+v %v", next, err)
	}
	path := filepath.Join(root, "0.jsonl")
	data := meta("0", "/p") + strings.Repeat(user("needle"), matchPageSize+1)
	fixture(t, path, data)
	page, err = s.List(ListQuery{Search: "needle"})
	if err != nil || s.listCache == cached {
		t.Fatal("changed rollout did not invalidate list search")
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte("0.jsonl")))
	matches, err := s.Find(MatchQuery{ID: id, Search: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	findCache := s.matchCache
	more, err := s.Find(MatchQuery{ID: id, Search: "needle", Cursor: matches.NextCursor})
	if err != nil || len(more.Matches) != 1 || s.matchCache != findCache {
		t.Fatalf("find pagination did not reuse full-file results: %+v %v", more, err)
	}
	fixture(t, path, meta("0", "/p")+user("changed"))
	if _, err := s.Find(MatchQuery{ID: id, Search: "needle", Cursor: matches.NextCursor}); err == nil {
		t.Fatal("a cursor from the old file was accepted")
	}
}

func TestConcurrentSearchAndDetailReads(t *testing.T) {
	s, root := newFixtureService(t)
	for i := 0; i < cachedDocuments+4; i++ {
		fixture(t, filepath.Join(root, fmt.Sprintf("%d.jsonl", i)), meta(fmt.Sprint(i), "/p")+user("needle"))
	}
	list, err := s.List(ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for _, summary := range list.Sessions {
		workers.Add(1)
		go func(id string) {
			defer workers.Done()
			page, err := s.Read(ReadQuery{ID: id})
			if err != nil || len(page.Events) != 1 {
				t.Errorf("concurrent detail: %+v %v", page, err)
				return
			}
			matches, err := s.Find(MatchQuery{ID: id, Search: "needle"})
			if err != nil || len(matches.Matches) != 1 {
				t.Errorf("concurrent find: %+v %v", matches, err)
			}
			text, err := s.ReadText(TextQuery{ID: id, EventID: page.Events[0].ID, Revision: page.Revision})
			if err != nil || text.Text != "needle" {
				t.Errorf("evicted detail changed its revision: %+v %v", text, err)
			}
		}(summary.ID)
	}
	searched, err := s.List(ListQuery{Search: "needle"})
	if err != nil || len(searched.Sessions) != len(list.Sessions) {
		t.Errorf("concurrent list: %+v %v", searched, err)
	}
	workers.Wait()
}
