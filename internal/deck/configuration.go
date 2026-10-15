package deck

import (
	"strings"

	"codex-deck/internal/network"
)

type ConfigurationPreferences struct {
	LowQuotaNotify     bool `json:"low_quota_notify"`
	LowQuotaThreshold  int  `json:"low_quota_threshold"`
	DisableUpdateCheck bool `json:"disable_update_check"`
}

type Configuration struct {
	Network     network.Config           `json:"network"`
	Settings    Settings                 `json:"settings"`
	Preferences ConfigurationPreferences `json:"preferences"`
}

type ConfigurationError struct {
	Section string
	Err     error
}

func (e *ConfigurationError) Error() string { return e.Err.Error() }
func (e *ConfigurationError) Unwrap() error { return e.Err }

func ValidateConfiguration(config Configuration) (Configuration, error) {
	var err error
	config.Network, err = network.Normalize(config.Network)
	if err != nil {
		return config, &ConfigurationError{Section: "network", Err: err}
	}
	config.Settings.WebDAVURL = strings.TrimSpace(config.Settings.WebDAVURL)
	if err := validateSettings(config.Settings); err != nil {
		return config, &ConfigurationError{Section: "backup", Err: err}
	}
	if err := validatePreferences(Preferences{LowQuotaThreshold: config.Preferences.LowQuotaThreshold}); err != nil {
		return config, &ConfigurationError{Section: "general", Err: err}
	}
	return config, nil
}

// 设置页一次写入本地配置，成功后才切换代理；系统自启动由独立接口修改。
func (s *Service) SaveConfiguration(config Configuration) (Configuration, error) {
	config, err := ValidateConfiguration(config)
	if err != nil {
		return config, err
	}
	route, err := network.Prepare(config.Network)
	if err != nil {
		return config, err
	}
	s.CancelAutomatic()
	if s.store.Snapshot().Settings != config.Settings {
		s.stopBackup()
		s.backupIO.Lock()
		defer s.backupIO.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.store.Snapshot()
	backupChanged := previous.Settings != config.Settings
	preferencesChanged := previous.Preferences.LowQuotaNotify != config.Preferences.LowQuotaNotify || previous.Preferences.LowQuotaThreshold != config.Preferences.LowQuotaThreshold || previous.Preferences.DisableUpdateCheck != config.Preferences.DisableUpdateCheck
	if err := s.store.Update(func(d *Data) error {
		d.Network = &config.Network
		d.Settings = config.Settings
		d.Preferences.LowQuotaNotify = config.Preferences.LowQuotaNotify
		d.Preferences.LowQuotaThreshold = config.Preferences.LowQuotaThreshold
		d.Preferences.DisableUpdateCheck = config.Preferences.DisableUpdateCheck
		return nil
	}); err != nil {
		return config, err
	}
	if previous.Network == nil || *previous.Network != config.Network {
		s.HTTP.Apply(route)
	}
	if backupChanged {
		s.backup = BackupStatus{}
	}
	if backupChanged || preferencesChanged {
		s.autoBackup()
	}
	return config, nil
}
