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

func (s *Service) RefreshAutomatic() State {
	s.autoMu.Lock()
	if s.autoCancel != nil {
		s.autoCancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.autoCancel = cancel
	s.autoMu.Unlock()
	defer cancel()
	s.mu.Lock()
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
