package sessions

import (
	"context"
	"fmt"
)

type request struct {
	id     uint64
	cancel context.CancelFunc
}

// Remember canceled generations too: a bridge call can arrive after its cancel.
func (s *Service) beginRequest(scope string, id uint64) (context.Context, func()) {
	if id == 0 {
		return s.ctx, func() {}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.requests[scope]
	ctx, cancel := context.WithCancel(s.ctx)
	if id <= previous.id {
		cancel()
		return ctx, cancel
	}
	if previous.cancel != nil {
		previous.cancel()
	}
	s.requests[scope] = request{id, cancel}
	return ctx, cancel
}

func (s *Service) CancelRequest(scope string, id uint64) error {
	if scope != "list" && scope != "detail" && scope != "find" {
		return fmt.Errorf("未知会话请求范围")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.requests[scope]
	if id >= previous.id {
		if previous.cancel != nil {
			previous.cancel()
		}
		s.requests[scope] = request{id: id}
	}
	return nil
}
