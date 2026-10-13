package network

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	Mode     string `json:"mode"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func Normalize(config Config) (Config, error) {
	if config.Mode == "" {
		config.Mode = "environment"
	}
	switch config.Mode {
	case "environment", "direct":
		return Config{Mode: config.Mode}, nil
	case "custom":
		config.URL = strings.TrimSpace(config.URL)
		address, err := url.Parse(config.URL)
		if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Hostname() == "" || address.User != nil || (address.Path != "" && address.Path != "/") || address.RawQuery != "" || address.Fragment != "" {
			return Config{}, fmt.Errorf("代理地址应为 http://主机:端口 或 https://主机:端口，认证信息请单独填写")
		}
		if address.Port() != "" {
			port, err := strconv.Atoi(address.Port())
			if err != nil || port < 1 || port > 65535 {
				return Config{}, fmt.Errorf("代理端口应为 1–65535")
			}
		}
		if config.Password != "" && config.Username == "" {
			return Config{}, fmt.Errorf("使用代理密码时请填写用户名")
		}
		return config, nil
	default:
		return Config{}, fmt.Errorf("不支持的代理模式")
	}
}

var transportTemplate = http.DefaultTransport.(*http.Transport).Clone()

type Route struct {
	transport http.RoundTripper
	config    Config
}
type Router struct{ active atomic.Pointer[Route] }

func Prepare(config Config) (*Route, error) {
	config, err := Normalize(config)
	if err != nil {
		return nil, err
	}
	if config.Mode == "environment" {
		return &Route{transport: http.DefaultTransport, config: config}, nil
	}
	transport := transportTemplate.Clone()
	transport.Proxy = nil
	if config.Mode == "custom" {
		address, _ := url.Parse(config.URL)
		if config.Username != "" {
			address.User = url.UserPassword(config.Username, config.Password)
		}
		transport.Proxy = http.ProxyURL(address)
	}
	return &Route{transport: transport, config: config}, nil
}

func New(config Config) (*Router, error) {
	route, err := Prepare(config)
	if err != nil {
		return nil, err
	}
	router := &Router{}
	router.active.Store(route)
	return router, nil
}

// 已发出的请求保留旧传输，新请求使用保存后的路由，不修改正在使用的 Transport。
func (r *Router) Apply(route *Route) {
	previous := r.active.Swap(route)
	if closer, ok := previous.transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (r *Router) RoundTrip(request *http.Request) (*http.Response, error) {
	route := r.active.Load()
	response, err := route.transport.RoundTrip(request)
	if err != nil && route.config.Password != "" {
		text := strings.ReplaceAll(err.Error(), route.config.Password, "[已隐藏]")
		text = strings.ReplaceAll(text, url.QueryEscape(route.config.Password), "[已隐藏]")
		return response, &proxyError{cause: err, text: text}
	}
	return response, err
}

type proxyError struct {
	cause error
	text  string
}

func (e *proxyError) Error() string { return e.text }
func (e *proxyError) Unwrap() error { return e.cause }

type contextKey struct{}

func WithRouter(ctx context.Context, router *Router) context.Context {
	return context.WithValue(ctx, contextKey{}, router)
}
func FromContext(ctx context.Context) http.RoundTripper {
	if router, ok := ctx.Value(contextKey{}).(*Router); ok {
		return router
	}
	return http.DefaultTransport
}

func Probe(ctx context.Context, config Config) error {
	route, err := Prepare(config)
	if err != nil {
		return err
	}
	router := &Router{}
	router.active.Store(route)
	client := &http.Client{Transport: router, Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Codex-Deck")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("连接测试失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("连接测试失败: HTTP %d", response.StatusCode)
	}
	return nil
}

func (r *Router) CloseIdleConnections() {
	if closer, ok := r.active.Load().transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
