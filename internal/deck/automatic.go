package deck

import (
	"context"
	"time"
)

const autoRefreshInterval = 5 * time.Minute

func nextUsageRefresh(account Account, attemptedAt time.Time) time.Time {
	latest := attemptedAt
	if account.Usage != nil && account.Usage.QueriedAt.After(latest) {
		latest = account.Usage.QueriedAt
	}
	if latest.IsZero() {
		return time.Time{}
	}
	next := latest.Add(autoRefreshInterval)
	if account.Usage != nil {
		for _, window := range []*Window{account.Usage.FiveHour, account.Usage.Weekly} {
			if window != nil && window.ResetAt != nil && window.ResetAt.After(latest) && window.ResetAt.Before(next) {
				next = *window.ResetAt
			}
		}
	}
	return next
}

// 用户操作取消用量 GET 和批次剩余查询，不取消已发出的令牌交换。
func (s *Service) CancelAutomatic() {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoCancel != nil {
		s.autoCancel()
	}
}

func (s *Service) SetAutomaticPaused(paused bool) {
	s.autoMu.Lock()
	s.autoPaused = paused
	if paused && s.autoCancel != nil {
		s.autoCancel()
	}
	s.autoMu.Unlock()
	s.WakeAutomatic()
}

func (s *Service) WakeAutomatic() {
	select {
	case s.autoWake <- struct{}{}:
	default:
	}
}

// 调度独立于窗口和 WebView；同一时刻只执行一个自动查询批次。
func (s *Service) StartAutomatic() {
	s.autoOnce.Do(func() {
		s.autoWorkers.Add(1)
		go func() {
			defer s.autoWorkers.Done()
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-s.autoWake:
				case <-timer.C:
				}
				if s.ctx.Err() != nil {
					return
				}
				state := s.RefreshAutomatic()
				s.autoMu.Lock()
				paused := s.autoPaused
				s.autoMu.Unlock()
				if paused {
					timer.Stop()
					continue
				}
				delay := time.Until(*state.NextRefreshAt)
				// 取消后等待下一周期或操作结束唤醒，避免立即重发取消的请求。
				if delay <= 0 {
					delay = autoRefreshInterval
				}
				timer.Reset(delay)
			}
		}()
	})
}

func (s *Service) RefreshAutomatic() (state State) {
	s.autoRunMu.Lock()
	defer s.autoRunMu.Unlock()
	s.autoMu.Lock()
	if s.autoPaused || s.ctx.Err() != nil {
		s.autoMu.Unlock()
		return s.GetState()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.autoCancel = cancel
	s.autoMu.Unlock()
	defer func() {
		cancel()
		s.autoMu.Lock()
		s.autoCancel = nil
		s.autoMu.Unlock()
		s.mu.Lock()
		s.autoRefreshing = false
		s.publish()
		state = s.state()
		s.mu.Unlock()
	}()
	s.mu.Lock()
	s.autoRefreshing = true
	s.publish()
	s.syncCLIAccount(true)
	accounts := s.store.Snapshot().Accounts
	s.mu.Unlock()
	for _, account := range accounts {
		if ctx.Err() != nil {
			break
		}
		s.mu.Lock()
		current, err := findAccount(s.store.Snapshot(), account.ID)
		if err == nil && current.AuthError == "" && !time.Now().Before(nextUsageRefresh(current, s.usageTries[current.ID])) {
			s.refreshUsageContext(ctx, current.ID)
		}
		s.mu.Unlock()
	}
	return s.GetState()
}
