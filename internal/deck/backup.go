package deck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 上传始终串行。待处理任务只保留最新快照，用户编辑不等待网络。
func (s *Service) autoBackup() {
	if s.store.Snapshot().Settings.WebDAVURL == "" || s.backup.Conflict {
		return
	}
	s.backupPending = true
	s.backupOnce.Do(func() { s.backupWorkers.Add(1); go s.backupWorker() })
	select {
	case s.backupWake <- struct{}{}:
	default:
	}
}

func (s *Service) backupWorker() {
	defer s.backupWorkers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.backupWake:
		}
		s.backupIO.Lock()
		s.mu.Lock()
		pending := s.backupPending
		s.backupPending = false
		if pending {
			s.backup.Busy = true
		}
		s.mu.Unlock()
		if pending {
			s.performBackup(false)
		}
		s.backupIO.Unlock()
	}
}

func (s *Service) performBackup(overwrite bool) error {
	s.mu.Lock()
	d := s.store.Snapshot()
	expected := d.Sync.ETag
	if d.Sync.URL != d.Settings.WebDAVURL || d.Sync.Username != d.Settings.WebDAVUsername {
		expected = ""
	}
	if overwrite {
		expected = s.conflictETag
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.backupCancel = cancel
	s.backup.Busy = true
	s.mu.Unlock()
	tag, err := writeBackup(ctx, d, expected, !overwrite && expected == "")
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.publish()
	s.backupCancel = nil
	s.backup.Busy = false
	if ctx.Err() != nil && err != nil && errors.Is(err, context.Canceled) {
		return err
	}
	at := time.Now().UTC()
	s.backup.At = &at
	s.backup.Error = ""
	s.backup.Conflict = false
	if err != nil {
		s.backup.Error = err.Error()
		s.backup.Conflict = errors.Is(err, ErrBackupConflict)
		if s.backup.Conflict {
			s.conflictETag = tag
			s.backupPending = false
		}
		return err
	}
	s.conflictETag = ""
	err = s.store.Update(func(current *Data) error {
		current.Sync = BackupSync{URL: d.Settings.WebDAVURL, Username: d.Settings.WebDAVUsername, ETag: tag}
		return nil
	})
	if err != nil {
		s.backup.Error = fmt.Sprintf("远端已上传，保存备份版本失败: %v", err)
	}
	return err
}

func (s *Service) Backup() error          { return s.backupManual(false) }
func (s *Service) OverwriteBackup() error { return s.backupManual(true) }
func (s *Service) backupManual(overwrite bool) error {
	s.CancelAutomatic()
	s.backupIO.Lock()
	defer s.backupIO.Unlock()
	s.mu.Lock()
	if overwrite && s.backup.Conflict && s.conflictETag == "" {
		s.mu.Unlock()
		return fmt.Errorf("远端未提供 ETag，无法安全确认覆盖")
	}
	if overwrite && !s.backup.Conflict {
		s.mu.Unlock()
		return fmt.Errorf("覆盖确认已失效，请重新检查备份")
	}
	s.backupPending = false
	s.mu.Unlock()
	return s.performBackup(overwrite)
}

func (s *Service) stopBackup() {
	s.mu.Lock()
	s.backupPending = false
	if s.backupCancel != nil {
		s.backupCancel()
	}
	s.mu.Unlock()
}

func (s *Service) SaveSettings(settings Settings) error {
	s.CancelAutomatic()
	settings.WebDAVURL = strings.TrimSpace(settings.WebDAVURL)
	if err := validateSettings(settings); err != nil {
		return err
	}
	s.stopBackup()
	s.backupIO.Lock()
	defer s.backupIO.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.store.Snapshot().Settings
	changed := previous.WebDAVURL != settings.WebDAVURL || previous.WebDAVUsername != settings.WebDAVUsername || previous.WebDAVPassword != settings.WebDAVPassword
	if err := s.store.Update(func(d *Data) error {
		d.Settings = settings
		if changed {
			d.Sync = BackupSync{}
		}
		return nil
	}); err != nil {
		return err
	}
	if changed {
		s.backup = BackupStatus{}
		s.conflictETag = ""
	}
	s.autoBackup()
	return nil
}
