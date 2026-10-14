package deck

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"codex-deck/internal/network"
	"github.com/joho/godotenv"
)

type CLIProxyVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CLIProxyState struct {
	Path      string             `json:"path"`
	Variables []CLIProxyVariable `json:"variables"`
}

type CLIProxyChange struct {
	URL      string             `json:"url"`
	Remove   bool               `json:"remove"`
	Expected []CLIProxyVariable `json:"expected"`
}

type cliEnvStatement struct {
	raw     string
	key     string
	value   string
	prefix  string
	comment string
}

var envAssignment = regexp.MustCompile(`^[ \t]*(?:export[ \t]+)?[A-Za-z0-9_.]+[ \t]*[=:][ \t]*`)

func managedProxyKey(key string) bool {
	return key == "http_proxy" || key == "HTTP_PROXY" || key == "https_proxy" || key == "HTTPS_PROXY"
}

func cliEnvStatements(data []byte) ([]cliEnvStatement, error) {
	if _, err := godotenv.Unmarshal(string(data)); err != nil {
		// dotenv 解析错误可能含其他变量的秘密值，不将原文传给前端。
		return nil, fmt.Errorf("CLI 环境文件格式无效，请检查变量赋值与引号")
	}
	var statements []cliEnvStatement
	var pending strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		pending.WriteString(line)
		values, err := godotenv.Unmarshal(pending.String())
		if err != nil {
			continue // 多行引号值交由 dotenv 库识别完整语句。
		}
		if len(values) > 1 {
			return nil, fmt.Errorf("CLI 环境文件应每行只声明一个变量")
		}
		statement := cliEnvStatement{raw: pending.String()}
		for key, value := range values {
			statement.key, statement.value = key, value
		}
		if managedProxyKey(statement.key) {
			statement.prefix = envAssignment.FindString(statement.raw)
			// 库解析后的值相同才说明 # 位于值外，保留该行注释。
			for i, char := range statement.raw {
				if char != '#' {
					continue
				}
				before, err := godotenv.Unmarshal(statement.raw[:i])
				if err == nil && before[statement.key] == statement.value {
					statement.comment = strings.TrimRight(statement.raw[i:], "\r\n")
					break
				}
			}
		}
		statements = append(statements, statement)
		pending.Reset()
	}
	return statements, nil
}

func (c CLI) proxyFile() ([]byte, []cliEnvStatement, error) {
	data, err := os.ReadFile(filepath.Join(c.Home, ".env"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取 CLI 环境文件失败: %w", err)
	}
	statements, err := cliEnvStatements(data)
	return data, statements, err
}

func (c CLI) proxyState(statements []cliEnvStatement) CLIProxyState {
	state := CLIProxyState{Path: filepath.Join(c.Home, ".env"), Variables: []CLIProxyVariable{}}
	for _, statement := range statements {
		if managedProxyKey(statement.key) {
			state.Variables = append(state.Variables, CLIProxyVariable{Key: statement.key, Value: statement.value})
		}
	}
	return state
}

func (c CLI) ReadProxy() (CLIProxyState, error) {
	_, statements, err := c.proxyFile()
	if err != nil {
		return CLIProxyState{}, err
	}
	return c.proxyState(statements), nil
}

func CLIProxyNetwork(address string) (network.Config, error) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return network.Config{}, fmt.Errorf("CLI 代理地址格式无效")
	}
	config := network.Config{Mode: "custom"}
	if parsed.User != nil {
		config.Username = parsed.User.Username()
		config.Password, _ = parsed.User.Password()
		parsed.User = nil
	}
	config.URL = parsed.String()
	return network.Normalize(config)
}

func (c CLI) SaveProxy(change CLIProxyChange) (CLIProxyState, error) {
	if !change.Remove {
		if _, err := CLIProxyNetwork(change.URL); err != nil {
			return CLIProxyState{}, err
		}
	}
	data, statements, err := c.proxyFile()
	if err != nil {
		return CLIProxyState{}, err
	}
	if !slices.Equal(c.proxyState(statements).Variables, change.Expected) {
		return CLIProxyState{}, fmt.Errorf("CLI 代理配置已被其他程序修改，请重新读取后保存")
	}
	if change.Remove && len(change.Expected) == 0 {
		return c.proxyState(statements), nil
	}
	encoded, err := godotenv.Marshal(map[string]string{"proxy": strings.TrimSpace(change.URL)})
	if err != nil {
		return CLIProxyState{}, err
	}
	_, value, _ := strings.Cut(encoded, "=")
	value = strings.ReplaceAll(value, "$", `\$`)
	ending := "\n"
	if strings.Contains(string(data), "\r\n") {
		ending = "\r\n"
	}
	var result strings.Builder
	found := map[string]bool{}
	for _, statement := range statements {
		if !managedProxyKey(statement.key) {
			result.WriteString(statement.raw)
			continue
		}
		found[strings.ToLower(statement.key)] = true
		if !change.Remove {
			result.WriteString(statement.prefix + value)
			if statement.comment != "" {
				result.WriteString(" " + statement.comment)
			}
		} else if statement.comment != "" {
			result.WriteString(statement.comment)
		}
		if strings.HasSuffix(statement.raw, "\n") {
			result.WriteString(ending)
		}
	}
	if !change.Remove {
		for _, key := range []string{"http_proxy", "https_proxy"} {
			if found[key] {
				continue
			}
			if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
				result.WriteString(ending)
			}
			result.WriteString(key + "=" + value + ending)
		}
	}
	updated := []byte(result.String())
	parsed, err := cliEnvStatements(updated)
	if err != nil {
		return CLIProxyState{}, err
	}
	if err := atomicWrite(filepath.Join(c.Home, ".env"), updated); err != nil {
		return CLIProxyState{}, fmt.Errorf("保存 CLI 代理失败: %w", err)
	}
	return c.proxyState(parsed), nil
}
