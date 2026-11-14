// booking 包槽位计算测试：区间合并/扣减、每周规则与日期覆盖、
// 可预约范围（rolling/fixed）、ValidateSlot 错误码顺序、busy 三来源
// （日历事件/外部源/已确认预约）、缓冲与最短通知、时区边界分桶。
package booking

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// summarizeDays 输出每天日期与档数（失败信息用）。
func summarizeDays(days []DaySlots) string {
	s := ""
	for _, d := range days {
		s += d.Date + ":" + itoa(len(d.Slots)) + " "
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func mustT(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// ---- 纯函数：区间合并与扣减 ----

func TestMergeIntervals(t *testing.T) {
	iv := func(a, b string) interval { return interval{mustT(a), mustT(b)} }
	in := []interval{
		iv("2026-12-07T03:00:00Z", "2026-12-07T04:00:00Z"),
		iv("2026-12-07T01:00:00Z", "2026-12-07T02:00:00Z"),
		iv("2026-12-07T02:00:00Z", "2026-12-07T02:30:00Z"), // 相接合并
		iv("2026-12-07T05:00:00Z", "2026-12-07T06:00:00Z"),
	}
	out := mergeIntervals(in)
	if len(out) != 3 {
		t.Fatalf("期望 3 段，实得 %d: %v", len(out), out)
	}
	if !out[0].start.Equal(mustT("2026-12-07T01:00:00Z")) || !out[0].end.Equal(mustT("2026-12-07T02:30:00Z")) {
		t.Fatalf("首段应为 01:00-02:30: %v", out[0])
	}
}

func TestSubtractIntervals(t *testing.T) {
	iv := func(a, b string) interval { return interval{mustT(a), mustT(b)} }
	base := []interval{iv("2026-12-07T01:00:00Z", "2026-12-07T08:00:00Z")}
	// 中间挖空
	got := subtractIntervals(base, []interval{iv("2026-12-07T03:00:00Z", "2026-12-07T04:00:00Z")})
	if len(got) != 2 || !got[0].end.Equal(mustT("2026-12-07T03:00:00Z")) || !got[1].start.Equal(mustT("2026-12-07T04:00:00Z")) {
		t.Fatalf("挖空结果错误: %v", got)
	}
	// 完全覆盖 → 空
	if got := subtractIntervals(base, []interval{iv("2026-12-07T00:00:00Z", "2026-12-07T09:00:00Z")}); len(got) != 0 {
		t.Fatalf("完全覆盖应清空: %v", got)
	}
	// 无交集 → 原样
	if got := subtractIntervals(base, []interval{iv("2026-12-08T01:00:00Z", "2026-12-08T02:00:00Z")}); len(got) != 1 {
		t.Fatalf("无交集应保留: %v", got)
	}
	// 两端各切一刀 → 三段
	got = subtractIntervals(base, []interval{
		iv("2026-12-07T01:30:00Z", "2026-12-07T02:00:00Z"),
		iv("2026-12-07T06:00:00Z", "2026-12-07T07:00:00Z"),
	})
	if len(got) != 3 {
		t.Fatalf("两刀应得三段: %v", got)
	}
}

// ---- intervalsForLocalDate：每周规则 + 日期覆盖 ----

func TestIntervalsForLocalDate(t *testing.T) {
	// 2026-12-07 是周一（Asia/Shanghai）
	set := &AvailabilitySet{
		ID: 1, OwnerID: 1, Name: "工作", Timezone: "Asia/Shanghai",
		Rules: []Rule{
			{Weekday: 1, StartMinute: 9 * 60, EndMinute: 12 * 60},
			{Weekday: 1, StartMinute: 14 * 60, EndMinute: 18 * 60},
		},
	}
	day := time.Date(2026, 12, 7, 20, 0, 0, 0, time.UTC) // 任意时刻，取本地日期
	ivs := intervalsForLocalDate(set, day)
	if len(ivs) != 2 {
		t.Fatalf("期望 2 段，实得 %d", len(ivs))
	}
	sh, _ := time.LoadLocation("Asia/Shanghai")
	wantStart := time.Date(2026, 12, 7, 9, 0, 0, 0, sh)
	if !ivs[0].start.Equal(wantStart) {
		t.Fatalf("首段开始应为上海 09:00（=01:00Z）: %v", ivs[0].start.UTC())
	}

	// 日期覆盖：available 追加 12:00-14:00（与两段相接连成 09:00-18:00），
	// unavailable 扣掉 16:00-17:00
	set.Dates = []DateRule{
		{Date: "2026-12-07", Kind: "available", StartMinute: 12 * 60, EndMinute: 14 * 60},
		{Date: "2026-12-07", Kind: "unavailable", StartMinute: 16 * 60, EndMinute: 17 * 60},
	}
	ivs = intervalsForLocalDate(set, day)
	if len(ivs) != 2 {
		t.Fatalf("覆盖后期望 2 段（09-16 与 17-18），实得 %d: %v", len(ivs), ivs)
	}
	if !ivs[0].start.Equal(wantStart) || !ivs[0].end.Equal(time.Date(2026, 12, 7, 16, 0, 0, 0, sh)) {
		t.Fatalf("首段应为 09:00-16:00: %v → %v", ivs[0].start.In(sh), ivs[0].end.In(sh))
	}
	if !ivs[1].start.Equal(time.Date(2026, 12, 7, 17, 0, 0, 0, sh)) {
		t.Fatalf("次段应从 17:00 开始: %v", ivs[1].start.In(sh))
	}

	// 其他日期不受覆盖影响
	other := intervalsForLocalDate(set, time.Date(2026, 12, 14, 20, 0, 0, 0, time.UTC))
	if len(other) != 2 || other[0].end.Equal(time.Date(2026, 12, 14, 12, 0, 0, 0, sh)) == false {
		t.Fatalf("无覆盖日期应保持每周规则: %v", other)
	}
}

// ---- rangeBounds：rolling 与 fixed ----

func TestRangeBounds(t *testing.T) {
	set := &AvailabilitySet{Timezone: "Asia/Shanghai"}
	now := mustT("2026-12-05T10:00:00Z")

	rolling := &EventType{RangeType: "rolling", RangeDays: 30}
	min, max, ok := rangeBounds(rolling, set, now)
	if !ok || !min.Equal(now) || !max.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("rolling 边界错误: %v %v %v", min, max, ok)
	}
	// 缺省 60 天
	rolling.RangeDays = 0
	_, max, _ = rangeBounds(rolling, set, now)
	if !max.Equal(now.Add(60*24*time.Hour)) {
		t.Fatalf("缺省 60 天错误: %v", max)
	}

	sd, ed := "2026-12-07", "2026-12-13"
	fixed := &EventType{RangeType: "fixed", StartDate: &sd, EndDate: &ed}
	min, max, ok = rangeBounds(fixed, set, now)
	sh, _ := time.LoadLocation("Asia/Shanghai")
	if !ok || !min.Equal(time.Date(2026, 12, 7, 0, 0, 0, 0, sh)) {
		t.Fatalf("fixed min 错误: %v", min)
	}
	if !max.Equal(time.Date(2026, 12, 14, 0, 0, 0, 0, sh)) {
		t.Fatalf("fixed max 应为结束日期次日零点: %v", max)
	}
	// 缺日期 → 不可预约
	fixed.StartDate = nil
	if _, _, ok := rangeBounds(fixed, set, now); ok {
		t.Fatal("fixed 缺日期应不可预约")
	}
}

// ---- DB 环境 ----

type slotEnv struct {
	t     *testing.T
	db    *sql.DB
	host  *Host
	calID int64
	set   *AvailabilitySet
}

func newSlotEnv(t *testing.T) *slotEnv {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	now := cal.NowUTCString()
	if _, err := database.Exec(`INSERT INTO users (email, username, password_hash, display_name, timezone, created_at, updated_at)
		VALUES ('host@example.com','host','x','Host','Asia/Shanghai',?,?)`, now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var hostID int64
	if err := database.QueryRow(`SELECT id FROM users WHERE username='host'`).Scan(&hostID); err != nil {
		t.Fatalf("host id: %v", err)
	}
	c, err := cal.CreateCalendar(context.Background(), database, hostID, "主日历", "", "", "Asia/Shanghai")
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	return &slotEnv{
		t: t, db: database,
		host:  &Host{ID: hostID, Username: "host", Timezone: "Asia/Shanghai"},
		calID: c.ID,
		set: &AvailabilitySet{
			ID: 1, OwnerID: hostID, Name: "工作", Timezone: "Asia/Shanghai",
			Rules: []Rule{{Weekday: 1, StartMinute: 9 * 60, EndMinute: 18 * 60}}, // 周一 09:00-18:00
		},
	}
}

// addBusyEvent 在主人日历上插入一个忙碌事件（上海时间字面量转 UTC 传入）。
func (e *slotEnv) addBusyEvent(startUTC, endUTC time.Time) {
	e.t.Helper()
	_, err := cal.CreateEventFromInput(context.Background(), e.db, e.calID, "busy-"+startUTC.Format("150405"), cal.EventInput{
		Summary: "忙碌", Start: startUTC, End: endUTC, TZ: "Asia/Shanghai",
	})
	if err != nil {
		e.t.Fatalf("add busy event: %v", err)
	}
}

func (e *slotEnv) validate(et *EventType, start, now time.Time) error {
	return ValidateSlot(context.Background(), e.db, e.host, et, e.set, start, now, 0, 0)
}

// ---- ValidateSlot：错误码顺序与 busy 交集 ----

func TestValidateSlotErrorPrecedence(t *testing.T) {
	e := newSlotEnv(t)
	now := mustT("2026-12-05T10:00:00Z") // 周六
	monday9 := mustT("2026-12-07T01:00:00Z")
	et := &EventType{ID: 1, DurationMinutes: 60, MinNoticeMinutes: 120, RangeType: "rolling", RangeDays: 30}

	// 1) 过早（rolling 下界=now）→ outside_range
	if c := errCode(e.validate(et, now.Add(-time.Hour), now)); c != "booking/outside_range" {
		t.Fatalf("过早应 outside_range，实得 %q", c)
	}
	// 2) 太近 → insufficient_notice（在范围内但通知不足）
	if c := errCode(e.validate(et, now.Add(30*time.Minute), now)); c != "booking/insufficient_notice" {
		t.Fatalf("太近应 insufficient_notice，实得 %q", c)
	}
	// 3) 通知足够但不在可用时段（周二）→ invalid_slot
	if c := errCode(e.validate(et, mustT("2026-12-08T01:00:00Z"), now)); c != "booking/invalid_slot" {
		t.Fatalf("非可用时段应 invalid_slot，实得 %q", c)
	}
	// 4) 可用时段但与日历事件冲突 → slot_unavailable
	e.addBusyEvent(monday9, monday9.Add(time.Hour))
	if c := errCode(e.validate(et, monday9, now)); c != "booking/slot_unavailable" {
		t.Fatalf("冲突应 slot_unavailable，实得 %q", c)
	}
	// 后一小时可用
	if err := e.validate(et, monday9.Add(time.Hour), now); err != nil {
		t.Fatalf("10:00 应可用: %v", err)
	}
}

// errCode 从 httpx.Error 提取稳定错误码。
func errCode(err error) string {
	if err == nil {
		return ""
	}
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	return err.Error()
}

// ---- 缓冲 ----

func TestValidateSlotBuffers(t *testing.T) {
	e := newSlotEnv(t)
	now := mustT("2026-12-05T10:00:00Z")
	monday11 := mustT("2026-12-07T03:00:00Z")               // 上海 11:00
	e.addBusyEvent(monday11, monday11.Add(time.Hour))        // 11:00-12:00 忙

	noBuf := &EventType{ID: 1, DurationMinutes: 60, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}
	// 无缓冲：09:00-10:00 与 11:00 事件不相交 → 可用
	if err := e.validate(noBuf, mustT("2026-12-07T01:00:00Z"), now); err != nil {
		t.Fatalf("无缓冲 09:00 应可用: %v", err)
	}
	// BufferBefore=120（会话结束后到下一忙碌事件的间隙）：09:00 结束 +120m
	// 缓冲要求 10:00 ≤ 11:00-120m（=09:00），不满足 → 冲突
	before := &EventType{ID: 2, DurationMinutes: 60, BufferBefore: 120, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}
	if err := e.validate(before, mustT("2026-12-07T01:00:00Z"), now); errCode(err) != "booking/slot_unavailable" {
		t.Fatalf("BufferBefore 相交应冲突: %v", err)
	}
	// BufferAfter=120（忙碌事件结束后到下一会话的间隙）：12:00 结束 +120m，
	// 13:00 开始的会话不满足 → 冲突；14:00 开始满足
	after := &EventType{ID: 3, DurationMinutes: 60, BufferAfter: 120, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}
	if err := e.validate(after, mustT("2026-12-07T05:00:00Z"), now); errCode(err) != "booking/slot_unavailable" {
		t.Fatalf("BufferAfter 相交应冲突: %v", err)
	}
	if err := e.validate(after, mustT("2026-12-07T06:00:00Z"), now); err != nil {
		t.Fatalf("14:00 满足 BufferAfter 应可用: %v", err)
	}
}

// ---- rolling 与 fixed 范围 ----

func TestValidateSlotRangeTypes(t *testing.T) {
	e := newSlotEnv(t)
	now := mustT("2026-12-05T10:00:00Z")
	et := &EventType{ID: 1, DurationMinutes: 60, RangeType: "rolling", RangeDays: 7, MinNoticeMinutes: 0}
	// 第 8 天的周一（12-14）超出 rolling 7 天
	if c := errCode(e.validate(et, mustT("2026-12-14T01:00:00Z"), now)); c != "booking/outside_range" {
		t.Fatalf("rolling 超界应 outside_range，实得 %q", c)
	}
	sd, ed := "2026-12-07", "2026-12-13"
	fixed := &EventType{ID: 2, DurationMinutes: 60, RangeType: "fixed", StartDate: &sd, EndDate: &ed, MinNoticeMinutes: 0}
	if err := e.validate(fixed, mustT("2026-12-07T01:00:00Z"), now); err != nil {
		t.Fatalf("fixed 窗口内应可用: %v", err)
	}
	if c := errCode(e.validate(fixed, mustT("2026-12-14T01:00:00Z"), now)); c != "booking/outside_range" {
		t.Fatalf("fixed 超界应 outside_range，实得 %q", c)
	}
}

// ---- busy 三来源：日历事件 / 外部源 / 已确认预约 ----

func TestCollectBusySources(t *testing.T) {
	e := newSlotEnv(t)
	now := mustT("2026-12-05T10:00:00Z")
	et := &EventType{ID: 1, DurationMinutes: 60, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}
	ctx := context.Background()

	// 外部源忙碌：周一 13:00-14:00（上海）= 05:00-06:00Z
	if _, err := e.db.Exec(`INSERT INTO external_feeds (user_id, name, url, created_at) VALUES (?,?,?,?)`,
		e.host.ID, "外部", "https://example.com/x.ics", cal.NowUTCString()); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, err := e.db.Exec(`INSERT INTO feed_events (feed_id, uid, busy, dtstart, dtend)
		SELECT id, 'ext-1', 1, '2026-12-07T05:00:00Z', '2026-12-07T06:00:00Z' FROM external_feeds WHERE user_id=?`, e.host.ID); err != nil {
		t.Fatalf("feed event: %v", err)
	}
	// 已确认预约：周一 15:00-16:00（上海）= 07:00-08:00Z
	res, err := e.db.Exec(`INSERT INTO event_types (user_id, slug, name, duration_minutes, calendar_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, e.host.ID, "intro", "介绍", 60, e.calID, cal.NowUTCString(), cal.NowUTCString())
	if err != nil {
		t.Fatalf("event type: %v", err)
	}
	etID, _ := res.LastInsertId()
	if _, err := e.db.Exec(`INSERT INTO bookings (event_type_id, host_id, calendar_id, guest_name, guest_email,
		start_utc, end_utc, status, manage_token_hash, ics_uid, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		etID, e.host.ID, e.calID, "客人", "guest@example.com",
		"2026-12-07T07:00:00Z", "2026-12-07T08:00:00Z", "confirmed", "tok-hash", "ics-uid",
		cal.NowUTCString(), cal.NowUTCString()); err != nil {
		t.Fatalf("booking: %v", err)
	}

	// 三个来源分别挡住各自时段，其余可用
	busyMap := map[string]string{
		"2026-12-07T05:00:00Z": "booking/slot_unavailable", // 外部源
		"2026-12-07T07:00:00Z": "booking/slot_unavailable", // 已确认预约
		"2026-12-07T01:00:00Z": "",                          // 空闲
	}
	for start, want := range busyMap {
		got := errCode(e.validate(et, mustT(start), now))
		if got != want {
			t.Fatalf("%s 期望 %q 实得 %q", start, want, got)
		}
	}

	// 日历事件来源：追加周一 09:00 事件
	e.addBusyEvent(mustT("2026-12-07T01:00:00Z"), mustT("2026-12-07T02:00:00Z"))
	if c := errCode(e.validate(et, mustT("2026-12-07T01:00:00Z"), now)); c != "booking/slot_unavailable" {
		t.Fatalf("日历事件来源应冲突，实得 %q", c)
	}

	// 已取消预约不算 busy
	if _, err := e.db.Exec(`UPDATE bookings SET status='cancelled' WHERE event_type_id=?`, etID); err != nil {
		t.Fatalf("cancel booking: %v", err)
	}
	if err := e.validate(et, mustT("2026-12-07T07:00:00Z"), now); err != nil {
		t.Fatalf("取消后的预约不应占用: %v", err)
	}
	_ = ctx
}

// ---- ComputeSlots：分桶、最短通知、时区边界 ----

func TestComputeSlots(t *testing.T) {
	e := newSlotEnv(t)
	// 上海周一 09:00-18:00 → 01:00Z..10:00Z，60 分钟一档
	now := mustT("2026-12-05T10:00:00Z")
	et := &EventType{ID: 1, DurationMinutes: 60, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}

	days, err := ComputeSlots(context.Background(), e.db, e.host, et, e.set, "UTC",
		time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), 1, now)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if len(days) != 1 || days[0].Date != "2026-12-07" {
		t.Fatalf("分桶错误: %+v", days)
	}
	if n := len(days[0].Slots); n != 9 { // 09..17 开头共 9 档
		t.Fatalf("期望 9 档，实得 %d: %+v", n, days[0].Slots)
	}
	if days[0].Slots[0].Start != "2026-12-07T01:00:00Z" || days[0].Slots[8].Start != "2026-12-07T09:00:00Z" {
		t.Fatalf("槽位起止错误: %+v", days[0].Slots)
	}

	// busy 事件隐藏对应槽位（09:00 上海档被占）
	e.addBusyEvent(mustT("2026-12-07T01:00:00Z"), mustT("2026-12-07T02:00:00Z"))
	days, _ = ComputeSlots(context.Background(), e.db, e.host, et, e.set, "UTC",
		time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), 1, now)
	if n := len(days[0].Slots); n != 8 || days[0].Slots[0].Start != "2026-12-07T02:00:00Z" {
		t.Fatalf("busy 后应 8 档且从 02:00 开始: %+v", days[0].Slots)
	}

	// 最短通知：now=周一 02:30Z + notice 120m → 只保留 04:30Z 之后的档（04:00 档 < 04:30 隐藏）
	notice := &EventType{ID: 2, DurationMinutes: 60, MinNoticeMinutes: 120, RangeType: "rolling", RangeDays: 30}
	days, _ = ComputeSlots(context.Background(), e.db, e.host, notice, e.set, "UTC",
		time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), 1, mustT("2026-12-07T02:30:00Z"))
	for _, s := range days[0].Slots {
		if s.Start < "2026-12-07T04:30:00Z" {
			t.Fatalf("最短通知不足的槽位应隐藏: %s", s.Start)
		}
	}
	if len(days[0].Slots) == 0 {
		t.Fatal("通知过滤后应有剩余槽位")
	}

	// slot_interval=120：60 分钟时长但两小时一档；此前已添加 01:00-02:00Z
	// 忙碌事件，档位序列应为 03:00、05:00、07:00、09:00
	step := &EventType{ID: 3, DurationMinutes: 60, SlotInterval: 120, MinNoticeMinutes: 0, RangeType: "rolling", RangeDays: 30}
	days, _ = ComputeSlots(context.Background(), e.db, e.host, step, e.set, "UTC",
		time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), 1, now)
	wantStarts := []string{"03:00:00", "05:00:00", "07:00:00", "09:00:00"}
	if len(days[0].Slots) != len(wantStarts) {
		t.Fatalf("interval=120 档位数错误: %+v", days[0].Slots)
	}
	for i, s := range days[0].Slots {
		if want := "2026-12-07T" + wantStarts[i] + "Z"; s.Start != want {
			t.Fatalf("interval=120 档位错误: 期望 %s 实得 %s", want, s.Start)
		}
	}

	// grid 时区边界：上海 12-07 的深夜档（无需覆盖到 12-08 桶）
	gridSH, _ := time.LoadLocation("Asia/Shanghai")
	days, _ = ComputeSlots(context.Background(), e.db, e.host, et, e.set, "Asia/Shanghai",
		time.Date(2026, 12, 7, 0, 0, 0, 0, gridSH), 1, now)
	if len(days) != 1 || days[0].Date != "2026-12-07" || len(days[0].Slots) == 0 {
		t.Fatalf("上海 grid 分桶错误: %+v", days)
	}
	// 跨时区分桶：补周二规则后，上海 12-08 的 09:00 档落在 UTC 12-08 桶（01:00Z）
	set2 := *e.set
	set2.Rules = append([]Rule(nil), e.set.Rules...)
	set2.Rules = append(set2.Rules, Rule{Weekday: 2, StartMinute: 9 * 60, EndMinute: 18 * 60})
	days, _ = ComputeSlots(context.Background(), e.db, e.host, et, &set2, "UTC",
		time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), 2, now)
	if len(days) != 2 || days[1].Date != "2026-12-08" || len(days[1].Slots) != 9 {
		t.Fatalf("跨日分桶错误: %v", summarizeDays(days))
	}
	if days[1].Slots[0].Start != "2026-12-08T01:00:00Z" {
		t.Fatalf("第二桶首档应为 UTC 01:00: %s", days[1].Slots[0].Start)
	}
}
