package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"codex-deck/internal/deck"
)

func TestCLIProxyBindingsKeepDeckConfigurationIndependent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "deck.json")
	cliHome := filepath.Join(root, "custom-codex-home")
	s, err := deck.NewService(context.Background(), path, cliHome)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	a := &App{ctx: context.Background(), service: s, ready: make(chan struct{})}
	close(a.ready)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := a.GetCLIProxy()
	if err != nil || state.Path != filepath.Join(cliHome, ".env") {
		t.Fatalf("read: %+v %v", state, err)
	}
	state, err = a.SaveCLIProxy(deck.CLIProxyChange{URL: "http://localhost:7897", Expected: state.Variables})
	if err != nil || len(state.Variables) != 2 {
		t.Fatalf("save: %+v %v", state, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || s.GetState().Network.Mode != "environment" {
		t.Fatal("CLI 代理改写了 Deck 配置或路由")
	}
	if err := a.TestCLIProxy("invalid"); err == nil {
		t.Fatal("连接测试接受了无效代理")
	}
	state, err = a.SaveCLIProxy(deck.CLIProxyChange{Remove: true, Expected: state.Variables})
	if err != nil || len(state.Variables) != 0 {
		t.Fatalf("remove: %+v %v", state, err)
	}
}
