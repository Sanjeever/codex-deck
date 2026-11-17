package sessions

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareResumeIsReadOnlyAndRevalidatesSession(t *testing.T) {
	s, root := newFixtureService(t)
	const sessionID = "9fb19d9b-42a9-4346-a9d0-27c242de94df"
	path := filepath.Join(root, "rollout.jsonl")
	data := meta(sessionID, "/project") + user("原账号创建的会话")
	fixture(t, path, data)
	list, err := s.List(ListQuery{})
	if err != nil || len(list.Sessions) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	id := list.Sessions[0].ID
	plan, err := s.PrepareResume(id)
	if err != nil || !strings.Contains(plan.Command, sessionID) || plan.Shell == "" {
		t.Fatalf("prepare: %+v %v", plan, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != data {
		t.Fatal("preparing resume changed the session")
	}
	if _, err := s.PrepareResume(path); err == nil {
		t.Fatal("accepted an unenumerated path")
	}
	fixture(t, path, meta("invalid", "/project")+user("replaced"))
	if _, err := s.PrepareResume(id); err == nil {
		t.Fatal("accepted a replaced session with an invalid Codex ID")
	}
	fixture(t, path, meta(sessionID, "")+user("missing cwd"))
	if _, err := s.PrepareResume(id); err == nil {
		t.Fatal("accepted a session without a working directory")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareResume(id); err == nil {
		t.Fatal("accepted a deleted session")
	}
}

type resumeInvocation struct {
	Home string
	Args []string
}

func TestResumeCommandHelper(t *testing.T) {
	if os.Getenv("RESUME_TEST_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	f, err := os.OpenFile(os.Getenv("RESUME_TEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(resumeInvocation{os.Getenv("CODEX_HOME"), args}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if len(args) > 0 && args[0] == "app-server" && os.Getenv("RESUME_TEST_RESTART_FAIL") == "1" {
		os.Exit(17)
	}
	os.Exit(0)
}

func TestResumeCommandsPreserveArgumentsAndStopAfterRestartFailure(t *testing.T) {
	platform := runtime.GOOS
	shells := []string{"bash", "zsh"}
	if platform == "windows" {
		shells = []string{"powershell"}
	}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			shellPath, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s unavailable", shell)
			}
			root := t.TempDir()
			home := filepath.Join(root, "codex ' $variable & `literal`")
			project := filepath.Join(root, "project ' $variable & `literal`")
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			stub := "#!/bin/sh\nexec \"$RESUME_TEST_BINARY\" -test.run=TestResumeCommandHelper -- \"$@\"\n"
			name := "codex"
			if platform == "windows" {
				name = "codex.ps1"
				stub = "& $env:RESUME_TEST_BINARY '-test.run=TestResumeCommandHelper' '--' @args\nexit $LASTEXITCODE\n"
			}
			if err := os.WriteFile(filepath.Join(root, name), []byte(stub), 0700); err != nil {
				t.Fatal(err)
			}
			const sessionID = "9fb19d9b-42a9-4346-a9d0-27c242de94df"
			plan := resumePlan(home, Summary{Project: project, ResumeCommand: "codex resume " + sessionID}, platform)
			for _, scenario := range []struct {
				name    string
				command string
				fail    bool
				calls   int
			}{
				{"resume", plan.Command, false, 1},
				{"restart and resume", plan.RestartCommand, false, 2},
				{"failed restart", plan.RestartCommand, true, 1},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					log := filepath.Join(t.TempDir(), "calls.jsonl")
					args := []string{"-c", scenario.command}
					if platform == "windows" {
						args = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", scenario.command}
					}
					cmd := exec.CommandContext(context.Background(), shellPath, args...)
					cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "CODEX_HOME=wrong-home", "RESUME_TEST_HELPER=1", "RESUME_TEST_BINARY="+binary, "RESUME_TEST_LOG="+log)
					if scenario.fail {
						cmd.Env = append(cmd.Env, "RESUME_TEST_RESTART_FAIL=1")
					}
					output, runErr := cmd.CombinedOutput()
					if runErr != nil && !scenario.fail {
						t.Fatalf("execute: %v\n%s", runErr, output)
					}
					f, err := os.Open(log)
					if err != nil {
						t.Fatalf("log: %v\n%s", err, output)
					}
					defer f.Close()
					var calls []resumeInvocation
					decoder := json.NewDecoder(f)
					for {
						var call resumeInvocation
						if err := decoder.Decode(&call); err == io.EOF {
							break
						} else if err != nil {
							t.Fatal(err)
						}
						calls = append(calls, call)
					}
					if len(calls) != scenario.calls {
						t.Fatalf("calls: %+v\n%s", calls, output)
					}
					for i, call := range calls {
						want := []string{"-C", project, "resume", sessionID}
						if scenario.name != "resume" && i == 0 {
							want = []string{"app-server", "daemon", "restart"}
						}
						if call.Home != home || !reflect.DeepEqual(call.Args, want) {
							t.Fatalf("call: %+v; want home=%q args=%q", call, home, want)
						}
					}
				})
			}
		})
	}
}
