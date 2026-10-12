package deck

import "fmt"

type WindowState struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (s *Service) WindowState() *WindowState { return s.store.Snapshot().Window }
func (s *Service) SaveWindow(window WindowState) error {
	if window.Width < 860 || window.Height < 500 {
		return fmt.Errorf("窗口尺寸小于最小值")
	}
	return s.store.Update(func(d *Data) error { d.Window = &window; return nil })
}
func (s *Service) SetDesktopError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.desktopError = ""
	if err != nil {
		s.desktopError = err.Error()
	}
	s.publish()
}
