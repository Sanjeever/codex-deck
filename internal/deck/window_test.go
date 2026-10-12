package deck

import "testing"

func TestWindowStateUsesSingleConfigFile(t *testing.T) {
	s := testService(t)
	window := WindowState{X: 64, Y: 32, Width: 1040, Height: 640}
	if err := s.SaveWindow(window); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().Window == nil || *store.Snapshot().Window != window {
		t.Fatal("窗口状态未保存")
	}
	if err := s.SaveWindow(WindowState{Width: 0, Height: 0}); err == nil {
		t.Fatal("接受无效窗口尺寸")
	}
}

func TestInvalidRestoredWindowDoesNotOverwriteConfig(t *testing.T) {
	s := testService(t)
	window := WindowState{Width: 1000, Height: 600}
	if err := s.SaveWindow(window); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Update(func(d *Data) error { d.Window.Width = 0; return nil }); err == nil {
		t.Fatal("恢复配置接受无效窗口")
	}
	reloaded, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if *reloaded.Snapshot().Window != window {
		t.Fatal("无效更新覆盖已有窗口状态")
	}
}
