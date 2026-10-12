package deck

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 所有令牌均为测试中签发的本地数据，不来自真实账号。
func testToken(t *testing.T, user, account string, expiry time.Time) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": user, "email": "test@example.test", "exp": expiry.Unix(), "https://api.openai.com/auth": map[string]string{"chatgpt_user_id": user, "chatgpt_account_id": account, "chatgpt_plan_type": "plus"}}).SignedString([]byte("local-test-only"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func testAccount(t *testing.T) Account {
	token := testToken(t, "user-1", "workspace-1", time.Now().Add(time.Hour))
	return Account{ID: "record-1", UserID: "user-1", AccountID: "workspace-1", Email: "test@example.test", Plan: "plus", Tokens: Tokens{IDToken: token, AccessToken: token, RefreshToken: "local-refresh", AccountID: "workspace-1", ExpiresAt: time.Now().Add(time.Hour), LastRefresh: time.Now()}}
}

func TestOAuthPKCECallbackAndRefresh(t *testing.T) {
	token := testToken(t, "user-1", "workspace-1", time.Now().Add(time.Hour))
	var login *Login
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Error("错误令牌地址")
		}
		if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("client_id") != OAuthClientID || r.Form.Get("code_verifier") != login.verifier || r.Form.Get("code") != "test-code" || r.Form.Get("redirect_uri") != OAuthRedirect {
				t.Error("授权码参数不匹配")
			}
		} else {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["grant_type"] != "refresh_token" || body["refresh_token"] != "old-refresh" {
				t.Error("刷新参数不匹配")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"id_token": token, "access_token": token, "refresh_token": "new-refresh", "expires_in": 3600, "token_type": "Bearer", "scope": "openid", "extra": "kept"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	o := OAuth{Client: server.Client(), Issuer: server.URL}
	var err error
	login, err = o.startAt(context.Background(), "127.0.0.1:0", OAuthRedirect)
	if err != nil {
		t.Fatal(err)
	}
	defer login.Cancel()
	u, err := url.Parse(login.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" || u.Query().Get("client_id") != OAuthClientID {
		t.Fatal("缺少 PKCE 参数")
	}
	callback := "http://" + login.address + "/auth/callback?code=test-code&state="
	resp, err := http.Get(callback + "bad-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal("未拒绝错误 state")
	}
	resp, err = http.Get(callback + u.Query().Get("state"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	tokens, i, err := o.Wait(login)
	if err != nil {
		t.Fatal(err)
	}
	if i.AccountID != "workspace-1" || tokens.RefreshToken != "new-refresh" {
		t.Fatal("认证结果错误")
	}
	var raw map[string]interface{}
	if err = json.Unmarshal(tokens.OAuthResponse, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["extra"] != "kept" {
		t.Fatal("丢失 OAuth 字段")
	}
	tokens.RefreshToken = "old-refresh"
	tokens, _, err = o.Refresh(context.Background(), tokens)
	if err != nil {
		t.Fatal(err)
	}
	if tokens.RefreshToken != "new-refresh" {
		t.Fatal("未保存新令牌")
	}
}

func TestOAuthCancelAndFailure(t *testing.T) {
	o := OAuth{Client: &http.Client{Timeout: time.Second}, Issuer: "https://auth.openai.com"}
	l, err := o.startAt(context.Background(), "127.0.0.1:0", OAuthRedirect)
	if err != nil {
		t.Fatal(err)
	}
	l.Cancel()
	if _, _, err = o.Wait(l); err == nil {
		t.Fatal("取消应返回错误")
	}
	deadline := time.Now().Add(time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", l.address, 20*time.Millisecond)
		if dialErr != nil {
			break
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("取消未关闭回调监听")
		}
		time.Sleep(time.Millisecond)
	}
	for _, code := range []string{"refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated", "invalid_grant"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": code})
		}))
		o.Issuer = server.URL
		o.Client = server.Client()
		if _, _, err := o.Refresh(context.Background(), Tokens{RefreshToken: "test"}); err == nil {
			t.Error("错误响应应失败")
		}
		server.Close()
	}
}

func TestOAuthRejectsIncompleteAndChangedIdentity(t *testing.T) {
	a := testAccount(t)
	for _, body := range []map[string]interface{}{
		{"access_token": a.Tokens.AccessToken},
		{"id_token": a.Tokens.IDToken, "access_token": testToken(t, "other-user", "workspace-1", time.Now().Add(time.Hour)), "refresh_token": "x"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(body) }))
		o := OAuth{Client: server.Client(), Issuer: server.URL}
		if _, _, err := o.exchange(context.Background(), nil, "application/json", Tokens{}, false); err == nil {
			t.Error("接受不完整或身份不匹配认证")
		}
		server.Close()
	}
}
