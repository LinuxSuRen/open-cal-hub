package cal

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// buildRecurRow 构造一个带 RRULE 的主事件行（原文为准，索引列仅用于兜底）。
func buildRecurRow(t *testing.T, id int64, rrule string, start time.Time) *EventRow {
	t.Helper()
	tz := "UTC"
	data := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:%s\r\nBEGIN:VEVENT\r\nUID:rr%d@t\r\n"+
		"DTSTART:%s\r\nDTEND:%s\r\nRRULE:%s\r\nSUMMARY:rec\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		ProductID, id, start.UTC().Format("20060102T150405Z"),
		start.UTC().Add(time.Hour).Format("20060102T150405Z"), rrule)
	return &EventRow{
		ID: id, CalendarID: 1, UID: fmt.Sprintf("rr%d@t", id), Data: data,
		Summary: "rec", Start: start.UTC(), End: start.UTC().Add(time.Hour),
		TZ: tz, RRule: rrule, Status: StatusConfirmed,
	}
}

func mustUTC(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// DAILY + COUNT：恰好展开 count 个实例，间隔一天。
func TestExpandDailyCount(t *testing.T) {
	row := buildRecurRow(t, 1, "FREQ=DAILY;COUNT=5", mustUTC("2026-12-01T09:00:00Z"))
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), true)
	if len(got) != 5 {
		t.Fatalf("期望 5 个实例，实得 %d", len(got))
	}
	for i, inst := range got {
		want := mustUTC("2026-12-01T09:00:00Z").AddDate(0, 0, i)
		if inst.Start != want.Format(time.RFC3339) {
			t.Fatalf("实例 %d 期望 %s 实得 %s", i, want.Format(time.RFC3339), inst.Start)
		}
		if !inst.Recurring {
			t.Fatalf("实例 %d 应标记 recurring", i)
		}
	}
}

// WEEKLY + 多 BYDAY：每周展开 MO/WE/FRI 三个实例。
func TestExpandWeeklyMultiByDay(t *testing.T) {
	// 2026-12-01 是周二；从 12-03（周四）开始的周规则 MO/WE/FRI
	row := buildRecurRow(t, 1, "FREQ=WEEKLY;BYDAY=MO,WE,FR", mustUTC("2026-12-03T09:00:00Z"))
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-07T00:00:00Z"), mustUTC("2026-12-14T00:00:00Z"), true)
	// [12-07, 12-14)：07(MO) 09(WE) 11(FR) 14(MO 不含，右开)
	want := []string{
		"2026-12-07T09:00:00Z", "2026-12-09T09:00:00Z", "2026-12-11T09:00:00Z",
	}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 个实例，实得 %d：%v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Start != w {
			t.Fatalf("实例 %d 期望 %s 实得 %s", i, w, got[i].Start)
		}
	}
}

// MONTHLY + BYDAY 序数：每月第 2 个周五。
func TestExpandMonthlyNthByDay(t *testing.T) {
	// 2026-12 的第 2 个周五是 12-11
	row := buildRecurRow(t, 1, "FREQ=MONTHLY;BYDAY=2FR", mustUTC("2026-12-11T10:00:00Z"))
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2027-02-01T00:00:00Z"), true)
	want := []string{"2026-12-11T10:00:00Z", "2027-01-08T10:00:00Z"}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 个实例，实得 %d：%v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Start != w {
			t.Fatalf("实例 %d 期望 %s 实得 %s", i, w, got[i].Start)
		}
	}
}

// UNTIL：右开边界恰好在实例开始时刻时该实例不出现（rrule-go Between 语义含两端，这里验证总量）。
func TestExpandUntil(t *testing.T) {
	row := buildRecurRow(t, 1, "FREQ=DAILY;UNTIL=20261205T090000Z", mustUTC("2026-12-01T09:00:00Z"))
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), true)
	// 12-01..12-05 每日一个（UNTIL 含当日 09:00）
	if len(got) != 5 {
		t.Fatalf("期望 5 个实例，实得 %d：%v", len(got), got)
	}
	last := got[len(got)-1].Start
	if last != "2026-12-05T09:00:00Z" {
		t.Fatalf("最后实例期望 2026-12-05T09:00:00Z 实得 %s", last)
	}
}

// EXDATE：排除的实例不出现，其余照常。
func TestExpandExDate(t *testing.T) {
	start := mustUTC("2026-12-01T09:00:00Z")
	data := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:%s\r\nBEGIN:VEVENT\r\nUID:ex@t\r\n"+
		"DTSTART:%s\r\nDTEND:%s\r\nRRULE:FREQ=DAILY;COUNT=5\r\n"+
		"EXDATE:20261202T090000Z\r\nEXDATE:20261204T090000Z\r\nSUMMARY:rec\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		ProductID, start.UTC().Format("20060102T150405Z"), start.UTC().Add(time.Hour).Format("20060102T150405Z"))
	row := &EventRow{
		ID: 1, CalendarID: 1, UID: "ex@t", Data: data,
		Summary: "rec", Start: start, End: start.Add(time.Hour),
		TZ: "UTC", RRule: "FREQ=DAILY;COUNT=5", Status: StatusConfirmed,
	}
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), true)
	var starts []string
	for _, inst := range got {
		starts = append(starts, inst.Start)
	}
	// 12-02 与 12-04 被 EXDATE 排除
	want := []string{"2026-12-01T09:00:00Z", "2026-12-03T09:00:00Z", "2026-12-05T09:00:00Z"}
	if len(starts) != len(want) {
		t.Fatalf("期望 %v 实得 %v", want, starts)
	}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("期望 %v 实得 %v", want, starts)
		}
	}
}

// 展开上限：无界 RRULE 在窗口内也最多 MaxExpandInstances 个实例（炸弹防护）。
func TestExpandInstanceCap(t *testing.T) {
	// 每分钟重复，跨度 3000 分钟 > 2000
	row := buildRecurRow(t, 1, "FREQ=MINUTELY", mustUTC("2026-12-01T00:00:00Z"))
	got := ExpandRange([]*EventRow{row}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-04T00:00:00Z"), true)
	if len(got) > MaxExpandInstances {
		t.Fatalf("实例数 %d 超过上限 %d", len(got), MaxExpandInstances)
	}
	if len(got) != MaxExpandInstances {
		t.Fatalf("期望封顶在 %d 个实例，实得 %d", MaxExpandInstances, len(got))
	}
}

// 覆盖行替换对应实例：同 UID 的 recurrence_id 行覆盖主事件展开出的该实例。
func TestExpandOverrideReplacesInstance(t *testing.T) {
	master := buildRecurRow(t, 1, "FREQ=DAILY;COUNT=3", mustUTC("2026-12-01T09:00:00Z"))
	rid := mustUTC("2026-12-02T09:00:00Z")
	// 覆盖行：改标题并顺延一小时
	ovData := fmt.Sprintf("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:%s\r\nBEGIN:VEVENT\r\nUID:rr1@t\r\n"+
		"RECURRENCE-ID:20261202T090000Z\r\nDTSTART:20261202T100000Z\r\nDTEND:20261202T110000Z\r\n"+
		"SUMMARY:改期\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", ProductID)
	override := &EventRow{
		ID: 2, CalendarID: 1, UID: "rr1@t", Data: ovData, RecurrenceID: &rid,
		Summary: "改期", Start: rid.Add(time.Hour), End: rid.Add(2 * time.Hour),
		TZ: "UTC", RRule: "", Status: StatusConfirmed,
	}
	got := ExpandRange([]*EventRow{master, override}, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), true)
	if len(got) != 3 {
		t.Fatalf("期望 3 个实例，实得 %d", len(got))
	}
	ov := got[1]
	if ov.Summary != "改期" || ov.Start != "2026-12-02T10:00:00Z" {
		t.Fatalf("覆盖实例内容错误: %+v", ov)
	}
	if ov.RecurrenceID == nil || *ov.RecurrenceID != "2026-12-02T09:00:00Z" {
		t.Fatalf("覆盖实例应带 recurrence_id: %+v", ov)
	}
	if ov.Recurring || ov.RRule != "" {
		t.Fatalf("覆盖实例不应标记重复: %+v", ov)
	}
	if ov.MasterID != master.ID {
		t.Fatalf("覆盖实例 master_id 应指向主事件 %d，实得 %d", master.ID, ov.MasterID)
	}
}

// includeCancelled=false 时取消状态实例被过滤，true 时保留。
func TestExpandCancelledFilter(t *testing.T) {
	master := buildRecurRow(t, 1, "FREQ=DAILY;COUNT=2", mustUTC("2026-12-01T09:00:00Z"))
	rid := mustUTC("2026-12-01T09:00:00Z")
	cancelData := strings.ReplaceAll(master.Data, "SUMMARY:rec", "SUMMARY:rec\r\nSTATUS:CANCELLED")
	cancelled := &EventRow{
		ID: 2, CalendarID: 1, UID: master.UID, Data: cancelData, RecurrenceID: &rid,
		Summary: "rec", Start: rid, End: rid.Add(time.Hour),
		TZ: "UTC", Status: StatusCancelled,
	}
	rows := []*EventRow{master, cancelled}
	if got := ExpandRange(rows, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), false); len(got) != 1 {
		t.Fatalf("过滤取消后期望 1 个实例，实得 %d", len(got))
	}
	if got := ExpandRange(rows, mustUTC("2026-12-01T00:00:00Z"), mustUTC("2026-12-31T00:00:00Z"), true); len(got) != 2 {
		t.Fatalf("含取消时期望 2 个实例，实得 %d", len(got))
	}
}

// CanonicalRRule：freq 大小写不敏感、interval>1、count/until 互斥。
func TestCanonicalRRule(t *testing.T) {
	if s, err := CanonicalRRule(&RRuleInput{Freq: "weekly", Byday: []string{"mo"}}); err != nil || !strings.HasPrefix(s, "FREQ=WEEKLY") {
		t.Fatalf("小写 freq 应被接受: %q %v", s, err)
	}
	if _, err := CanonicalRRule(&RRuleInput{Freq: "DAILY", Count: 3, Until: "2026-12-01T00:00:00Z"}); err == nil {
		t.Fatal("count 与 until 互斥应报错")
	}
	if _, err := CanonicalRRule(&RRuleInput{Freq: "HOURLY"}); err == nil {
		t.Fatal("不支持的 freq 应报错")
	}
	if _, err := CanonicalRRule(&RRuleInput{Freq: "DAILY", Count: MaxExpandInstances + 1}); err == nil {
		t.Fatal("count 超上限应报错")
	}
}
