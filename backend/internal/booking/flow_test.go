// 预约成交链路端到端测试（外部测试包）：公开页 → 槽位 → 下单 → 管理 →
// 取消/改期，并发下单唯一性，蜜罐，邮件出箱与提醒 worker。
package booking_test

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
	"sync"
	"testing"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/booking"
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
	booking.Register(mux, a)
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

func (e *env) newUser(username string) string {
	e.t.Helper()
	r := e.do("POST", "/api/v1/auth/register", map[string]any{
		"email": username + "@example.com", "password": "passw0rd!", "username": username,
	}, "")
	if r.status != http.StatusCreated {
		e.t.Fatalf("register %s: %d %s", username, r.status, r.raw)
	}
	var body string
	if err := e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='verify_email' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		e.t.Fatalf("outbox: %v", err)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`).FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token: %s", body)
	}
	if r := e.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": m[1]}, ""); r.status != 200 {
		e.t.Fatalf("verify: %d %s", r.status, r.raw)
	}
	r = e.do("POST", "/api/v1/auth/login", map[string]string{
		"email": username + "@example.com", "password": "passw0rd!",
	}, "")
	if r.status != 200 {
		e.t.Fatalf("login: %d %s", r.status, r.raw)
	}
	return dataMap(r)["access_token"].(string)
}

// setupHost 创建可用时间集（周一 09:00-18:00 上海）与预约类型，返回 slug。
func (e *env) setupHost(token string) string {
	e.t.Helper()
	r := e.do("POST", "/api/v1/availability-sets", map[string]any{
		"name": "工作时间", "timezone": "Asia/Shanghai", "is_default": true,
		"rules": []map[string]any{{"weekday": 1, "start_minute": 540, "end_minute": 1080}},
	}, token)
	if r.status != http.StatusCreated {
		e.t.Fatalf("availability: %d %s", r.status, r.raw)
	}
	availID := int64(dataMap(r)["id"].(float64))
	r = e.do("POST", "/api/v1/event-types", map[string]any{
		"slug": "intro", "name": "介绍会谈", "duration_minutes": 60,
		"availability_id": availID, "min_notice_minutes": 0,
	}, token)
	if r.status != http.StatusCreated {
		e.t.Fatalf("event type: %d %s", r.status, r.raw)
	}
	return "intro"
}

// firstFreeSlot 调 slots 接口拿未来第一个可用槽位（跳过指定槽）。
func (e *env) firstFreeSlot(skip map[string]bool) string {
	e.t.Helper()
	start := time.Now().UTC().Format("2006-01-02")
	r := e.do("GET", "/api/v1/pub/u/rick/intro/slots?start="+start+"&days=28&tz=Asia/Shanghai", nil, "")
	if r.status != 200 {
		e.t.Fatalf("slots: %d %s", r.status, r.raw)
	}
	days, _ := dataMap(r)["days"].([]any)
	for _, d := range days {
		dm, _ := d.(map[string]any)
		slots, _ := dm["slots"].([]any)
		for _, s := range slots {
			sm, _ := s.(map[string]any)
			st, _ := sm["start"].(string)
			if !skip[st] {
				return st
			}
		}
	}
	e.t.Fatal("28 天内无可用槽位")
	return ""
}

func TestBookingFlow(t *testing.T) {
	e := newEnv(t)
	hostTok := e.newUser("rick")
	e.setupHost(hostTok)

	// 公开主页：含类型摘要
	if r := e.do("GET", "/api/v1/pub/u/rick", nil, ""); r.status != 200 {
		t.Fatalf("pub profile: %d %s", r.status, r.raw)
	} else {
		ets := dataMap(r)["event_types"].([]any)
		if len(ets) != 1 || ets[0].(map[string]any)["slug"] != "intro" {
			t.Fatalf("event_types: %s", r.raw)
		}
	}
	// 不存在的主人
	if r := e.do("GET", "/api/v1/pub/u/nobody", nil, ""); errCode(r) != "booking/host_not_found" {
		t.Fatalf("host_not_found: %s", r.raw)
	}
	// 类型详情：availability 与 buffers
	if r := e.do("GET", "/api/v1/pub/u/rick/intro", nil, ""); r.status != 200 {
		t.Fatalf("pub type: %d %s", r.status, r.raw)
	} else if dataMap(r)["availability"].(map[string]any)["timezone"] != "Asia/Shanghai" {
		t.Fatalf("availability: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/pub/u/rick/none", nil, ""); errCode(r) != "booking/event_type_not_found" {
		t.Fatalf("bad slug: %s", r.raw)
	}
	// slots 参数校验
	if r := e.do("GET", "/api/v1/pub/u/rick/intro/slots?start=bad&days=7", nil, ""); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad start: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/pub/u/rick/intro/slots?start=2026-12-07&days=99", nil, ""); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad days: %s", r.raw)
	}

	slotA := e.firstFreeSlot(nil)
	slotB := e.firstFreeSlot(map[string]bool{slotA: true})

	// 校验分支：缺姓名 / 坏邮箱 / 坏时间
	post := func(body map[string]any) result {
		return e.do("POST", "/api/v1/pub/u/rick/intro/bookings", body, "")
	}
	if r := post(map[string]any{"start": slotA, "name": "", "email": "g@example.com"}); errCode(r) != "common/validation_failed" {
		t.Fatalf("no name: %s", r.raw)
	}
	if r := post(map[string]any{"start": slotA, "name": "客人", "email": "bad"}); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad email: %s", r.raw)
	}
	if r := post(map[string]any{"start": "not-a-time", "name": "客人", "email": "g@example.com"}); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad start: %s", r.raw)
	}

	// 蜜罐：假 201、不入库
	if r := post(map[string]any{"start": slotB, "name": "机器人", "email": "bot@example.com", "company": "spam inc"}); r.status != http.StatusCreated {
		t.Fatalf("honeypot should fake 201: %d %s", r.status, r.raw)
	}
	var n int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM bookings WHERE guest_email='bot@example.com'`).Scan(&n)
	if n != 0 {
		t.Fatal("蜜罐请求不得入库")
	}

	// 下单成功：booking + manage_url
	r := post(map[string]any{"start": slotA, "name": "客人甲", "email": "guest@example.com", "note": "聊聊", "timezone": "Asia/Shanghai"})
	if r.status != http.StatusCreated {
		t.Fatalf("book: %d %s", r.status, r.raw)
	}
	d := dataMap(r)
	manageURL, _ := d["manage_url"].(string)
	if !strings.HasPrefix(manageURL, "/b/") {
		t.Fatalf("manage_url: %s", r.raw)
	}
	bk := d["booking"].(map[string]any)
	if bk["event_type_name"] != "介绍会谈" || bk["start"] != slotA {
		t.Fatalf("booking view: %s", r.raw)
	}

	// 同槽位再约 → 409 slot_unavailable
	if r := post(map[string]any{"start": slotA, "name": "客人乙", "email": "g2@example.com"}); r.status != http.StatusConflict || errCode(r) != "booking/slot_unavailable" {
		t.Fatalf("double book: %d %s", r.status, r.raw)
	}
	// 槽位从列表消失
	if got := e.firstFreeSlot(nil); got == slotA {
		t.Fatal("已约槽位不应再可用")
	}

	// 主人视角列表
	if r := e.do("GET", "/api/v1/me/bookings", nil, hostTok); r.status != 200 {
		t.Fatalf("me/bookings: %d %s", r.status, r.raw)
	} else if r.body["total"].(float64) != 1 {
		t.Fatalf("me/bookings total: %s", r.raw)
	}
	// 状态过滤
	if r := e.do("GET", "/api/v1/me/bookings?status=cancelled", nil, hostTok); r.body["total"].(float64) != 0 {
		t.Fatalf("filter cancelled: %s", r.raw)
	}
	if r := e.do("GET", "/api/v1/me/bookings?status=zzz", nil, hostTok); errCode(r) != "common/validation_failed" {
		t.Fatalf("bad status: %s", r.raw)
	}

	// 事件已写入主人日历
	var evCount int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM events e
		JOIN calendars c ON c.id = e.calendar_id JOIN users u ON u.id = c.owner_id
		WHERE u.username='rick' AND e.summary LIKE '%客人甲'`).Scan(&evCount)
	if evCount != 1 {
		t.Fatalf("预约事件应写入主人日历: %d", evCount)
	}

	// 管理页视图
	token := strings.TrimPrefix(manageURL, "/b/")
	if r := e.do("GET", "/api/v1/pub/b/"+token, nil, ""); r.status != 200 {
		t.Fatalf("manage view: %d %s", r.status, r.raw)
	} else {
		mv := dataMap(r)
		if mv["host_username"] != "rick" || mv["guest_name"] != "客人甲" {
			t.Fatalf("manage view data: %s", r.raw)
		}
	}
	if r := e.do("GET", "/api/v1/pub/b/deadbeef", nil, ""); errCode(r) != "booking/not_found" {
		t.Fatalf("bad token: %s", r.raw)
	}

	// 改期到 slotB
	r = e.do("POST", "/api/v1/pub/b/"+token+"/reschedule", map[string]any{"start": slotB}, "")
	if r.status != 200 {
		t.Fatalf("reschedule: %d %s", r.status, r.raw)
	}
	if dataMap(r)["start"] != slotB {
		t.Fatalf("reschedule start: %s", r.raw)
	}
	// 旧槽恢复可用、新槽被占
	if got := e.firstFreeSlot(nil); got != slotA {
		t.Fatalf("改期后旧槽应恢复，期望 %s 实得 %s", slotA, got)
	}
	// 改期到已占槽（自己占用的 slotB 排除自身后仍冲突——先占 slotB2）
	slotC := e.firstFreeSlot(map[string]bool{slotA: true, slotB: true})
	if r := e.do("POST", "/api/v1/pub/u/rick/intro/bookings", map[string]any{
		"start": slotC, "name": "客人丙", "email": "g3@example.com"}, ""); r.status != http.StatusCreated {
		t.Fatalf("book C: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/pub/b/"+token+"/reschedule", map[string]any{"start": slotC}, ""); r.status != http.StatusConflict {
		t.Fatalf("reschedule to occupied: %d %s", r.status, r.raw)
	}

	// 取消（客人）：事件置 CANCELLED、槽位恢复、再取消 409
	if r := e.do("POST", "/api/v1/pub/b/"+token+"/cancel", map[string]string{"reason": "有事"}, ""); r.status != 200 {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	var status string
	_ = e.a.DB.QueryRow(`SELECT e.status FROM events e WHERE e.summary LIKE '%客人甲'`).Scan(&status)
	if status != "cancelled" {
		t.Fatalf("事件应置 cancelled: %s", status)
	}
	if r := e.do("POST", "/api/v1/pub/b/"+token+"/cancel", nil, ""); r.status != http.StatusConflict || errCode(r) != "booking/already_cancelled" {
		t.Fatalf("double cancel: %d %s", r.status, r.raw)
	}
	if got := e.firstFreeSlot(nil); got != slotA {
		t.Fatalf("取消后槽位应恢复，期望 %s 实得 %s", slotA, got)
	}
	// 已取消的预约不能再改期
	if r := e.do("POST", "/api/v1/pub/b/"+token+"/reschedule", map[string]any{"start": slotA}, ""); errCode(r) != "booking/already_cancelled" {
		t.Fatalf("reschedule cancelled: %s", r.raw)
	}

	// 主人取消客人丙的预约（用 me/bookings 拿 id）
	r = e.do("GET", "/api/v1/me/bookings?status=confirmed", nil, hostTok)
	items := r.body["data"].([]any)
	bookingID := int64(items[0].(map[string]any)["id"].(float64))
	if r := e.do("POST", fmt.Sprintf("/api/v1/bookings/%d/cancel", bookingID), map[string]string{"reason": "主人有事"}, hostTok); r.status != 200 {
		t.Fatalf("host cancel: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", fmt.Sprintf("/api/v1/bookings/%d/cancel", bookingID), nil, hostTok); errCode(r) != "booking/already_cancelled" {
		t.Fatalf("host double cancel: %s", r.raw)
	}

	// 邮件出箱：确认 ×2、改期 ×2、取消 ×2（双方）
	for _, kind := range []string{"booking_confirmed", "booking_rescheduled", "booking_cancelled"} {
		var cnt int
		if err := e.a.DB.QueryRow(`SELECT COUNT(*) FROM mail_outbox WHERE kind=?`, kind).Scan(&cnt); err != nil || cnt < 2 {
			t.Fatalf("outbox %s = %d err=%v", kind, cnt, err)
		}
	}
	// ICS 附件留档：正文包含 VCALENDAR 与 METHOD
	var body string
	_ = e.a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='booking_confirmed' LIMIT 1`).Scan(&body)
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "METHOD:REQUEST") {
		t.Fatalf("确认邮件应附 ICS: %.200s", body)
	}
}

// 并发下单同一槽位：恰好一个 201 一个 409。
func TestBookingConcurrentSameSlot(t *testing.T) {
	e := newEnv(t)
	hostTok := e.newUser("rick")
	e.setupHost(hostTok)

	slot := e.firstFreeSlot(nil)
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- e.do("POST", "/api/v1/pub/u/rick/intro/bookings", map[string]any{
				"start": slot, "name": fmt.Sprintf("客人%d", i), "email": fmt.Sprintf("c%d@example.com", i),
			}, "")
		}(i)
	}
	wg.Wait()
	close(results)
	var created, conflict int
	for r := range results {
		switch r.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("意外状态: %d %s", r.status, r.raw)
		}
	}
	if created != 1 || conflict != 1 {
		t.Fatalf("并发下单期望 1×201+1×409，实得 %d×201+%d×409", created, conflict)
	}
	var n int
	_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM bookings WHERE status='confirmed'`).Scan(&n)
	if n != 1 {
		t.Fatalf("应恰有一条已确认预约，实得 %d", n)
	}
}
