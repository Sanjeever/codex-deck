package deck

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreSerialSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex-deck.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Update(func(d *Data) error { d.LastSwitchedID += "x"; return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	reloaded, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Snapshot().LastSwitchedID) != 20 {
		t.Fatal("并发保存丢失更新")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("留下额外文件")
	}
	copy := s.Snapshot()
	copy.Settings.WebDAVUsername = "changed"
	if s.Snapshot().Settings.WebDAVUsername != "" {
		t.Fatal("快照修改影响配置")
	}
}

func TestCorruptStoreIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex-deck.json")
	bad := []byte(`{"version":1,"accounts":`)
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Fatal("损坏配置应报错")
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(bad) {
		t.Fatal("覆盖损坏配置")
	}
}

func TestFailedSaveKeepsPreviousData(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "deck.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(func(d *Data) error { d.Version = 9; return nil }); err == nil {
		t.Fatal("接受无效版本")
	}
	if s.Snapshot().Version != 1 {
		t.Fatal("失败保存影响内存")
	}
	s.path = filepath.Join(t.TempDir(), "directory")
	if err = os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.Update(func(d *Data) error { d.LastSwitchedID = "x"; return nil }); err == nil {
		t.Fatal("保存失败未报错")
	}
	if s.Snapshot().LastSwitchedID != "" {
		t.Fatal("失败保存影响内存")
	}
}
