package main

import (
	"codex-deck/internal/deck"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ImportAuth() (*deck.AuthImportPreview, error) {
	s, err := a.backend()
	if err != nil {
		return nil, err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入 auth.json", DefaultFilename: "auth.json", ShowHiddenFiles: true,
		Filters: []runtime.FileFilter{{DisplayName: "JSON 认证文件", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	preview, err := s.PrepareAuthImport(path)
	if err != nil {
		return nil, err
	}
	if !preview.Existing {
		if err := s.ConfirmAuthImport(preview.ID); err != nil {
			s.CancelAuthImport()
			return nil, err
		}
	}
	return &preview, nil
}

func (a *App) ConfirmAuthImport(id string) error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	return s.ConfirmAuthImport(id)
}

func (a *App) CancelAuthImport() error {
	s, err := a.backend()
	if err != nil {
		return err
	}
	s.CancelAuthImport()
	return nil
}

func (a *App) ExportAuth(id string) (bool, error) {
	s, err := a.backend()
	if err != nil {
		return false, err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "导出 auth.json", DefaultFilename: "auth.json", ShowHiddenFiles: true, CanCreateDirectories: true,
		Filters: []runtime.FileFilter{{DisplayName: "JSON 认证文件", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return false, err
	}
	cliTarget, err := s.IsCLIAuthPath(path)
	if err != nil {
		return false, err
	}
	if cliTarget {
		choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type: runtime.WarningDialog, Title: "覆盖 CLI 凭据", Message: "覆盖 CLI 的 auth.json？这会改变 CLI 当前账号。重新启动 Codex CLI 后生效。",
			Buttons: []string{"取消", "确认覆盖"}, DefaultButton: "取消", CancelButton: "取消",
		})
		if err != nil || choice != "确认覆盖" {
			return false, err
		}
	}
	if err := s.ExportAuth(id, path, cliTarget); err != nil {
		return false, err
	}
	return true, nil
}
