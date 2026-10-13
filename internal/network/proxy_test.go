package network

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func readThrough(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCustomProxyAuthenticationAndHotSwitch(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "target.invalid" || r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:password")) {
			t.Error("代理请求地址或认证错误")
		}
		close(started)
		<-finish
		w.Write([]byte("first"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("second")) }))
	defer second.Close()
	router, err := New(Config{Mode: "custom", URL: first.URL, Username: "user", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: router, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	result := make(chan string, 1)
	go func() {
		response, err := client.Get("http://target.invalid/")
		if err != nil {
			result <- err.Error()
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			result <- err.Error()
			return
		}
		result <- string(data)
	}()
	<-started
	route, err := Prepare(Config{Mode: "custom", URL: second.URL})
	if err != nil {
		t.Fatal(err)
	}
	router.Apply(route)
	if got := readThrough(t, client, "http://target.invalid/"); got != "second" {
		t.Fatal("新请求未切换代理", got)
	}
	close(finish)
	if got := <-result; got != "first" {
		t.Fatal("切换代理中断已有请求", got)
	}
}

func TestHTTPSConnectDoesNotForwardProxyCredentials(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("目标服务收到代理认证")
		}
		w.Write([]byte("secure"))
	}))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:secret")) {
			t.Error("CONNECT 未使用代理认证")
		}
		remote, err := net.Dial("tcp", target.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			return
		}
		connection, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			remote.Close()
			t.Error(err)
			return
		}
		defer connection.Close()
		defer remote.Close()
		buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buffered.Flush()
		go io.Copy(remote, buffered)
		io.Copy(connection, remote)
	}))
	defer proxy.Close()
	route, err := Prepare(Config{Mode: "custom", URL: proxy.URL, Username: "user", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	route.transport.(*http.Transport).TLSClientConfig = target.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	router := &Router{}
	router.active.Store(route)
	client := &http.Client{Transport: router, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	if got := readThrough(t, client, target.URL); got != "secure" {
		t.Fatal("TLS 通道失败", got)
	}
}

func TestProxyModesInIsolatedProcess(t *testing.T) {
	if os.Getenv("CODEX_DECK_PROXY_CHILD") != "" {
		router, err := New(Config{Mode: os.Getenv("CODEX_DECK_PROXY_CHILD")})
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Transport: router, Timeout: time.Second}
		defer client.CloseIdleConnections()
		response, err := client.Get("http://target.invalid/")
		if os.Getenv("CODEX_DECK_PROXY_CHILD") == "direct" {
			if err == nil {
				response.Body.Close()
				t.Fatal("直连错误地使用环境代理")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("环境代理失败")
		}
		return
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer proxy.Close()
	for _, mode := range []string{"environment", "direct"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProxyModesInIsolatedProcess$")
		for _, entry := range os.Environ() {
			name, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(name, "HTTP_PROXY") || strings.EqualFold(name, "HTTPS_PROXY") || strings.EqualFold(name, "NO_PROXY") {
				continue
			}
			cmd.Env = append(cmd.Env, entry)
		}
		cmd.Env = append(cmd.Env, "CODEX_DECK_PROXY_CHILD="+mode, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY=", "NO_PROXY=")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, output)
		}
	}
}

func TestInvalidConfigurationAndUnavailableProxy(t *testing.T) {
	for _, config := range []Config{{Mode: "unknown"}, {Mode: "custom", URL: "socks5://localhost:1080"}, {Mode: "custom", URL: "http://user:secret@localhost:1234"}, {Mode: "custom", URL: "http://localhost:70000"}, {Mode: "custom", URL: "http://localhost/?secret=1"}} {
		if _, err := Prepare(config); err == nil {
			t.Fatal("接受无效配置", config.Mode)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	router, err := New(Config{Mode: "custom", URL: "http://" + address, Username: "user", Password: "never-show"})
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("代理失败后静默直连") }))
	defer target.Close()
	_, err = (&http.Client{Transport: router, Timeout: time.Second}).Get(target.URL)
	if err == nil || strings.Contains(err.Error(), "never-show") {
		t.Fatal("错误未暴露或泄露代理密码", err)
	}
}
