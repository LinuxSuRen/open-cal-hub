// dav 包协议级测试（外部测试包）：用 go-webdav 的 caldav.NewClient 打真实
// httptest 服务，覆盖 PROPFIND 深度 0/1、MKCOL、PUT→GET ETag 一致性、
// REPORT time-range、DELETE 与 Basic 认证（登录密码 + 应用密码）。
package dav_test

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
	"regexp"
	"testing"
	"time"

	ics "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/dav"
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
	dav.Register(mux, a)
	a.Mux = mux
	srv := httptest.NewServer(server.New(a))
	t.Cleanup(srv.Close)
	return &env{t: t, a: a, srv: srv}
}

func (e *env) rest(method, path string, body any, token string) (int, map[string]any, string) {
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

// newUser 注册 + 验证 + 登录，返回访问令牌。
func (e *env) newUser(username string) string {
	e.t.Helper()
	st, _, raw := e.rest("POST", "/api/v1/auth/register", map[string]any{
		"email": username + "@example.com", "password": "passw0rd!", "username": username,
	}, "")
	if st != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", username, st, raw)
	}
	var body string
	if err := e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='verify_email' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		e.t.Fatalf("outbox: %v", err)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token: %s", body)
	}
	if st, _, raw = e.rest("POST", "/api/v1/auth/verify-email", map[string]string{"token": m[1]}, ""); st != 200 {
		e.t.Fatalf("verify: %d %s", st, raw)
	}
	st, m2, raw := e.rest("POST", "/api/v1/auth/login", map[string]string{
		"email": username + "@example.com", "password": "passw0rd!",
	}, "")
	if st != 200 {
		e.t.Fatalf("login: %d %s", st, raw)
	}
	data, _ := m2["data"].(map[string]any)
	tok, _ := data["access_token"].(string)
	return tok
}

// basicClient 构造带 Basic 认证的 caldav 客户端。
func (e *env) basicClient(login, password string) *caldav.Client {
	e.t.Helper()
	hc := &http.Client{Transport: basicTransport{login: login, password: password, rt: http.DefaultTransport}}
	c, err := caldav.NewClient(hc, e.srv.URL)
	if err != nil {
		e.t.Fatalf("caldav client: %v", err)
	}
	return c
}

type basicTransport struct {
	login, password string
	rt              http.RoundTripper
}

func (b basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.SetBasicAuth(b.login, b.password)
	return b.rt.RoundTrip(r)
}

// sampleVEVENT 构造一个 VEVENT 的 VCALENDAR；uid 决定资源路径名。
func sampleVEVENT(uid, summary string, start time.Time, withTZID bool) *ics.Calendar {
	ev := ics.NewEvent()
	ev.Props.SetText(ics.PropUID, uid)
	ev.Props.SetDateTime(ics.PropDateTimeStamp, time.Now().UTC())
	ev.Props.SetText(ics.PropSummary, summary)
	if withTZID {
		sh, _ := time.LoadLocation("Asia/Shanghai")
		ev.Props.SetDateTime(ics.PropDateTimeStart, start.In(sh))
		ev.Props.SetDateTime(ics.PropDateTimeEnd, start.In(sh).Add(time.Hour))
	} else {
		ev.Props.SetDateTime(ics.PropDateTimeStart, start.UTC())
		ev.Props.SetDateTime(ics.PropDateTimeEnd, start.UTC().Add(time.Hour))
	}
	cal := ics.NewCalendar()
	cal.Props.SetText(ics.PropProductID, "-//Test//Sample//CN")
	cal.Props.SetText(ics.PropVersion, "2.0")
	cal.Children = append(cal.Children, ev.Component)
	return cal
}

// 首用户注册后自动建默认日历（注册钩子不在此环境接线，这里手动建）。
func (e *env) ensureDefaultCalendar(userID int64) int64 {
	e.t.Helper()
	var id int64
	err := e.a.DB.QueryRow(`SELECT id FROM calendars WHERE owner_id=? AND deleted_at IS NULL ORDER BY id LIMIT 1`, userID).Scan(&id)
	if err == nil {
		return id
	}
	if _, err := e.a.DB.Exec(`INSERT INTO calendars (owner_id, name, description, color, timezone, is_default, created_at, updated_at)
		VALUES (?,?,?,?,?,1,?,?)`, userID, "默认日历", "", "#4F6EF7", "UTC", time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		e.t.Fatalf("insert calendar: %v", err)
	}
	if err := e.a.DB.QueryRow(`SELECT id FROM calendars WHERE owner_id=? AND deleted_at IS NULL ORDER BY id LIMIT 1`, userID).Scan(&id); err != nil {
		e.t.Fatalf("load calendar: %v", err)
	}
	return id
}

func TestCalDAVProtocol(t *testing.T) {
	e := newEnv(t)
	_ = e.newUser("alice") // 注册（同时保证用户可 Basic 登录）
	_ = e.newUser("bob")
	var aliceID int64
	if err := e.a.DB.QueryRow(`SELECT id FROM users WHERE username='alice'`).Scan(&aliceID); err != nil {
		t.Fatalf("alice id: %v", err)
	}
	calID := e.ensureDefaultCalendar(aliceID)

	ctx := context.Background()
	cli := e.basicClient("alice", "passw0rd!")
	principal := dav.Prefix + "/users/alice/"
	homeSet := principal + "calendars/"
	calPath := fmt.Sprintf("%s%d/", homeSet, calID)

	// 无认证 → 401 + WWW-Authenticate
	req, _ := http.NewRequest("PROPFIND", e.srv.URL+principal, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("propfind no auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("无认证 PROPFIND 应 401 且带 WWW-Authenticate: %d", resp.StatusCode)
	}
	// 错误密码 → 401
	if _, err := e.basicClient("alice", "wrong-pass").FindCalendarHomeSet(ctx, principal); err == nil {
		t.Fatal("错误密码应 401")
	}

	// PROPFIND 深度 0（principal）：返回 calendar-home-set
	hs, err := cli.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		t.Fatalf("find home set: %v", err)
	}
	if hs != homeSet {
		t.Fatalf("home set 期望 %s 实得 %s", homeSet, hs)
	}

	// PROPFIND 深度 1（home set）：列出默认日历
	cals, err := cli.FindCalendars(ctx, homeSet)
	if err != nil {
		t.Fatalf("find calendars: %v", err)
	}
	if len(cals) != 1 || cals[0].Name != "默认日历" || cals[0].Path != calPath {
		t.Fatalf("calendars = %+v", cals)
	}
	if len(cals[0].SupportedComponentSet) == 0 {
		t.Fatalf("supported-component-set 缺失: %+v", cals[0])
	}

	// MKCOL：在 home set 下创建日历
	if err := cli.Mkdir(ctx, homeSet+"from-dav"); err != nil {
		t.Fatalf("mkcol: %v", err)
	}
	cals, err = cli.FindCalendars(ctx, homeSet)
	if err != nil || len(cals) != 2 {
		t.Fatalf("MKCOL 后应有两个日历: %v %+v", err, cals)
	}

	// PUT（TZID 形式）→ 201 + ETag
	uid1 := "dav-tzid-001"
	objPath := fmt.Sprintf("%s%d/%s.ics", homeSet, calID, uid1)
	start := time.Date(2026, 12, 7, 9, 0, 0, 0, time.UTC) // 上海 17:00
	put1, err := cli.PutCalendarObject(ctx, objPath, sampleVEVENT(uid1, "带时区事件", start, true))
	if err != nil {
		t.Fatalf("put tzid: %v", err)
	}
	if put1.ETag == "" {
		t.Fatal("PUT 应返回 ETag")
	}

	// PUT（UTC Z 形式）
	uid2 := "dav-utc-002"
	objPath2 := fmt.Sprintf("%s%d/%s.ics", homeSet, calID, uid2)
	start2 := time.Date(2026, 12, 10, 2, 0, 0, 0, time.UTC)
	if _, err := cli.PutCalendarObject(ctx, objPath2, sampleVEVENT(uid2, "UTC 事件", start2, false)); err != nil {
		t.Fatalf("put utc: %v", err)
	}

	// GET → ETag 与 PUT 一致、内容可解析、UID 正确
	got, err := cli.GetCalendarObject(ctx, objPath)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if got.ETag != put1.ETag {
		t.Fatalf("ETag 不一致: put=%s get=%s", put1.ETag, got.ETag)
	}
	if len(got.Data.Events()) != 1 || got.Data.Events()[0].Props.Get(ics.PropUID).Value != uid1 {
		t.Fatalf("GET 内容错误: %+v", got.Data)
	}

	// PUT 覆盖已有资源（UID 相同、改标题）→ ETag 变化
	upd, err := cli.PutCalendarObject(ctx, objPath, sampleVEVENT(uid1, "带时区事件-改", start, true))
	if err != nil {
		t.Fatalf("put update: %v", err)
	}
	if upd.ETag == put1.ETag {
		t.Fatal("更新后 ETag 应变化")
	}

	// REPORT time-range：窗口内含 uid1（12-07）与 uid2（12-10），窗外不含
	decFilter := func(start, end time.Time) *caldav.CalendarQuery {
		return &caldav.CalendarQuery{
			CompRequest: caldav.CalendarCompRequest{Name: ics.CompEvent},
			CompFilter: caldav.CompFilter{
				Name: ics.CompCalendar,
				Comps: []caldav.CompFilter{{
					Name:  ics.CompEvent,
					Start: start,
					End:   end,
				}},
			},
		}
	}
	res, err := cli.QueryCalendar(ctx, calPath, decFilter(
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("time-range 期望 2 个对象，实得 %d", len(res))
	}
	res, err = cli.QueryCalendar(ctx, calPath, decFilter(
		time.Date(2026, 12, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 9, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("report empty window: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("空窗口期望 0 个对象，实得 %d", len(res))
	}

	// DELETE → 再 GET 404
	if err := cli.RemoveAll(ctx, objPath2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := cli.GetCalendarObject(ctx, objPath2); err == nil {
		t.Fatal("删除后 GET 应 404")
	}
	var n int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE uid=?`, uid2).Scan(&n)
	if n != 0 {
		t.Fatalf("删除后应无残留行: %d", n)
	}

	// 越权：bob 访问 alice 的日历对象 → 404（不泄漏存在性）
	bobCli := e.basicClient("bob", "passw0rd!")
	if _, err := bobCli.GetCalendarObject(ctx, objPath); err == nil {
		t.Fatal("bob 不应能读 alice 的对象")
	}
	// bob MKCOL 到 alice 的 home set 段 → 403
	mkreq, _ := http.NewRequest("MKCOL", e.srv.URL+fmt.Sprintf("%s/users/alice/calendars/hack", dav.Prefix), nil)
	mkreq.SetBasicAuth("bob", "passw0rd!")
	mkresp, err := http.DefaultClient.Do(mkreq)
	if err != nil {
		t.Fatalf("bob mkcol: %v", err)
	}
	io.Copy(io.Discard, mkresp.Body)
	mkresp.Body.Close()
	// 上游协议层不识别本包错误类型，403 会退化为 500（backend.go 已注明）；
	// 关键安全性质是：请求失败且不产生副作用。
	if mkresp.StatusCode < 400 {
		t.Fatalf("bob MKCOL 到 alice 路径应失败，实得 %d", mkresp.StatusCode)
	}
	var hacked int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM calendars WHERE name='hack'`).Scan(&hacked)
	if hacked != 0 {
		t.Fatal("越权 MKCOL 不应创建日历")
	}
}

// 应用密码 Basic 认证。
func TestCalDAVAppPasswordAuth(t *testing.T) {
	e := newEnv(t)
	aliceTok := e.newUser("alice")
	var aliceID int64
	_ = e.a.DB.QueryRow(`SELECT id FROM users WHERE username='alice'`).Scan(&aliceID)
	calID := e.ensureDefaultCalendar(aliceID)

	st, m, raw := e.rest("POST", "/api/v1/me/app-passwords", map[string]string{"label": "dav"}, aliceTok)
	if st != http.StatusCreated {
		t.Fatalf("create app password: %d %s", st, raw)
	}
	data, _ := m["data"].(map[string]any)
	appPw, _ := data["password"].(string)
	if appPw == "" {
		t.Fatalf("app password missing: %s", raw)
	}

	// 登录密码与应用密码都能通过 Basic 认证完成 PROPFIND
	ctx := context.Background()
	for _, pw := range []string{"passw0rd!", appPw} {
		hs, err := e.basicClient("alice@example.com", pw).FindCalendarHomeSet(ctx, dav.Prefix+"/users/alice/")
		if err != nil {
			t.Fatalf("basic auth (%s): %v", pw, err)
		}
		if hs != dav.Prefix+"/users/alice/calendars/" {
			t.Fatalf("home set: %s", hs)
		}
	}
	// 应用密码用过之后 last_used_at 落库
	var last any
	_ = e.a.DB.QueryRow(`SELECT last_used_at FROM app_passwords WHERE label='dav'`).Scan(&last)
	if last == nil {
		t.Fatal("应用密码使用后应更新 last_used_at")
	}
	_ = calID
}
