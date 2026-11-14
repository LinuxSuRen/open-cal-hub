// cal 包端到端流程测试（外部测试包）：httptest + 临时 SQLite。
// 覆盖：成员权限矩阵（owner/editor/viewer/陌生人）、分享链接过期与撤销、
// ICS 导入按 UID 去重、PATCH/DELETE scope=single 生成 recurrence_id 覆盖行。
package cal_test

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
	"strings"
	"testing"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
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
	user.Register(mux, a) // 回填 a.LoadUser
	cal.Register(mux, a)
	a.Mux = auth.OptionalMiddleware(a.Signer, a.User)(mux)
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

func dataList(r result) []any {
	if r.body == nil {
		return nil
	}
	d, _ := r.body["data"].([]any)
	return d
}

func (e *env) outboxToken(kind string) string {
	e.t.Helper()
	var body string
	if err := e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind=? ORDER BY id DESC LIMIT 1`, kind).Scan(&body); err != nil {
		e.t.Fatalf("read outbox(%s): %v", kind, err)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token in mail body: %s", body)
	}
	return m[1]
}

// newUser 注册并验证一个用户，返回其访问令牌。
func (e *env) newUser(username string) string {
	e.t.Helper()
	r := e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": username + "@example.com", "password": "passw0rd!", "username": username,
	}, "")
	if r.status != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", username, r.status, r.raw)
	}
	tok := e.outboxToken("verify_email")
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": tok}, ""); r.status != 200 {
		e.t.Fatalf("verify-email %s: %d %s", username, r.status, r.raw)
	}
	r = e.do("POST", "/api/v1/auth/login", map[string]string{"email": username + "@example.com", "password": "passw0rd!"}, "")
	if r.status != 200 {
		e.t.Fatalf("login %s: %d %s", username, r.status, r.raw)
	}
	access, _ := dataMap(r)["access_token"].(string)
	return access
}

func (e *env) userID(username string) int64 {
	e.t.Helper()
	var id int64
	if err := e.a.DB.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id); err != nil {
		e.t.Fatalf("user id: %v", err)
	}
	return id
}

func (e *env) createCalendar(token, name string) int64 {
	e.t.Helper()
	r := e.do("POST", "/api/v1/calendars", map[string]string{"name": name, "timezone": "Asia/Shanghai"}, token)
	if r.status != http.StatusCreated {
		e.t.Fatalf("create calendar: %d %s", r.status, r.raw)
	}
	return int64(dataMap(r)["id"].(float64))
}

func (e *env) createEvent(token string, calID int64, summary, start, end string, rrule any) (int64, string) {
	e.t.Helper()
	body := map[string]any{"summary": summary, "start": start, "end": end, "tz": "Asia/Shanghai"}
	if rrule != nil {
		body["rrule"] = rrule
	}
	r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), body, token)
	if r.status != http.StatusCreated {
		e.t.Fatalf("create event: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	return int64(d["id"].(float64)), d["uid"].(string)
}

// ---- 成员权限矩阵 ----

func TestCalendarMemberPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	ownerTok := e.newUser("rick")
	editorTok := e.newUser("erin")
	viewerTok := e.newUser("carol")
	strangerTok := e.newUser("mallory")

	calID := e.createCalendar(ownerTok, "工作")

	// 添加成员：角色校验、重复添加、添加属主
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/members", calID),
		map[string]string{"username_or_email": "erin", "role": "chief"}, ownerTok); errCode(r) != "common/validation_failed" {
		t.Fatalf("非法角色应被拒: %s", r.raw)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/members", calID),
		map[string]string{"username_or_email": "rick", "role": "viewer"}, ownerTok); errCode(r) != "cal/member_exists" {
		t.Fatalf("添加属主应报已存在: %s", r.raw)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/members", calID),
		map[string]string{"username_or_email": "erin", "role": "editor"}, ownerTok); r.status != http.StatusCreated {
		t.Fatalf("add editor: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/members", calID),
		map[string]string{"username_or_email": "carol", "role": "viewer"}, ownerTok); r.status != http.StatusCreated {
		t.Fatalf("add viewer: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/members", calID),
		map[string]string{"username_or_email": "erin", "role": "viewer"}, ownerTok); errCode(r) != "cal/member_exists" {
		t.Fatalf("重复添加应报已存在: %s", r.raw)
	}

	// 基线事件（owner 建）
	baseID, _ := e.createEvent(ownerTok, calID, "基线", "2026-12-07T09:00:00Z", "2026-12-07T10:00:00Z", nil)
	listPath := fmt.Sprintf("/api/v1/calendars/%d/events?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z", calID)

	// viewer：可读不可写；不能管理成员与分享
	if r := e.do("GET", listPath, nil, viewerTok); r.status != 200 || len(dataList(r)) != 1 {
		t.Fatalf("viewer list: %d %s", r.status, r.raw)
	}
	if status := e.createEventShouldFail(viewerTok, calID, "2026-12-08T09:00:00Z"); status != http.StatusForbidden {
		t.Fatalf("viewer create should 403: %d", status)
	}
	if r := e.do("PATCH", fmt.Sprintf("/api/v1/events/%d", baseID), map[string]any{"summary": "hack"}, viewerTok); r.status != http.StatusForbidden {
		t.Fatalf("viewer patch should 403: %d %s", r.status, r.raw)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/events/%d", baseID), nil, viewerTok); r.status != http.StatusForbidden {
		t.Fatalf("viewer delete should 403: %d %s", r.status, r.raw)
	}
	if r := e.do("GET", fmt.Sprintf("/api/v1/calendars/%d/members", calID), nil, viewerTok); r.status != http.StatusForbidden {
		t.Fatalf("viewer list members should 403: %d", r.status)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/share-links", calID), map[string]string{"label": "l"}, viewerTok); r.status != http.StatusForbidden {
		t.Fatalf("viewer share should 403: %d", r.status)
	}

	// editor：可增改删事件；不可改日历元数据
	editorEventID, _ := e.createEvent(editorTok, calID, "编辑建", "2026-12-09T09:00:00Z", "2026-12-09T10:00:00Z", nil)
	if r := e.do("PATCH", fmt.Sprintf("/api/v1/events/%d", editorEventID), map[string]any{"summary": "编辑改"}, editorTok); r.status != 200 {
		t.Fatalf("editor patch: %d %s", r.status, r.raw)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/events/%d", editorEventID), nil, editorTok); r.status != 200 {
		t.Fatalf("editor delete: %d %s", r.status, r.raw)
	}
	if r := e.do("PATCH", fmt.Sprintf("/api/v1/calendars/%d", calID), map[string]any{"name": "改名"}, editorTok); r.status != http.StatusForbidden {
		t.Fatalf("editor patch calendar should 403: %d", r.status)
	}

	// 陌生人：一律视作不存在（404），不泄漏日历存在性
	if r := e.do("GET", listPath, nil, strangerTok); r.status != http.StatusNotFound || errCode(r) != "cal/calendar_not_found" {
		t.Fatalf("stranger list should calendar_not_found: %d %s", r.status, r.raw)
	}
	if status := e.createEventShouldFail(strangerTok, calID, "2026-12-08T09:00:00Z"); status != http.StatusNotFound {
		t.Fatalf("stranger create should 404: %d", status)
	}

	// 共享日历出现在 carol 的日历列表里，角色 viewer
	if r := e.do("GET", "/api/v1/calendars", nil, viewerTok); r.status != 200 {
		t.Fatalf("list calendars: %d", r.status)
	} else {
		found := false
		for _, it := range dataList(r) {
			if m, ok := it.(map[string]any); ok && int64(m["id"].(float64)) == calID {
				found = true
				if m["role"] != "viewer" {
					t.Fatalf("carol 角色应为 viewer: %s", r.raw)
				}
			}
		}
		if !found {
			t.Fatalf("共享日历应出现在列表: %s", r.raw)
		}
	}

	// 移除成员后立即失去读权限；重复移除 404
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/calendars/%d/members/%d", calID, e.userID("carol")), nil, ownerTok); r.status != 200 {
		t.Fatalf("remove member: %d %s", r.status, r.raw)
	}
	if r := e.do("GET", listPath, nil, viewerTok); r.status != http.StatusNotFound {
		t.Fatalf("removed member should 404: %d", r.status)
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/calendars/%d/members/%d", calID, e.userID("carol")), nil, ownerTok); r.status != http.StatusNotFound || errCode(r) != "cal/member_not_found" {
		t.Fatalf("double remove: %d %s", r.status, r.raw)
	}
}

// createEventShouldFail 期望失败的创建，返回响应状态码。
func (e *env) createEventShouldFail(token string, calID int64, start string) int {
	r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), map[string]any{
		"summary": "x", "start": start, "end": start, "tz": "UTC",
	}, token)
	return r.status
}

// ---- 分享链接：创建/公开访问/撤销/过期 ----

func TestShareLinkLifecycle(t *testing.T) {
	e := newEnv(t)
	ownerTok := e.newUser("rick")
	calID := e.createCalendar(ownerTok, "公开")

	e.createEvent(ownerTok, calID, "周会", "2026-12-07T01:00:00Z", "2026-12-07T02:00:00Z", nil)

	// 创建分享
	r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/share-links", calID), map[string]string{"label": "给朋友"}, ownerTok)
	if r.status != http.StatusCreated {
		t.Fatalf("create share: %d %s", r.status, r.raw)
	}
	token, _ := dataMap(r)["token"].(string)
	if token == "" {
		t.Fatalf("share token missing: %s", r.raw)
	}

	// 公开访问：信息 + 事件列表（无需登录）
	if r := e.do("GET", "/api/v1/pub/share/"+token, nil, ""); r.status != 200 {
		t.Fatalf("pub info: %d %s", r.status, r.raw)
	} else if dataMap(r)["calendar"].(map[string]any)["name"] != "公开" {
		t.Fatalf("pub info calendar name: %s", r.raw)
	}
	pubEvents := "/api/v1/pub/share/" + token + "/events?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z"
	if r := e.do("GET", pubEvents, nil, ""); r.status != 200 || len(dataList(r)) != 1 {
		t.Fatalf("pub events: %d %s", r.status, r.raw)
	}
	// 订阅 ICS 可下载
	if r := e.do("GET", "/api/v1/pub/share/"+token+"/ics", nil, ""); r.status != 200 {
		t.Fatalf("pub ics: %d %s", r.status, r.raw)
	}
	// 乱 token → 404
	if r := e.do("GET", "/api/v1/pub/share/deadbeef", nil, ""); r.status != http.StatusNotFound || errCode(r) != "cal/share_not_found" {
		t.Fatalf("bad token: %d %s", r.status, r.raw)
	}

	// 过期分享 → 410 share_expired
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	r = e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/share-links", calID), map[string]string{"label": "过期", "expires_at": past}, ownerTok)
	expiredToken, _ := dataMap(r)["token"].(string)
	if r := e.do("GET", "/api/v1/pub/share/"+expiredToken, nil, ""); r.status != http.StatusGone || errCode(r) != "cal/share_expired" {
		t.Fatalf("expired share should 410: %d %s", r.status, r.raw)
	}

	// 列表包含两条；撤销第一条后公开访问 404
	r = e.do("GET", fmt.Sprintf("/api/v1/calendars/%d/share-links", calID), nil, ownerTok)
	if r.status != 200 || len(dataList(r)) != 2 {
		t.Fatalf("list shares: %d %s", r.status, r.raw)
	}
	var linkID float64
	for _, it := range dataList(r) {
		if m, ok := it.(map[string]any); ok && m["token"] == token {
			linkID = m["id"].(float64)
		}
	}
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/share-links/%d", int64(linkID)), nil, ownerTok); r.status != 200 {
		t.Fatalf("revoke share: %d %s", r.status, r.raw)
	}
	if r := e.do("GET", pubEvents, nil, ""); r.status != http.StatusNotFound || errCode(r) != "cal/share_not_found" {
		t.Fatalf("revoked share should 404: %d %s", r.status, r.raw)
	}
}

// ---- ICS 导入按 UID 去重 ----

func TestImportICSDedup(t *testing.T) {
	e := newEnv(t)
	ownerTok := e.newUser("rick")
	calID := e.createCalendar(ownerTok, "导入")

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:dup@import\r\nDTSTART:20261207T090000Z\r\nDTEND:20261207T100000Z\r\nSUMMARY:第一次\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:dup@import\r\nDTSTART:20261207T090000Z\r\nDTEND:20261207T100000Z\r\nSUMMARY:重复\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:new@import\r\nDTSTART:20261208T090000Z\r\nDTEND:20261208T100000Z\r\nSUMMARY:新事件\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/import", calID), map[string]string{"ics": ics}, ownerTok)
	if r.status != 200 {
		t.Fatalf("import: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	if int(d["imported"].(float64)) != 2 || int(d["skipped"].(float64)) != 1 {
		t.Fatalf("首次导入期望 imported=2 skipped=1: %s", r.raw)
	}

	// 再次导入同一批：全部跳过
	r = e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/import", calID), map[string]string{"ics": ics}, ownerTok)
	d = dataMap(r)
	if int(d["imported"].(float64)) != 0 || int(d["skipped"].(float64)) != 3 {
		t.Fatalf("重复导入期望 imported=0 skipped=3: %s", r.raw)
	}

	// DB 中每个 UID 只有一行
	var n int
	if err := e.a.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE calendar_id=? AND uid='dup@import'`, calID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("dup uid rows = %d err=%v", n, err)
	}

	// 非 ICS 内容 → cal/import_failed
	if r := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/import", calID), map[string]string{"ics": "not-a-calendar"}, ownerTok); errCode(r) != "cal/import_failed" {
		t.Fatalf("bad ics: %s", r.raw)
	}
}

// ---- scope=single：PATCH 生成覆盖行、DELETE 单实例取消、scope=all 清系列 ----

func TestRecurringScopeSingle(t *testing.T) {
	e := newEnv(t)
	ownerTok := e.newUser("rick")
	calID := e.createCalendar(ownerTok, "重复")

	// 每周一 09:00（上海）= 01:00Z，自 12-07 起
	masterID, masterUID := e.createEvent(ownerTok, calID, "周会",
		"2026-12-07T09:00:00+08:00", "2026-12-07T10:00:00+08:00",
		map[string]any{"freq": "WEEKLY", "byday": []string{"MO"}})
	listPath := fmt.Sprintf("/api/v1/calendars/%d/events?from=2026-12-01T00:00:00Z&to=2027-01-01T00:00:00Z", calID)

	listStarts := func() []string {
		r := e.do("GET", listPath, nil, ownerTok)
		var out []string
		for _, it := range dataList(r) {
			m := it.(map[string]any)
			out = append(out, fmt.Sprintf("%v|%v", m["start"], m["summary"]))
		}
		return out
	}

	// 12 月的周一：07 14 21 28
	if got := listStarts(); len(got) != 4 {
		t.Fatalf("期望 4 个实例，实得 %d: %v", len(got), got)
	}

	// 改 12-14 这一实例：scope=single + recurrence_id
	r := e.do("PATCH", fmt.Sprintf("/api/v1/events/%d?scope=single", masterID), map[string]any{
		"summary": "改期周会", "recurrence_id": "2026-12-14T01:00:00Z",
	}, ownerTok)
	if r.status != 200 {
		t.Fatalf("patch single: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	if d["summary"] != "改期周会" || d["recurrence_id"] != "2026-12-14T01:00:00Z" {
		t.Fatalf("覆盖实例响应错误: %s", r.raw)
	}

	// 覆盖行落库：同 UID 且 recurrence_id 非空恰一行
	var overrides int
	if err := e.a.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE calendar_id=? AND uid=? AND recurrence_id IS NOT NULL`,
		calID, masterUID).Scan(&overrides); err != nil || overrides != 1 {
		t.Fatalf("覆盖行数 = %d err=%v", overrides, err)
	}

	// 列表：12-14 实例换成新标题，其余 3 个保持原样
	got := listStarts()
	if len(got) != 4 {
		t.Fatalf("覆盖后仍应 4 个实例: %v", got)
	}
	changed := 0
	for _, s := range got {
		if strings.HasPrefix(s, "2026-12-14T01:00:00Z|改期周会") {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("12-14 实例应为改期标题: %v", got)
	}

	// 缺 recurrence_id 的 scope=single → 校验错误
	if r := e.do("PATCH", fmt.Sprintf("/api/v1/events/%d?scope=single", masterID), map[string]any{"summary": "x"}, ownerTok); errCode(r) != "common/validation_failed" {
		t.Fatalf("missing recurrence_id: %s", r.raw)
	}

	// 删除单实例（12-21）：默认含取消可见；include_cancelled=false 时隐藏
	r = e.do("DELETE", fmt.Sprintf("/api/v1/events/%d?scope=single&recurrence_id=2026-12-21T01:00:00Z", masterID), nil, ownerTok)
	if r.status != 200 {
		t.Fatalf("delete single: %d %s", r.status, r.raw)
	}
	r = e.do("GET", listPath, nil, ownerTok)
	cancelled := 0
	for _, it := range dataList(r) {
		if m, ok := it.(map[string]any); ok && m["start"] == "2026-12-21T01:00:00Z" && m["status"] == "cancelled" {
			cancelled++
		}
	}
	if cancelled != 1 {
		t.Fatalf("12-21 应为取消覆盖: %s", r.raw)
	}
	r = e.do("GET", fmt.Sprintf("/api/v1/calendars/%d/events?from=2026-12-01T00:00:00Z&to=2027-01-01T00:00:00Z&include_cancelled=false", calID), nil, ownerTok)
	if n := len(dataList(r)); n != 3 {
		t.Fatalf("过滤取消后应 3 个实例，实得 %d", n)
	}

	// scope=all：整系列（含覆盖行）删除
	if r := e.do("DELETE", fmt.Sprintf("/api/v1/events/%d?scope=all", masterID), nil, ownerTok); r.status != 200 {
		t.Fatalf("delete all: %d %s", r.status, r.raw)
	}
	var left int
	if err := e.a.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE calendar_id=?`, calID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("系列删除后应无残留行: %d err=%v", left, err)
	}
}
