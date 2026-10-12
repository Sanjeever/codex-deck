package deck

import (
	"fmt"
	"strings"
	"time"
)

type NotificationMark struct {
	ResetAt *time.Time `json:"reset_at"`
	Sent    bool       `json:"sent"`
}
type QuotaNotifications struct {
	FiveHour NotificationMark `json:"five_hour"`
	Weekly   NotificationMark `json:"weekly"`
}

func (p Preferences) threshold() float64 {
	if p.LowQuotaThreshold == 0 {
		return 10
	}
	return float64(p.LowQuotaThreshold)
}

func quotaNotice(window *Window, mark *NotificationMark, at time.Time, threshold float64, label string) string {
	if window == nil || (window.ResetAt != nil && !window.ResetAt.After(at)) {
		return ""
	}
	if mark.ResetAt != nil && !mark.ResetAt.After(at) {
		*mark = NotificationMark{}
	}
	if mark.ResetAt == nil {
		mark.ResetAt = window.ResetAt
	}
	if window.RemainingPercent > threshold {
		if window.ResetAt == nil {
			*mark = NotificationMark{}
		}
		return ""
	}
	if mark.Sent {
		return ""
	}
	mark.Sent = true
	return fmt.Sprintf("%s 剩余 %.1f%%", label, window.RemainingPercent)
}

func (s *Service) lowQuotaNotice(a *Account, p Preferences) string {
	if !p.LowQuotaNotify || s.notifier == nil || a.Usage == nil || s.ctx.Err() != nil {
		return ""
	}
	mode, err := s.cli.StoreMode()
	if err != nil || mode != "file" {
		return ""
	}
	_, identity, err := s.cli.Read()
	if err != nil || !identityMatches(*a, identity) {
		return ""
	}
	messages := []string{}
	if message := quotaNotice(a.Usage.FiveHour, &a.Notifications.FiveHour, a.Usage.QueriedAt, p.threshold(), "5H"); message != "" {
		messages = append(messages, message)
	}
	if message := quotaNotice(a.Usage.Weekly, &a.Notifications.Weekly, a.Usage.QueriedAt, p.threshold(), "Weekly"); message != "" {
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		return ""
	}
	return "当前账号 · " + strings.Join(messages, "；")
}

func (s *Service) sendQuotaNotice(body string) {
	if body == "" {
		return
	}
	notify := s.notifier
	s.notificationWorkers.Add(1)
	go func() {
		defer s.notificationWorkers.Done()
		if err := notify(body); err != nil {
			s.SetDesktopError(fmt.Errorf("低额度通知失败: %w", err))
		}
	}()
}

func (s *Service) SetNotifier(callback func(string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = callback
}
