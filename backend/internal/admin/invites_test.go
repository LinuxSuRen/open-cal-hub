// invite-codes 响应形状锁定（契约 §6）：POST 返回 {"codes":[...]} 对象包裹，GET 列表裸数组。
package admin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
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

type inviteEnv struct {
	t   *testing.T
	a   *app.App
	srv *httptest.Server
}

func newInviteEnv(t *testing.T) *inviteEnv {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Server.BaseURL = "http://calhub.test"
	cfg.Auth.JWTSecret = "unit-test-secret-0123456789"
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
	admin.Register(mux, a)
	a.Mux = mux
	srv := httptest.NewServer(server.New(a))
	t.Cleanup(srv.Close)
	return &inviteEnv{t: t, a: a, srv: srv}
}

func (e *inviteEnv) do(method, path string, body any, token string) (int, map[string]any, string) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return resp.StatusCode, m, string(raw)
}

func (e *inviteEnv) adminToken() string {
	e.t.Helper()
	st, _, raw := e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": "admin@example.com", "password": "passw0rd!", "username": "adminrick",
	}, "")
	if st != http.StatusCreated {
		e.t.Fatalf("register: %d %s", st, raw)
	}
	var body string
	_ = e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='verify_email' ORDER BY id DESC LIMIT 1`).Scan(&body)
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if st, _, raw = e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": m[1]}, ""); st != 200 {
		e.t.Fatalf("verify: %d %s", st, raw)
	}
	st, m2, raw := e.do("POST", "/api/v1/auth/login", map[string]string{
		"email": "admin@example.com", "password": "passw0rd!",
	}, "")
	if st != 200 {
		e.t.Fatalf("login: %d %s", st, raw)
	}
	d, _ := m2["data"].(map[string]any)
	return d["access_token"].(string)
}

func TestInviteCodesResponseShape(t *testing.T) {
	e := newInviteEnv(t)
	tok := e.adminToken()

	// POST → {"data":{"codes":[...]}}（对象包裹）
	st, m, raw := e.do("POST", "/api/v1/admin/invite-codes", map[string]any{
		"count": 3, "expires_in_hours": 48,
	}, tok)
	if st != http.StatusCreated {
		t.Fatalf("create codes: %d %s", st, raw)
	}
	d, _ := m["data"].(map[string]any)
	if d == nil {
		t.Fatalf("data 应为对象包裹: %s", raw)
	}
	codes, _ := d["codes"].([]any)
	if len(codes) != 3 {
		t.Fatalf("codes 数量: %s", raw)
	}
	codeRe := regexp.MustCompile(`^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$`)
	for _, c := range codes {
		if s, ok := c.(string); !ok || !codeRe.MatchString(s) {
			t.Fatalf("code 形如 7XKQ-4MHB-9WZT: %v", c)
		}
	}

	// GET → 裸数组，含 code/created_at/expires_at/used_at
	st, m, raw = e.do("GET", "/api/v1/admin/invite-codes", nil, tok)
	if st != 200 {
		t.Fatalf("list codes: %d %s", st, raw)
	}
	list, _ := m["data"].([]any)
	if len(list) != 3 {
		t.Fatalf("列表应裸数组 3 条: %s", raw)
	}
	first, _ := list[0].(map[string]any)
	if first["code"] == nil || first["created_at"] == nil {
		t.Fatalf("列表字段: %s", raw)
	}

	// DELETE 已创建的 code → 200；重复删除 404
	del := codes[0].(string)
	if st, _, raw = e.do("DELETE", fmt.Sprintf("/api/v1/admin/invite-codes/%s", del), nil, tok); st != 200 {
		t.Fatalf("delete code: %d %s", st, raw)
	}
	if st, _, _ = e.do("DELETE", fmt.Sprintf("/api/v1/admin/invite-codes/%s", del), nil, tok); st != 404 {
		t.Fatalf("double delete: %d", st)
	}
}
