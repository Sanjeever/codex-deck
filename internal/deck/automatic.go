package deck

import "time"

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

func (s *Service) RefreshAutomatic() State {
	s.mu.Lock()
	s.syncCLIAccount(true)
	accounts := s.store.Snapshot().Accounts
	s.mu.Unlock()
	for _, account := range accounts {
		s.mu.Lock()
		current, err := findAccount(s.store.Snapshot(), account.ID)
		if err == nil && current.AuthError == "" && !time.Now().Before(nextUsageRefresh(current, s.usageTries[current.ID])) {
			s.refreshUsage(current.ID)
		}
		s.mu.Unlock()
	}
	return s.GetState()
}
