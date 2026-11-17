package sessions

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type ResumePlan struct {
	Command        string `json:"command"`
	RestartCommand string `json:"restart_command"`
	Shell          string `json:"shell"`
}

func (s *Service) PrepareResume(id string) (ResumePlan, error) {
	e, f, err := s.session(s.ctx, id, false)
	if err != nil {
		return ResumePlan{}, err
	}
	defer f.Close()
	if e.summary.ResumeCommand == "" {
		return ResumePlan{}, fmt.Errorf("会话未记录有效的 Codex ID")
	}
	if e.summary.Project == "" {
		return ResumePlan{}, fmt.Errorf("会话未记录工作目录")
	}
	if err := s.unchanged(e, f); err != nil {
		return ResumePlan{}, err
	}
	return resumePlan(filepath.Dir(s.root), e.summary, runtime.GOOS), nil
}

func resumePlan(home string, session Summary, platform string) ResumePlan {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	shell := "Bash / Zsh"
	if platform == "windows" {
		quote = func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
		shell = "PowerShell"
	}
	resume := "codex -C " + quote(session.Project) + " " + strings.TrimPrefix(session.ResumeCommand, "codex ")
	if platform == "windows" {
		env := "$env:CODEX_HOME = " + quote(home) + "; "
		return ResumePlan{
			Command:        env + resume,
			RestartCommand: env + "codex app-server daemon restart; if ($LASTEXITCODE -eq 0) { " + resume + " }",
			Shell:          shell,
		}
	}
	env := "CODEX_HOME=" + quote(home) + " "
	return ResumePlan{
		Command:        env + resume,
		RestartCommand: env + "codex app-server daemon restart && " + env + resume,
		Shell:          shell,
	}
}
