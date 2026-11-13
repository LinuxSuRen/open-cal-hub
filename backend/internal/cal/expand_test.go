package cal

import (
	"testing"
	"time"
)

// 回归：每周一 09:00（Asia/Shanghai）系列在 11 月窗口应展开出 16 与 23 日两个实例
//（30 日实例因窗口右边界 11-30T00:00Z 早于其开始而不含）。
// 真实数据来自 2026-11-13 冒烟（见 docs/review-report.md）。
// 正确格式（RECUR 原生值）与历史 VALUE=TEXT 写法都必须通过。
func TestExpandRangeWeeklyByDay(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		data string
	}{
		{"recur_native", "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nDTEND;TZID=Asia/Shanghai:20261116T100000\r\nDTSTART;TZID=Asia/Shanghai:20261116T090000\r\nRRULE:FREQ=WEEKLY;BYDAY=MO\r\nSUMMARY:周会\r\nUID:calhub-test@exp\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"},
		{"legacy_value_text", "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nDTEND;TZID=Asia/Shanghai:20261116T100000\r\nDTSTART;TZID=Asia/Shanghai:20261116T090000\r\nRRULE;VALUE=TEXT:FREQ=WEEKLY\\;BYDAY=MO\r\nSUMMARY:周会\r\nUID:calhub-test@exp\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := &EventRow{
				ID: 1, CalendarID: 1, UID: "calhub-test@exp", Data: tc.data,
				Summary: "周会",
				Start:   time.Date(2026, 11, 16, 9, 0, 0, 0, tz).UTC(),
				End:     time.Date(2026, 11, 16, 10, 0, 0, 0, tz).UTC(),
				TZ:      "Asia/Shanghai", RRule: "FREQ=WEEKLY;BYDAY=MO", Status: "confirmed",
			}
			got := ExpandRange([]*EventRow{row}, from, to, true)
			if len(got) != 2 {
				t.Fatalf("期望 2 个实例（11-16、11-23），实得 %d: %+v", len(got), got)
			}
			for i, want := range []string{"2026-11-16T01:00:00Z", "2026-11-23T01:00:00Z"} {
				if got[i].Start != want {
					t.Fatalf("实例 %d 期望 %s 实得 %s", i, want, got[i].Start)
				}
			}
		})
	}
}
