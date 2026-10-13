package main

import (
	"codex-deck/internal/sessions"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionBindingsIndependentOfAccounts(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("{\"timestamp\":\"2026-10-13T08:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"synthetic\",\"cwd\":\"/test\"}}\n{\"timestamp\":\"2026-10-13T08:01:00Z\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"测试请求\"}}\n")
	path := filepath.Join(root, "rollout-test.jsonl")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.sessions = sessions.NewService(context.Background(), home)
	a.err = fmt.Errorf("账号加载失败")
	close(a.ready)
	list, err := a.ListSessions(sessions.ListQuery{})
	if err != nil || len(list.Sessions) != 1 || list.Source != root {
		t.Fatalf("list: %+v %v", list, err)
	}
	id := list.Sessions[0].ID
	page, err := a.ReadSession(sessions.ReadQuery{ID: id})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("read: %+v %v", page, err)
	}
	matches, err := a.FindSessionMatches(sessions.MatchQuery{ID: id, Search: "测试"})
	if err != nil || len(matches.Matches) != 1 {
		t.Fatalf("find: %+v %v", matches, err)
	}
	if _, err = a.ReadSession(sessions.ReadQuery{ID: path}); err == nil {
		t.Fatal("absolute path accepted as ID")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("session file changed")
	}
	if a.service != nil {
		t.Fatal("account service should not be required")
	}
}
