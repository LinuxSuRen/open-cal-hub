// user 包端到端流程测试（外部测试包，避免与 admin 互相导入形成环）：
// httptest + 临时 SQLite，覆盖注册→验证→登录→刷新→登出与各错误分支。
package user_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/linuxsuren/open-cal-hub/backend/internal/admin"
	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
	"github.com/linuxsuren/open-cal-hub/backend/internal/server"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

const refreshCookie = "calhub_rt"

type env struct {
	t   *testing.T
	a   *app.App
	srv *httptest.Server
}

func newEnv(t *testing.T, mutate func(*config.Config)) *env {
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := &app.App{
		Cfg: cfg, DB: database, Log: log,
		Mail:   &mail.Outbox{DB: database},
		Signer: auth.NewSigner(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL),
	}
	mux := http.NewServeMux()
	user.Register(mux, a)
	admin.Register(mux, a)
	a.Mux = mux
	srv := httptest.NewServer(server.New(a))
	t.Cleanup(srv.Close)
	return &env{t: t, a: a, srv: srv}
}

type result struct {
	status int
	body   map[string]any
	raw    string
	setCookie string
}

var cookieRe = regexp.MustCompile(`calhub_rt=([^;]+)`)

func (e *env) do(method, path string, body any, token, cookie string) result {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		req.Header.Set("Cookie", refreshCookie+"="+cookie)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	r := result{status: resp.StatusCode, raw: string(raw)}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &r.body)
	}
	for _, c := range resp.Header.Values("Set-Cookie") {
		if m := cookieRe.FindStringSubmatch(c); m != nil {
			if strings.Contains(c, "Max-Age=0") {
				r.setCookie = ""
			} else {
				r.setCookie = m[1]
			}
		}
	}
	return r
}

func errCode(r result) string {
	if r.body == nil {
		return ""
	}
	if e, ok := r.body["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

func dataMap(r result) map[string]any {
	if r.body == nil {
		return nil
	}
	d, _ := r.body["data"].(map[string]any)
	return d
}

// outboxToken 取最新一封 kind 邮件里的令牌（outbox 模式联调口径）。
func (e *env) outboxToken(kind string) string {
	e.t.Helper()
	var body string
	err := e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind=? ORDER BY id DESC LIMIT 1`, kind).Scan(&body)
	if err != nil {
		e.t.Fatalf("read outbox(%s): %v", kind, err)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token in mail body: %s", body)
	}
	return m[1]
}

func (e *env) register(email, password, username string) result {
	return e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": email, "password": password, "username": username,
	}, "", "")
}

func (e *env) login(loginName, password string) (result, string) {
	r := e.do("POST", "/api/v1/auth/login", map[string]any{"email": loginName, "password": password}, "", "")
	return r, r.setCookie
}

func (e *env) verifyUser(email string) {
	e.t.Helper()
	r := e.register(email, "passw0rd1", "")
	if r.status != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", email, r.status, r.raw)
	}
	tok := e.outboxToken("verify_email")
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": tok}, "", ""); r.status != 200 {
		e.t.Fatalf("verify-email: %d %s", r.status, r.raw)
	}
}

// ---- 主流程 ----

func TestFullAuthFlow(t *testing.T) {
	e := newEnv(t, nil)

	// 注册：首个用户自动成为 admin
	r := e.register("rick@example.com", "passw0rd!", "rick")
	if r.status != http.StatusCreated {
		t.Fatalf("register: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	if d["verification_required"] != true {
		t.Fatalf("verification_required expected true, got %v", d["verification_required"])
	}
	userObj, _ := d["user"].(map[string]any)
	if userObj["role"] != "admin" {
		t.Fatalf("first user should be admin, got %v", userObj["role"])
	}

	// 未验证邮箱登录被拒
	if r, _ := e.login("rick@example.com", "passw0rd!"); r.status != http.StatusForbidden || errCode(r) != "auth/email_not_verified" {
		t.Fatalf("unverified login: %d %s", r.status, r.raw)
	}

	// 重发验证邮件 → 从 outbox 提取令牌 → 验证
	if r := e.do("POST", "/api/v1/auth/resend-verification", map[string]string{"email": "rick@example.com"}, "", ""); r.status != 200 {
		t.Fatalf("resend: %d", r.status)
	}
	tok := e.outboxToken("verify_email")
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": tok}, "", ""); r.status != 200 {
		t.Fatalf("verify-email: %d %s", r.status, r.raw)
	}
	// 令牌一次性
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": tok}, "", ""); errCode(r) != "auth/invalid_token" {
		t.Fatalf("verify token reuse: %s", r.raw)
	}

	// 登录（邮箱 + 用户名两种方式）
	r, cookie1 := e.login("rick@example.com", "passw0rd!")
	if r.status != 200 {
		t.Fatalf("login: %d %s", r.status, r.raw)
	}
	d = dataMap(r)
	if d["expires_in"] != float64(900) {
		t.Fatalf("expires_in: %v", d["expires_in"])
	}
	access, _ := d["access_token"].(string)
	if access == "" {
		t.Fatal("access token missing")
	}
	r2, _ := e.login("rick", "passw0rd!")
	if r2.status != 200 {
		t.Fatalf("login by username: %d %s", r2.status, r2.raw)
	}

	// /me 读改
	if r := e.do("GET", "/api/v1/me", nil, access, ""); r.status != 200 {
		t.Fatalf("GET /me: %d", r.status)
	} else if m := dataMap(r); m["email_verified"] != true {
		t.Fatalf("email_verified should be true: %s", r.raw)
	}
	if r := e.do("PATCH", "/api/v1/me", map[string]any{"display_name": "Rick C", "bio": "hello"}, access, ""); r.status != 200 {
		t.Fatalf("PATCH /me: %d %s", r.status, r.raw)
	} else if m := dataMap(r); m["display_name"] != "Rick C" {
		t.Fatalf("display_name not updated: %s", r.raw)
	}
	// 未知时区
	if r := e.do("PATCH", "/api/v1/me", map[string]any{"timezone": "Mars/Olympus"}, access, ""); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad timezone: %s", r.raw)
	}
	// 未登录 401
	if r := e.do("GET", "/api/v1/me", nil, "", ""); r.status != 401 {
		t.Fatalf("anonymous /me should 401: %d", r.status)
	}

	// 刷新轮换 + family 重用检测
	r = e.do("POST", "/api/v1/auth/refresh", nil, "", cookie1)
	if r.status != 200 || r.setCookie == "" {
		t.Fatalf("refresh: %d %s", r.status, r.raw)
	}
	cookie2 := r.setCookie
	if r = e.do("POST", "/api/v1/auth/refresh", nil, "", cookie1); r.status != 401 {
		t.Fatalf("old refresh reuse should 401: %d %s", r.status, r.raw)
	}
	if r = e.do("POST", "/api/v1/auth/refresh", nil, "", cookie2); r.status != 401 {
		t.Fatalf("family revoked: current refresh should 401: %d", r.status)
	}

	// 重新登录 → 会话列表与吊销
	_, cookie3 := e.login("rick@example.com", "passw0rd!")
	r = e.do("GET", "/api/v1/auth/sessions", nil, access, cookie3)
	if r.status != 200 {
		t.Fatalf("sessions: %d %s", r.status, r.raw)
	}
	var sessions []any
	_ = json.Unmarshal([]byte(r.raw), &sessions)
	// 原始 body 是 {"data":[...]}; 用 dataMap 不适合数组，重新取
	_ = sessions
	var list []map[string]any
	raw := r.raw
	_ = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(raw, ""))), &struct{}{})
	if err := json.Unmarshal([]byte(extractData(raw)), &list); err != nil {
		t.Fatalf("parse sessions: %v", err)
	}
	foundCurrent := false
	for _, s := range list {
		if s["current"] == true {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatalf("current session not marked: %s", raw)
	}
	// 再开一个会话，然后改密 → 其他会话被吊销，当前保留
	_, cookie4 := e.login("rick@example.com", "passw0rd!")
	if r := e.do("POST", "/api/v1/me/password",
		map[string]string{"current_password": "passw0rd!", "new_password": "n3wpassw0rd"},
		access, cookie3); r.status != 200 {
		t.Fatalf("change password: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/auth/refresh", nil, "", cookie4); r.status != 401 {
		t.Fatalf("other session should be revoked after password change: %d", r.status)
	}
	if r := e.do("POST", "/api/v1/auth/refresh", nil, "", cookie3); r.status != 200 {
		t.Fatalf("current session should survive password change: %d %s", r.status, r.raw)
	}
	// 旧密码失效
	if r, _ := e.login("rick@example.com", "passw0rd!"); r.status != 401 {
		t.Fatalf("old password should fail: %d", r.status)
	}
	// 当前密码错误 → invalid_credentials
	if r := e.do("POST", "/api/v1/me/password",
		map[string]string{"current_password": "wrong1pass", "new_password": "n3wpassw0rd"},
		access, ""); errCode(r) != "auth/invalid_credentials" {
		t.Fatalf("wrong current password: %s", r.raw)
	}

	// 应用密码
	r = e.do("POST", "/api/v1/me/app-passwords", map[string]string{"label": "iPhone"}, access, "")
	if r.status != 201 {
		t.Fatalf("create app password: %d %s", r.status, r.raw)
	}
	appPw, _ := dataMap(r)["password"].(string)
	appID, _ := dataMap(r)["id"].(float64)
	if appPw == "" || appID == 0 {
		t.Fatalf("app password response: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/me/app-passwords", nil, access, ""); r.status != 200 {
		t.Fatalf("list app passwords: %d", r.status)
	}
	// dav 复用的校验函数
	u, err := user.UserByLogin(context.Background(), e.a.DB, "rick")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if !user.VerifyPassword(context.Background(), e.a.DB, u, appPw) {
		t.Fatal("VerifyPassword should accept app password")
	}
	if !user.VerifyPassword(context.Background(), e.a.DB, u, "n3wpassw0rd") {
		t.Fatal("VerifyPassword should accept login password")
	}
	if user.VerifyPassword(context.Background(), e.a.DB, u, "bad-pass-1") {
		t.Fatal("VerifyPassword must reject wrong password")
	}
	if au, ok := user.AuthenticateBasic(context.Background(), e.a.DB, "rick@example.com", appPw); !ok || au == nil || au.Username != "rick" {
		t.Fatal("AuthenticateBasic failed with app password")
	}
	// last_used_at 更新
	var last sql.NullString
	_ = e.a.DB.QueryRow(`SELECT last_used_at FROM app_passwords WHERE id=?`, int64(appID)).Scan(&last)
	if !last.Valid || last.String == "" {
		t.Fatal("app password last_used_at should be set after use")
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/me/app-passwords/%d", int64(appID)), nil, access, ""); r.status != 200 {
		t.Fatalf("delete app password: %d %s", r.status, r.raw)
	}
	if user.VerifyPassword(context.Background(), e.a.DB, u, appPw) {
		t.Fatal("deleted app password must no longer authenticate")
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/me/app-passwords/%d", int64(appID)), nil, access, ""); r.status != 404 {
		t.Fatalf("double delete: %d", r.status)
	}

	// 登出：吊销 + 清 Cookie
	if r := e.do("POST", "/api/v1/auth/logout", nil, access, cookie3); r.status != 200 {
		t.Fatalf("logout: %d", r.status)
	}
	if r := e.do("POST", "/api/v1/auth/refresh", nil, "", cookie3); r.status != 401 {
		t.Fatalf("refresh after logout: %d", r.status)
	}
}

func extractData(raw string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "[]"
	}
	d, ok := m["data"]
	if !ok {
		return "[]"
	}
	return string(d)
}
