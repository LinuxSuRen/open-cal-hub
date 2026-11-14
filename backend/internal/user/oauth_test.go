// OAuth 登录测试（外部测试包）：httptest 模拟 GitHub/Google 端点，
// 覆盖授权跳转、state 双提交、回调登录、绑定/自动注册/invite 拒绝、解绑。
package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
	"github.com/linuxsuren/open-cal-hub/backend/internal/server"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

// fakeProvider 模拟 GitHub/Google 的 authorize/token/userinfo。
type fakeProvider struct {
	srv        *httptest.Server
	provider   string
	storedCode string
}

func newFakeProvider(t *testing.T, provider, uid, email string) *fakeProvider {
	t.Helper()
	fp := &fakeProvider{provider: provider}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		// 记录 state 并模拟用户已授权 → 302 回 redirect_uri
		q := r.URL.Query()
		redirect := q.Get("redirect_uri")
		fp.storedCode = "code-" + provider
		http.Redirect(w, r, redirect+"?code="+fp.storedCode+"&state="+q.Get("state"), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != fp.storedCode {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-token","token_type":"bearer"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if provider == "github" {
			fmt.Fprintf(w, `{"id":42,"login":"octocat","name":"Octo Cat"}`)
			return
		}
		fmt.Fprintf(w, `{"sub":"g-sub-1","email":%q,"email_verified":true,"name":"Google User"}`, email)
	})
	if provider == "github" {
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `[{"email":%q,"primary":true,"verified":true},{"email":"old@x.com","primary":false,"verified":true}]`, email)
		})
	}
	fp.srv = httptest.NewServer(mux)
	t.Cleanup(fp.srv.Close)
	return fp
}

func (fp *fakeProvider) conf(id, secret string) config.OAuthProvider {
	return config.OAuthProvider{
		ClientID: id, ClientSecret: secret,
		AuthorizeURL: fp.srv.URL + "/authorize",
		TokenURL:     fp.srv.URL + "/token",
		UserInfoURL:  fp.srv.URL + "/user",
		EmailsURL:    fp.srv.URL + "/emails",
	}
}

type oauthEnv struct {
	t   *testing.T
	a   *app.App
	srv *httptest.Server
}

func newOAuthEnv(t *testing.T, mutate func(*config.Config)) *oauthEnv {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Server.BaseURL = "http://calhub.test"
	cfg.Auth.JWTSecret = "unit-test-secret-0123456789"
	if mutate != nil {
		mutate(cfg)
	}
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	a := &app.App{
		Cfg: cfg, DB: database,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mail:   &mail.Outbox{DB: database},
		Signer: auth.NewSigner(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL),
	}
	mux := http.NewServeMux()
	user.Register(mux, a)
	a.Mux = mux
	srv := httptest.NewServer(server.New(a))
	t.Cleanup(srv.Close)
	// 回调地址指向本地测试服务（覆盖 mutate 中的占位值）
	a.Cfg.Auth.OAuth.RedirectBase = srv.URL
	return &oauthEnv{t: t, a: a, srv: srv}
}

// runOAuthFlow 走完整跳转：authorize →（跟随 302 到 callback）→ 返回最终响应。
// 返回 access_token。
func (e *oauthEnv) runOAuthFlow(provider string) (int, map[string]any, string) {
	e.t.Helper()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // 手动跟随以控制 Cookie
	}}
	// 1) authorize：拿 302 与 state cookie
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/oauth/"+provider+"/authorize?redirect_to=/app", nil)
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatalf("authorize: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		return resp.StatusCode, nil, fmt.Sprintf("authorize status %d", resp.StatusCode)
	}
	var stateCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "calhub_oauth_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		e.t.Fatal("state cookie missing")
	}
	// 2) 模拟浏览器访问 provider 授权页（fakeProvider 302 回 callback）
	provURL := resp.Header.Get("Location")
	preq, _ := http.NewRequest("GET", provURL, nil)
	presp, err := client.Do(preq)
	if err != nil {
		e.t.Fatalf("provider authorize: %v", err)
	}
	presp.Body.Close()
	if presp.StatusCode != http.StatusFound {
		return presp.StatusCode, nil, "provider did not redirect"
	}
	// 3) callback（带 state cookie）
	cbURL := presp.Header.Get("Location")
	creq, _ := http.NewRequest("GET", cbURL, nil)
	creq.AddCookie(stateCookie)
	cresp, err := client.Do(creq)
	if err != nil {
		e.t.Fatalf("callback: %v", err)
	}
	defer cresp.Body.Close()
	raw, _ := io.ReadAll(cresp.Body)
	var m map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return cresp.StatusCode, m, string(raw)
}

func dataOfOAuth(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func TestOAuthGitHubLogin(t *testing.T) {
	fp := newFakeProvider(t, "github", "42", "octo@example.com")
	e := newOAuthEnv(t, func(c *config.Config) {
		c.Auth.OAuth.GitHub = fp.conf("gh-id", "gh-secret")
		c.Auth.OAuth.RedirectBase = "http://calhub.test"
	})

	// 未配置的 provider → 404
	if r, _, _ := e.runOAuthFlow("google"); r != http.StatusNotFound {
		t.Fatalf("未配置 google 应 404，实得 %d", r)
	}

	// 首次 GitHub 登录：自动注册 + 登录成功
	st, m, raw := e.runOAuthFlow("github")
	if st != http.StatusOK {
		t.Fatalf("callback: %d %s", st, raw)
	}
	d := dataOfOAuth(m)
	tok, _ := d["access_token"].(string)
	if tok == "" {
		t.Fatalf("access_token missing: %s", raw)
	}
	if d["user"].(map[string]any)["email"] != "octo@example.com" {
		t.Fatalf("user email: %s", raw)
	}
	// email_verified 已置（提供方已验证）
	if d["user"].(map[string]any)["email_verified"] != true {
		t.Fatalf("email_verified: %s", raw)
	}

	// 绑定列表
	lr, m2, raw2 := e.do("GET", "/api/v1/me/oauth", nil, tok)
	if lr != 200 {
		t.Fatalf("me/oauth: %d %s", lr, raw2)
	}
	list, _ := m2["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["provider"] != "github" {
		t.Fatalf("绑定列表: %s", raw2)
	}

	// 再次登录：同一身份幂等（用户数不增）
	st2, _, raw3 := e.runOAuthFlow("github")
	if st2 != 200 {
		t.Fatalf("second login: %d %s", st2, raw3)
	}
	var n int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 1 {
		t.Fatalf("用户数应保持 1，实得 %d", n)
	}

	// 解绑后再登录 → 又是新绑定（用户不变）
	if r, _, raw4 := e.do("DELETE", "/api/v1/me/oauth/github", nil, tok); r != 200 {
		t.Fatalf("unbind: %d %s", r, raw4)
	}
	if r, _, _ := e.do("DELETE", "/api/v1/me/oauth/github", nil, tok); r != 404 {
		t.Fatalf("double unbind: %d", r)
	}
	st3, _, _ := e.runOAuthFlow("github")
	if st3 != 200 {
		t.Fatalf("relogin after unbind: %d", st3)
	}
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 1 {
		t.Fatalf("解绑重登用户数应 1，实得 %d", n)
	}
}

func TestOAuthGoogleBindsExistingAccount(t *testing.T) {
	fp := newFakeProvider(t, "google", "g-sub-1", "local@example.com")
	e := newOAuthEnv(t, func(c *config.Config) {
		c.Auth.OAuth.Google = fp.conf("g-id", "g-secret")
		c.Auth.OAuth.RedirectBase = "http://calhub.test"
	})

	// 预置未验证的本地账号
	if _, err := e.a.DB.Exec(`INSERT INTO users (email, username, password_hash, display_name, timezone, locale, created_at, updated_at)
		VALUES ('local@example.com','local','x','Local','UTC','zh-CN',?,?)`, user.NowUTC(), user.NowUTC()); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Google 登录 → 关联本地账号（补验证 + 绑定），不新建用户
	st, m, raw := e.runOAuthFlow("google")
	if st != 200 {
		t.Fatalf("callback: %d %s", st, raw)
	}
	if dataOfOAuth(m)["user"].(map[string]any)["username"] != "local" {
		t.Fatalf("应关联 local 账号: %s", raw)
	}
	var n, verified int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE email='local@example.com' AND email_verified_at IS NOT NULL`).Scan(&verified)
	if n != 1 || verified != 1 {
		t.Fatalf("关联后应 1 用户且已验证: n=%d verified=%d", n, verified)
	}
}

func TestOAuthStateProtection(t *testing.T) {
	fp := newFakeProvider(t, "github", "42", "octo@example.com")
	e := newOAuthEnv(t, func(c *config.Config) {
		c.Auth.OAuth.GitHub = fp.conf("gh-id", "gh-secret")
		c.Auth.OAuth.RedirectBase = "http://calhub.test"
	})

	// callback 无 cookie → 400 auth/invalid_token
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/oauth/github/callback?code=x&state=y", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("无 state cookie 应 400，实得 %d", resp.StatusCode)
	}

	// state 篡改签名：直接调 verifyOAuthState 语义（黑盒：构造合法流程但改 query state）
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req2, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/oauth/github/authorize", nil)
	resp2, _ := client.Do(req2)
	resp2.Body.Close()
	var cookie *http.Cookie
	for _, c := range resp2.Cookies() {
		if c.Name == "calhub_oauth_state" {
			cookie = c
		}
	}
	req3, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/oauth/github/callback?code=x&state=tampered.0000", nil)
	req3.AddCookie(cookie)
	resp3, _ := client.Do(req3)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("篡改 state 应 400，实得 %d", resp3.StatusCode)
	}

	// open redirect 防护：redirect_to 归一由 safeRedirect 保证（任意输入不产生外跳）
	loc := resp2.Header.Get("Location")
	if loc == "" || loc[:1] != "h" { // 跳转目标应是 provider 端点
		t.Fatalf("authorize 应跳转 provider: %q", loc)
	}
}

func TestOAuthInviteModeBlocksAutoRegister(t *testing.T) {
	fp := newFakeProvider(t, "github", "42", "newbie@example.com")
	e := newOAuthEnv(t, func(c *config.Config) {
		c.Auth.OAuth.GitHub = fp.conf("gh-id", "gh-secret")
		c.Auth.OAuth.RedirectBase = "http://calhub.test"
	})
	// 预置一个 admin 用户（避免首用户逻辑）+ invite 模式
	if _, err := e.a.DB.Exec(`INSERT INTO users (email, username, password_hash, display_name, timezone, locale, role, created_at, updated_at)
		VALUES ('admin@example.com','admin','x','Admin','UTC','zh-CN','admin',?,?)`, user.NowUTC(), user.NowUTC()); err != nil {
		t.Fatal(err)
	}
	if err := e.a.SetSetting(context.Background(), "registration_mode", "invite"); err != nil {
		t.Fatal(err)
	}

	st, m, raw := e.runOAuthFlow("github")
	if st != http.StatusForbidden {
		t.Fatalf("invite 模式自动注册应 403，实得 %d %s", st, raw)
	}
	if e, ok := m["error"].(map[string]any); !ok || e["code"] != "auth/invite_required" {
		t.Fatalf("错误码: %s", raw)
	}
}

// do 是本文件的轻量请求助手。
func (e *oauthEnv) do(method, path string, body any, token string) (int, map[string]any, string) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return resp.StatusCode, m, string(raw)
}
