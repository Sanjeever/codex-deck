package deck

import "fmt"

func (s *Service) SavePreferences(preferences Preferences) error {
	s.CancelAutomatic()
	if err := validatePreferences(preferences); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.Update(func(d *Data) error { d.Preferences = preferences; return nil }); err != nil {
		return err
	}
	s.autoBackup()
	return nil
}

func validatePreferences(p Preferences) error {
	if p.LowQuotaThreshold < 0 || p.LowQuotaThreshold > 99 {
		return fmt.Errorf("低额度阈值应为 1–99%%")
	}
	if p.Sort != "" && p.Sort != "name" && p.Sort != "quota" {
		return fmt.Errorf("不支持的账号排序")
	}
	return nil
}
