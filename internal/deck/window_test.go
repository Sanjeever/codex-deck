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
