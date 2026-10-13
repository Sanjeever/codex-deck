package autostart

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"
	"howett.net/plist"
)

type launchAgent struct {
	Label            string
	ProgramArguments []string
	RunAtLoad        bool
	Disabled         bool `plist:",omitempty"`
}

func (m *Manager) plistEnabled() (bool, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var job launchAgent
	if _, err := plist.Unmarshal(data, &job); err != nil {
		return false, err
	}
	if job.Label != label || len(job.ProgramArguments) == 0 {
		return false, fmt.Errorf("自启动 LaunchAgent 缺少正确的标识或启动命令")
	}
	return job.RunAtLoad && !job.Disabled, nil
}

func (m *Manager) setPlist(enabled bool) error {
	if !enabled {
		return removeRegistration(m.path)
	}
	data, err := plist.Marshal(launchAgent{Label: label, ProgramArguments: []string{m.executable, Flag}, RunAtLoad: true}, plist.XMLFormat)
	if err != nil {
		return err
	}
	return writeRegistration(m.path, data)
}

var desktopOptions = ini.LoadOptions{IgnoreInlineComment: true, PreserveSurroundedQuote: true, KeyValueDelimiters: "="}

func (m *Manager) desktopEnabled() (bool, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file, err := ini.LoadSources(desktopOptions, data)
	if err != nil {
		return false, err
	}
	section, err := file.GetSection("Desktop Entry")
	if err != nil {
		return false, err
	}
	if section.Key("Type").String() != "Application" || section.Key("Exec").String() == "" {
		return false, fmt.Errorf("自启动桌面条目缺少应用类型或启动命令")
	}
	for _, name := range []string{"Hidden", "X-GNOME-Autostart-enabled"} {
		if !section.HasKey(name) {
			continue
		}
		value, err := section.Key(name).Bool()
		if err != nil {
			return false, err
		}
		if (name == "Hidden" && value) || (name == "X-GNOME-Autostart-enabled" && !value) {
			return false, nil
		}
	}
	return true, nil
}

func (m *Manager) setDesktop(enabled bool) error {
	if !enabled {
		return removeRegistration(m.path)
	}
	if strings.Contains(m.executable, "=") {
		return fmt.Errorf("桌面自启动规范不支持程序路径中的等号")
	}
	file := ini.Empty(desktopOptions)
	section := file.Section("Desktop Entry")
	section.Key("Type").SetValue("Application")
	section.Key("Name").SetValue("Codex Deck")
	section.Key("Terminal").SetValue("false")
	var data bytes.Buffer
	if _, err := file.WriteTo(&data); err != nil {
		return err
	}
	// Exec 使用桌面条目的双层转义，避免 INI 库重新包裹启动命令。
	argument := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`, "%", "%%").Replace(m.executable)
	command := `"` + argument + `" ` + Flag
	command = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(command)
	fmt.Fprintln(&data, "Exec="+command)
	return writeRegistration(m.path, data.Bytes())
}
