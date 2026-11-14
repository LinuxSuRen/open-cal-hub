// admin 包专项测试（外部测试包）：stats 汇总、用户管理自保护、
// settings 白名单与审计日志写入断言。
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

type env struct {
	t   *testing.T
	a   *app.App
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
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
}

func (e *env) do(method, path string, body any, token string) result {
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

// newUser 注册并验证用户，返回 (token, id)。首用户自动 admin。
func (e *env) newUser(username string) (string, int64) {
	e.t.Helper()
	r := e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": username + "@example.com", "password": "passw0rd!", "username": username,
	}, "")
	if r.status != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", username, r.status, r.raw)
	}
	uid := int64(dataMap(r)["user"].(map[string]any)["id"].(float64))
	var body string
	if err := e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='verify_email' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		e.t.Fatalf("outbox: %v", err)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token: %s", body)
	}
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": m[1]}, ""); r.status != 200 {
		e.t.Fatalf("verify %s: %d %s", username, r.status, r.raw)
	}
	r = e.do("POST", "/api/v1/auth/login", map[string]string{
		"email": username + "@example.com", "password": "passw0rd!",
	}, "")
	if r.status != 200 {
		e.t.Fatalf("login %s: %d %s", username, r.status, r.raw)
	}
	return dataMap(r)["access_token"].(string), uid
}

// ---- stats ----

func TestAdminStats(t *testing.T) {
	e := newEnv(t)
	adminTok, _ := e.newUser("rick") // 首用户=admin
	bobTok, _ := e.newUser("bob")

	// 非 admin → 403
	if r := e.do("GET", "/api/v1/admin/stats", nil, bobTok); r.status != http.StatusForbidden {
		t.Fatalf("非 admin 应 403: %d", r.status)
	}

	// 造数据：bob 的日历 + 事件 + 一封 pending 出箱邮件（注册邮件已产生）
	now := "2026-12-05T10:00:00Z"
	if _, err := e.a.DB.Exec(`INSERT INTO calendars (owner_id, name, description, color, timezone, is_default, created_at, updated_at)
		SELECT id, '日历', '', '#000000', 'UTC', 1, ?, ? FROM users WHERE username='bob'`, now, now); err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if _, err := e.a.DB.Exec(`INSERT INTO events (calendar_id, uid, data, summary, location, dtstart, dtend, tzid, all_day, rrule, reminders, status, sequence, etag, created_at, updated_at)
		SELECT id, 'stat@t', 'BEGIN:VCALENDAR', '事件', '', '2026-12-07T01:00:00Z', '2026-12-07T02:00:00Z', 'UTC', 0, '', '[]', 'confirmed', 0, 'e', ?, ?
		FROM calendars WHERE name='日历'`, now, now); err != nil {
		t.Fatalf("event: %v", err)
	}
	// outbox 模式发出的邮件状态为 sent；补一封待发送邮件验证 pending 统计
	if _, err := e.a.DB.Exec(`INSERT INTO mail_outbox (to_addr, subject, body_text, kind, status, created_at)
		VALUES ('x@example.com', '待发', 'body', 'custom', 'pending', ?)`, now); err != nil {
		t.Fatalf("outbox: %v", err)
	}

	r := e.do("GET", "/api/v1/admin/stats", nil, adminTok)
	if r.status != 200 {
		t.Fatalf("stats: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	for key, want := range map[string]float64{
		"users": 2, "calendars": 1, "events": 1, "bookings_30d": 0,
	} {
		if d[key] != want {
			t.Fatalf("stats[%s] 期望 %v 实得 %v", key, want, d[key])
		}
	}
	if d["pending_outbox"].(float64) != 1 {
		t.Fatalf("pending_outbox 应为 1: %s", r.raw)
	}
	if _, ok := d["active_users_30d"]; !ok {
		t.Fatalf("缺少 active_users_30d: %s", r.raw)
	}
}

// ---- 用户管理自保护 ----

func TestAdminUserSelfProtection(t *testing.T) {
	e := newEnv(t)
	adminTok, adminID := e.newUser("rick")
	_, bobID := e.newUser("bob")

	patch := func(id int64, body map[string]any) result {
		return e.do("PATCH", fmt.Sprintf("/api/v1/admin/users/%d", id), body, adminTok)
	}

	// 不能自降级 / 自停用 / 自删除
	if r := patch(adminID, map[string]any{"role": "user"}); r.status != http.StatusBadRequest {
		t.Fatalf("自降级应 400: %d %s", r.status, r.raw)
	}
	if r := patch(adminID, map[string]any{"status": "disabled"}); r.status != http.StatusBadRequest {
		t.Fatalf("自停用应 400: %d %s", r.status, r.raw)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/admin/users/%d", adminID), nil, adminTok); r.status != http.StatusBadRequest {
		t.Fatalf("自删除应 400: %d %s", r.status, r.raw)
	}
	// 幂等的自我 role=admin 允许
	if r := patch(adminID, map[string]any{"role": "admin"}); r.status != 200 {
		t.Fatalf("自保持 admin 应允许: %d %s", r.status, r.raw)
	}

	// 停用 bob：成功 + 审计写入 + bob 会话被吊销（登录被拒在 user 流程已测）
	if r := patch(bobID, map[string]any{"status": "disabled"}); r.status != 200 {
		t.Fatalf("停用 bob: %d %s", r.status, r.raw)
	}
	var action, detail string
	var actor int64
	if err := e.a.DB.QueryRow(`SELECT actor_id, action, detail FROM audit_logs
		WHERE action='user.disable' AND target_id=? ORDER BY id DESC LIMIT 1`, fmt.Sprintf("%d", bobID)).Scan(&actor, &action, &detail); err != nil {
		t.Fatalf("审计缺失: %v", err)
	}
	if actor != adminID {
		t.Fatalf("审计 actor 期望 %d 实得 %d", adminID, actor)
	}
	if !regexp.MustCompile(`"status_to":"disabled"`).MatchString(detail) {
		t.Fatalf("审计 detail 缺少 status_to: %s", detail)
	}

	// 降级不存在/非法参数
	if r := patch(99999, map[string]any{"status": "disabled"}); r.status != http.StatusNotFound {
		t.Fatalf("不存在用户应 404: %d", r.status)
	}
	if r := patch(bobID, map[string]any{"status": "zzz"}); r.status != http.StatusBadRequest {
		t.Fatalf("非法状态应 400: %d", r.status)
	}
	if r := patch(bobID, map[string]any{}); r.status != http.StatusBadRequest {
		t.Fatalf("空 body 应 400: %d", r.status)
	}

	// 列表过滤与搜索
	if r := e.do("GET", "/api/v1/admin/users?status=disabled", nil, adminTok); r.status != 200 {
		t.Fatalf("按状态过滤: %d %s", r.status, r.raw)
	} else if total := r.body["total"].(float64); total != 1 {
		t.Fatalf("disabled 过滤应 1 人: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/admin/users?status=zzz", nil, adminTok); r.status != http.StatusBadRequest {
		t.Fatalf("非法过滤值应 400: %d", r.status)
	}
	if r := e.do("GET", "/api/v1/admin/users?query=bob", nil, adminTok); r.status != 200 {
		t.Fatalf("搜索: %d %s", r.status, r.raw)
	}
}

// ---- settings 白名单 ----

func TestAdminSettings(t *testing.T) {
	e := newEnv(t)
	adminTok, _ := e.newUser("rick")

	// 未知键 → admin/invalid_setting
	if r := e.do("PUT", "/api/v1/admin/settings", map[string]any{"magic_key": "x"}, adminTok); errCode(r) != "admin/invalid_setting" {
		t.Fatalf("未知键: %s", r.raw)
	}
	// 合法键的非法值
	if r := e.do("PUT", "/api/v1/admin/settings", map[string]any{"registration_mode": "wtf"}, adminTok); errCode(r) != "admin/invalid_setting" {
		t.Fatalf("非法 mode: %s", r.raw)
	}
	if r := e.do("PUT", "/api/v1/admin/settings", map[string]any{"booking_horizon_days": 0}, adminTok); errCode(r) != "admin/invalid_setting" {
		t.Fatalf("horizon 下界: %s", r.raw)
	}
	if r := e.do("PUT", "/api/v1/admin/settings", map[string]any{"site_name": ""}, adminTok); errCode(r) != "admin/invalid_setting" {
		t.Fatalf("空 site_name: %s", r.raw)
	}

	// 合法更新 round-trip
	r := e.do("PUT", "/api/v1/admin/settings", map[string]any{
		"site_name": "MyHub", "registration_mode": "invite",
		"require_email_verification": false, "booking_horizon_days": 90,
	}, adminTok)
	if r.status != 200 {
		t.Fatalf("put settings: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	// 注意：booking_horizon_days 在响应中为字符串（getSettings 口径，PUT 末尾复用它输出）
	if d["site_name"] != "MyHub" || d["registration_mode"] != "invite" || d["require_email_verification"] != false || d["booking_horizon_days"] != "90" {
		t.Fatalf("更新响应错误: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/admin/settings", nil, adminTok); r.status != 200 || dataMap(r)["site_name"] != "MyHub" {
		t.Fatalf("get settings: %d %s", r.status, r.raw)
	}

	// settings.update 审计
	var n int
	if err := e.a.DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='settings.update'`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("settings 审计缺失: %d err=%v", n, err)
	}
}
