package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackendRequestsAndUsage(t *testing.T) {
	a := testAccount(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+a.Tokens.AccessToken || r.Header.Get("ChatGPT-Account-Id") != a.AccountID {
			t.Error("认证请求头错误")
		}
		switch r.URL.Path {
		case "/wham/usage":
			w.Write([]byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":604800,"used_percent":75,"reset_at":2000000000},"secondary_window":{"limit_window_seconds":18000,"used_percent":20}},"rate_limit_reset_credits":{"available_count":0}}`))
		case "/wham/rate-limit-reset-credits":
			w.Write([]byte(`{"available_count":1,"credits":[{"id":"credit-1","reset_type":"codex_rate_limits","status":"available","granted_at":"2026-01-01T00:00:00Z","expires_at":null}]}`))
		case "/wham/rate-limit-reset-credits/consume":
			if r.Method != "POST" {
				t.Error("消费方法错误")
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["redeem_request_id"] != "request-1" || body["credit_id"] != "credit-1" || len(body) != 2 {
				t.Error("消费参数错误")
			}
			w.Write([]byte(`{"code":"already_redeemed","windows_reset":0}`))
		default:
			t.Error("路径错误")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	b := Backend{Client: server.Client(), BaseURL: server.URL}
	u, err := b.Usage(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if u.FiveHour.RemainingPercent != 80 || u.Weekly.RemainingPercent != 25 || u.FiveHour.ResetAt != nil || u.ResetCredits == nil || *u.ResetCredits != 0 {
		t.Fatal("用量转换错误")
	}
	c, err := b.Credits(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if c.AvailableCount != 1 || c.Credits[0].Status != "available" {
		t.Fatal("卡片详情错误")
	}
	result, err := b.Consume(context.Background(), a, "request-1", "credit-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "already_redeemed" {
		t.Fatal("消费结果错误")
	}
}

func TestBackendMissingWindowsAndInvalidResponse(t *testing.T) {
	for _, body := range []string{`{"plan_type":"free"}`, `{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":3600,"used_percent":5}}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		b := Backend{Client: server.Client(), BaseURL: server.URL}
		u, err := b.Usage(context.Background(), testAccount(t))
		if err != nil {
			t.Fatal(err)
		}
		if u.FiveHour != nil || u.Weekly != nil || u.ResetCredits != nil {
			t.Error("缺失字段被当成有效值")
		}
		server.Close()
	}
	for _, body := range []string{`{}`, `<html>blocked</html>`, `{"plan_type":"plus","rate_limit":{"primary_window":{"limit_window_seconds":18000}}}`, `{"plan_type":"plus","account_id":"other"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		b := Backend{Client: server.Client(), BaseURL: server.URL}
		if _, err := b.Usage(context.Background(), testAccount(t)); err == nil {
			t.Error("接受无效响应")
		}
		server.Close()
	}
}

func TestConsumeCodesAndHTTPErrors(t *testing.T) {
	for _, code := range []string{"reset", "already_redeemed", "nothing_to_reset", "no_credit", "future_code"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"code": code})
		}))
		b := Backend{Client: server.Client(), BaseURL: server.URL}
		_, err := b.Consume(context.Background(), testAccount(t), "same-request", "")
		if (err != nil) != (code == "future_code") {
			t.Error("消费结果未准确区分")
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`<html>forbidden</html>`))
	}))
	defer server.Close()
	b := Backend{Client: server.Client(), BaseURL: server.URL}
	if _, err := b.Usage(context.Background(), testAccount(t)); err == nil {
		t.Fatal("HTTP 错误未暴露")
	}
}
