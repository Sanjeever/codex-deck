package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestCLIProxyCreateAndRead(t *testing.T) {
	c := CLI{Home: filepath.Join(t.TempDir(), "custom-codex-home")}
	state, err := c.ReadProxy()
	if err != nil || state.Path != filepath.Join(c.Home, ".env") || len(state.Variables) != 0 {
		t.Fatalf("read: %+v %v", state, err)
	}
	if _, err := os.Stat(state.Path); !os.IsNotExist(err) {
		t.Fatal("读取时创建了文件")
	}
	state, err = c.SaveProxy(CLIProxyChange{URL: "http://127.0.0.1:7897", Expected: state.Variables})
	if err != nil {
		t.Fatal(err)
	}
	values, err := godotenv.Read(state.Path)
	if err != nil || values["http_proxy"] != "http://127.0.0.1:7897" || values["https_proxy"] != values["http_proxy"] {
		t.Fatalf("written proxy: %v %v", values, err)
	}
}

func TestCLIProxyReadsReferencesToEarlierFileVariables(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	path := filepath.Join(c.Home, ".env")
	before := "ADDRESS=http://localhost:7897\nhttp_proxy=${ADDRESS} # 地址复用\nhttps_proxy=${ADDRESS}\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := c.ReadProxy()
	if err != nil || len(state.Variables) != 2 || state.Variables[0].Value != "http://localhost:7897" {
		t.Fatalf("未按文件变量读取代理: %+v %v", state, err)
	}
	if _, err := c.SaveProxy(CLIProxyChange{URL: "http://localhost:7898", Expected: state.Variables}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(after), "ADDRESS=http://localhost:7897\n") || !strings.Contains(string(after), "# 地址复用\n") {
		t.Fatal("改写了其他变量或丢失注释")
	}
}

func TestCLIProxyPreservesOtherContentAndComments(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	path := filepath.Join(c.Home, ".env")
	before := "# 用户设置\r\nAPI_KEY='not-for-frontend' # 私密值\r\nCERT='第一行\r\nhttp_proxy=http://not-a-setting:1234\r\n第三行'\r\nexport HTTP_PROXY = \"http://old:8000\" # 保留说明\r\nhttp_proxy='http://duplicate:8001'\r\nHTTPS_PROXY=http://old:8000\r\nALL_PROXY=socks5://other:1080\r\nNO_PROXY=localhost,127.0.0.1\r\nLAST=value"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := c.ReadProxy()
	if err != nil || len(state.Variables) != 3 {
		t.Fatalf("read variables: %+v %v", state, err)
	}
	encoded, err := json.Marshal(state)
	if err != nil || strings.Contains(string(encoded), "not-for-frontend") || strings.Contains(string(encoded), "not-a-setting") {
		t.Fatal("读取代理泄露了其他变量")
	}
	state, err = c.SaveProxy(CLIProxyChange{URL: "http://localhost:7897", Expected: state.Variables})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(before, `"http://old:8000"`, `"http://localhost:7897"`)
	want = strings.ReplaceAll(want, `'http://duplicate:8001'`, `"http://localhost:7897"`)
	want = strings.ReplaceAll(want, `HTTPS_PROXY=http://old:8000`, `HTTPS_PROXY="http://localhost:7897"`)
	if string(after) != want {
		t.Fatalf("改写了其他内容:\ngot: %s\nwant: %s", after, want)
	}
	if _, err := c.SaveProxy(CLIProxyChange{Remove: true, Expected: state.Variables}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "# 保留说明\r\n") || !strings.Contains(string(after), "ALL_PROXY=socks5://other:1080\r\nNO_PROXY=localhost,127.0.0.1\r\nLAST=value") || !strings.Contains(string(after), "http_proxy=http://not-a-setting:1234") {
		t.Fatal("移除代理改写了其他变量或注释")
	}
	state, err = c.ReadProxy()
	if err != nil || len(state.Variables) != 0 {
		t.Fatal("移除后仍有 HTTP/HTTPS 代理字段")
	}
}

func TestCLIProxyDetectsChangedProxyAndPreservesUnrelatedEdits(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	path := filepath.Join(c.Home, ".env")
	if err := os.WriteFile(path, []byte("http_proxy=http://old:8000\nOTHER=before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := c.ReadProxy()
	if err != nil {
		t.Fatal(err)
	}
	changed := "http_proxy=http://external:8001\nOTHER=after\n"
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveProxy(CLIProxyChange{URL: "http://localhost:7897", Expected: state.Variables}); err == nil {
		t.Fatal("覆盖了外部代理修改")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != changed {
		t.Fatal("冲突后修改了文件")
	}
	state, err = c.ReadProxy()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(changed, "OTHER=after", "OTHER=latest")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveProxy(CLIProxyChange{URL: "http://localhost:7897", Expected: state.Variables}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(after), "OTHER=latest") {
		t.Fatal("保存未保留最新的其他配置")
	}
}

func TestCLIProxyRejectsInvalidContentAndAddress(t *testing.T) {
	c := CLI{Home: t.TempDir()}
	path := filepath.Join(c.Home, ".env")
	before := "API_KEY=\"secret-not-for-error"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadProxy(); err == nil || strings.Contains(err.Error(), "secret-not-for-error") {
		t.Fatal("未报告格式错误或泄露其他秘密值")
	}
	if _, err := c.SaveProxy(CLIProxyChange{URL: "http://localhost:7897"}); err == nil {
		t.Fatal("保存覆盖了无效环境文件")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != before {
		t.Fatal("失败后改写了环境文件")
	}
	for _, address := range []string{"", "socks5://localhost:7897", "http://localhost:99999", "http://localhost:7897/path"} {
		if _, err := CLIProxyNetwork(address); err == nil {
			t.Fatalf("接受无效代理地址: %s", address)
		}
	}
}

func TestCLIProxyKeepsLiteralPasswordAndDoesNotChangeEnvironment(t *testing.T) {
	t.Setenv("http_proxy", "http://inherited:8080")
	c := CLI{Home: t.TempDir()}
	address := "http://user:pa$word@localhost:7897"
	state, err := c.SaveProxy(CLIProxyChange{URL: address})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Variables) != 2 || state.Variables[0].Value != address || os.Getenv("http_proxy") != "http://inherited:8080" {
		t.Fatalf("代理值或进程环境变化: %+v", state)
	}
	config, err := CLIProxyNetwork(address)
	if err != nil || config.Username != "user" || config.Password != "pa$word" || config.URL != "http://localhost:7897" {
		t.Fatalf("代理认证解析失败: %+v %v", config, err)
	}
}
