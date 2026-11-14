// 契约对齐锁定测试（issue 联调清单）：
// 1) 全天事件排他结束语义；3) rrule until 双格式（RFC3339 与紧凑）；4) invite-codes 响应形状。
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

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
	"github.com/linuxsuren/open-cal-hub/backend/internal/server"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

type calAlignEnv struct {
	t   *testing.T
	a   *app.App
	srv *httptest.Server
}

func newCalAlignEnv(t *testing.T) *calAlignEnv {
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
	cal.Register(mux, a)
	a.Mux = auth.OptionalMiddleware(a.Signer, a.User)(mux)
	srv := httptest.NewServer(server.New(a))
	t.Cleanup(srv.Close)
	return &calAlignEnv{t: t, a: a, srv: srv}
}

func (e *calAlignEnv) do(method, path string, body any, token string) (int, map[string]any, string) {
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

func dataOf(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func (e *calAlignEnv) verifiedToken(username string) string {
	e.t.Helper()
	st, _, raw := e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": username + "@example.com", "password": "passw0rd!", "username": username,
	}, "")
	if st != http.StatusCreated {
		e.t.Fatalf("register: %d %s", st, raw)
	}
	var body string
	_ = e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='verify_email' ORDER BY id DESC LIMIT 1`).Scan(&body)
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token: %s", body)
	}
	if st, _, raw = e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": m[1]}, ""); st != 200 {
		e.t.Fatalf("verify: %d %s", st, raw)
	}
	st, m2, raw := e.do("POST", "/api/v1/auth/login", map[string]string{
		"email": username + "@example.com", "password": "passw0rd!",
	}, "")
	if st != 200 {
		e.t.Fatalf("login: %d %s", st, raw)
	}
	return dataOf(m2)["access_token"].(string)
}

// 全天事件：排他结束（start=当日 00:00Z、end=结束次日 00:00Z；缺省 end 自动 +1 天）。
func TestAllDayExclusiveEndSemantics(t *testing.T) {
	e := newCalAlignEnv(t)
	tok := e.verifiedToken("rick")
	st, m, raw := e.do("POST", "/api/v1/calendars", map[string]string{"name": "对齐"}, tok)
	if st != http.StatusCreated {
		t.Fatalf("calendar: %d %s", st, raw)
	}
	calID := int64(dataOf(m)["id"].(float64))

	// 两天全天：12-07 至 12-09（排他）
	st, m, raw = e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), map[string]any{
		"summary": "两天全天", "start": "2026-12-07", "end": "2026-12-09", "all_day": true,
	}, tok)
	if st != http.StatusCreated {
		t.Fatalf("create allday: %d %s", st, raw)
	}
	d := dataOf(m)
	if d["start"] != "2026-12-07T00:00:00Z" || d["end"] != "2026-12-09T00:00:00Z" {
		t.Fatalf("全天排他语义: start=%v end=%v", d["start"], d["end"])
	}
	if d["all_day"] != true {
		t.Fatalf("all_day: %v", d["all_day"])
	}

	// 缺省 end → 自动次日
	st, m, raw = e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), map[string]any{
		"summary": "单日全天", "start": "2026-12-10", "all_day": true,
	}, tok)
	if st != http.StatusCreated {
		t.Fatalf("create allday no end: %d %s", st, raw)
	}
	d = dataOf(m)
	if d["start"] != "2026-12-10T00:00:00Z" || d["end"] != "2026-12-11T00:00:00Z" {
		t.Fatalf("缺省 end 语义: start=%v end=%v", d["start"], d["end"])
	}

	// end 早于等于 start → 归一为次日
	st, m, _ = e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), map[string]any{
		"summary": "归一", "start": "2026-12-11", "end": "2026-12-11", "all_day": true,
	}, tok)
	if st != http.StatusCreated {
		t.Fatalf("create allday same end: %d", st)
	}
	if d := dataOf(m); d["end"] != "2026-12-12T00:00:00Z" {
		t.Fatalf("同日 end 应归一为次日: %v", d["end"])
	}

	// 跨日渲染：列表窗口 [12-08, 12-09) 命中两天全天的实例（按开始时刻归属）
	st, m, raw = e.do("GET", fmt.Sprintf("/api/v1/calendars/%d/events?from=2026-12-08T00:00:00Z&to=2026-12-09T00:00:00Z", calID), nil, tok)
	if st != 200 {
		t.Fatalf("list: %d %s", st, raw)
	}
	list, _ := m["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("跨日窗口应命中 1 个实例（排他结束覆盖 12-08），实得 %d", len(list))
	}
}

// rrule until：RFC3339 与紧凑格式等价，输出统一紧凑；非法格式拒绝。
func TestRRuleUntilFormats(t *testing.T) {
	// CanonicalRRule 层
	a, err := cal.CanonicalRRule(&cal.RRuleInput{Freq: "WEEKLY", Byday: []string{"MO"}, Until: "2026-12-30T23:59:59Z"})
	if err != nil {
		t.Fatalf("RFC3339 until: %v", err)
	}
	b, err := cal.CanonicalRRule(&cal.RRuleInput{Freq: "WEEKLY", Byday: []string{"MO"}, Until: "20261230T235959Z"})
	if err != nil {
		t.Fatalf("紧凑 until: %v", err)
	}
	if a != b {
		t.Fatalf("两种 until 输入应产出相同规范文本: %q vs %q", a, b)
	}
	if !strings.Contains(a, "UNTIL=20261230T235959Z") {
		t.Fatalf("输出应为紧凑 UNTIL: %s", a)
	}
	if _, err := cal.CanonicalRRule(&cal.RRuleInput{Freq: "DAILY", Until: "2026/12/30"}); err == nil {
		t.Fatal("非法 until 应报错")
	}

	// HTTP 层：两种格式创建等价系列并展开一致
	e := newCalAlignEnv(t)
	tok := e.verifiedToken("rick")
	st, m, raw := e.do("POST", "/api/v1/calendars", map[string]string{"name": "对齐"}, tok)
	if st != http.StatusCreated {
		t.Fatalf("calendar: %d %s", st, raw)
	}
	calID := int64(dataOf(m)["id"].(float64))
	create := func(until string) int64 {
		st, m, raw := e.do("POST", fmt.Sprintf("/api/v1/calendars/%d/events", calID), map[string]any{
			"summary": "每周", "start": "2026-12-07T09:00:00Z", "end": "2026-12-07T10:00:00Z",
			"rrule":   map[string]any{"freq": "WEEKLY", "byday": []string{"MO"}, "until": until},
		}, tok)
		if st != http.StatusCreated {
			e.t.Fatalf("create until=%s: %d %s", until, st, raw)
		}
		return int64(dataOf(m)["id"].(float64))
	}
	_ = create("2026-12-30T23:59:59Z")
	_ = create("20261230T235959Z")
	st, m, raw = e.do("GET", fmt.Sprintf("/api/v1/calendars/%d/events?from=2026-12-01T00:00:00Z&to=2027-01-15T00:00:00Z", calID), nil, tok)
	if st != 200 {
		t.Fatalf("list: %d %s", st, raw)
	}
	list, _ := m["data"].([]any)
	// 12 月周一 07/14/21/28 × 2 系列 = 8
	if len(list) != 8 {
		t.Fatalf("两种 until 应各展开 4 个实例，实得 %d: %s", len(list), raw)
	}
}
