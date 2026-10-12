package deck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPError struct {
	Status  int
	Code    string
	Message string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message) }

func requestJSON(ctx context.Context, client *http.Client, method, endpoint, contentType string, body []byte, headers map[string]string, result interface{}) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-deck/0.1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 HTTP 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := &HTTPError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
		var payload map[string]json.RawMessage
		if json.Unmarshal(b, &payload) == nil {
			var code string
			if json.Unmarshal(payload["error"], &code) == nil {
				e.Code = code
			} else {
				var detail struct {
					Code    string `json:"code"`
					Type    string `json:"type"`
					Message string `json:"message"`
				}
				if json.Unmarshal(payload["error"], &detail) == nil {
					e.Code = detail.Code
					if e.Code == "" {
						e.Code = detail.Type
					}
					if detail.Message != "" {
						e.Message = detail.Message
					}
				}
			}
			var description string
			if json.Unmarshal(payload["error_description"], &description) == nil && description != "" {
				e.Message = description
			}
		}
		// 服务端错误可能包含请求令牌，不能把这些值送入界面。
		for _, secret := range headers {
			if strings.HasPrefix(secret, "Bearer ") {
				e.Message = strings.ReplaceAll(e.Message, strings.TrimPrefix(secret, "Bearer "), "[已隐藏]")
			}
		}
		return nil, e
	}
	if err := json.Unmarshal(b, result); err != nil {
		return nil, fmt.Errorf("HTTP 响应不是有效 JSON: %w", err)
	}
	return b, nil
}
