package deck

import (
	"context"
	"errors"
	"strings"
	"time"
)

// 上传始终串行。待处理任务只保留最新快照，用户编辑不等待网络。
func (s *Service) autoBackup() {
	if s.store.Snapshot().Settings.WebDAVURL == "" {
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
			s.performBackup()
		}
		s.backupIO.Unlock()
	}
}

func (s *Service) performBackup() error {
	s.mu.Lock()
	d := s.store.Snapshot()
	ctx, cancel := context.WithCancel(s.ctx)
	s.backupCancel = cancel
	s.backup.Busy = true
	s.mu.Unlock()
	_, err := writeSnapshot(ctx, d)
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
	if err != nil {
		s.backup.Error = err.Error()
		return err
	}
	return nil
}

func (s *Service) Backup() error {
	s.CancelAutomatic()
	s.backupIO.Lock()
	defer s.backupIO.Unlock()
	s.mu.Lock()
	s.backupPending = false
	s.mu.Unlock()
	return s.performBackup()
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
		return nil
	}); err != nil {
		return err
	}
	if changed {
		s.backup = BackupStatus{}
	}
	s.autoBackup()
	return nil
}
