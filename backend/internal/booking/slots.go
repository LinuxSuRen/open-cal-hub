package booking

import (
	"context"
	"sort"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// interval 是绝对时间区间 [start, end)。
type interval struct {
	start, end time.Time
}

// intervalsForLocalDate 计算某本地日期（可用时区）的可用区间：
// 每周规则为底，available 日期追加、unavailable 日期扣除，结果有序且不相交。
func intervalsForLocalDate(set *AvailabilitySet, localDate time.Time) []interval {
	loc := tzOrUTC(set.Timezone)
	dayStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	weekday := int(dayStart.Weekday()) // 0=周日
	var avail []interval
	for _, r := range set.Rules {
		if r.Weekday != weekday {
			continue
		}
		avail = append(avail, interval{
			start: dayStart.Add(time.Duration(r.StartMinute) * time.Minute),
			end:   dayStart.Add(time.Duration(r.EndMinute) * time.Minute),
		})
	}
	dateStr := dayStart.Format("2006-01-02")
	var cuts, extra []interval
	for _, d := range set.Dates {
		if d.Date != dateStr {
			continue
		}
		iv := interval{
			start: dayStart.Add(time.Duration(d.StartMinute) * time.Minute),
			end:   dayStart.Add(time.Duration(d.EndMinute) * time.Minute),
		}
		if d.Kind == "unavailable" {
			cuts = append(cuts, iv)
		} else {
			extra = append(extra, iv)
		}
	}
	avail = append(avail, extra...)
	avail = mergeIntervals(avail)
	avail = subtractIntervals(avail, mergeIntervals(cuts))
	return avail
}

func tzOrUTC(name string) *time.Location {
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return time.UTC
}

func mergeIntervals(in []interval) []interval {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool {
		if !in[i].start.Equal(in[j].start) {
			return in[i].start.Before(in[j].start)
		}
		return in[i].end.Before(in[j].end)
	})
	out := []interval{in[0]}
	for _, cur := range in[1:] {
		last := &out[len(out)-1]
		if !cur.start.After(last.end) {
			if cur.end.After(last.end) {
				last.end = cur.end
			}
			continue
		}
		out = append(out, cur)
	}
	return out
}

// subtractIntervals 从 base 中扣除 cuts（均为有序不相交集合）。
func subtractIntervals(base, cuts []interval) []interval {
	for _, cut := range cuts {
		var next []interval
		for _, b := range base {
			switch {
			case !cut.start.Before(b.end) || !b.start.Before(cut.end): // 无交集
				next = append(next, b)
			case cut.start.After(b.start) && cut.end.Before(b.end): // 中间挖空
				next = append(next, interval{b.start, cut.start}, interval{cut.end, b.end})
			case cut.start.After(b.start): // 前半保留
				next = append(next, interval{b.start, cut.start})
			case cut.end.Before(b.end): // 后半保留
				next = append(next, interval{cut.end, b.end})
			} // 完全覆盖则丢弃
		}
		base = next
	}
	return base
}

// busySpan 忙碌时段来源标记。
type busySpan struct {
	interval
	eventRowID int64 // 关联事件行（排除自身预约事件用）
}

// collectBusy 汇总主人全部日历事件 + 外部源忙碌 + 已确认预约。
func collectBusy(ctx context.Context, q cal.DBTX, hostID int64, from, to time.Time,
	excludeEventID int64, excludeBookingID int64) ([]busySpan, error) {
	margin := 48 * time.Hour
	var busy []busySpan

	calIDs, err := cal.CalendarIDsByOwner(ctx, q, hostID)
	if err != nil {
		return nil, err
	}
	if len(calIDs) > 0 {
		rows, err := cal.LoadEventRowsInRange(ctx, q, calIDs, from.Add(-margin), to.Add(margin))
		if err != nil {
			return nil, err
		}
		for _, inst := range cal.ExpandRange(rows, from.Add(-margin), to.Add(margin), false) {
			start, err1 := time.Parse(time.RFC3339, inst.Start)
			end, err2 := time.Parse(time.RFC3339, inst.End)
			if err1 != nil || err2 != nil {
				continue
			}
			if excludeEventID != 0 && inst.ID == excludeEventID {
				continue
			}
			busy = append(busy, busySpan{interval{start, end}, inst.ID})
		}
	}

	feeds, err := q.QueryContext(ctx, `SELECT f.dtstart, f.dtend FROM feed_events f
		JOIN external_feeds e ON e.id = f.feed_id
		WHERE e.user_id = ? AND f.busy = 1 AND f.dtstart < ? AND f.dtend > ?`,
		hostID, cal.UTCString(to.Add(margin)), cal.UTCString(from.Add(-margin)))
	if err != nil {
		return nil, err
	}
	defer feeds.Close()
	for feeds.Next() {
		var s, e string
		if err := feeds.Scan(&s, &e); err != nil {
			return nil, err
		}
		start, err1 := time.Parse(time.RFC3339, s)
		end, err2 := time.Parse(time.RFC3339, e)
		if err1 != nil || err2 != nil {
			continue
		}
		busy = append(busy, busySpan{interval{start, end}, 0})
	}

	bookings, err := q.QueryContext(ctx, `SELECT id, start_utc, end_utc FROM bookings
		WHERE host_id = ? AND status = 'confirmed' AND id != ? AND start_utc < ? AND end_utc > ?`,
		hostID, excludeBookingID, cal.UTCString(to.Add(margin)), cal.UTCString(from.Add(-margin)))
	if err != nil {
		return nil, err
	}
	defer bookings.Close()
	for bookings.Next() {
		var id int64
		var s, e string
		if err := bookings.Scan(&id, &s, &e); err != nil {
			return nil, err
		}
		start, err1 := time.Parse(time.RFC3339, s)
		end, err2 := time.Parse(time.RFC3339, e)
		if err1 != nil || err2 != nil {
			continue
		}
		busy = append(busy, busySpan{interval{start, end}, 0})
	}
	return busy, nil
}

// conflictsWithBusy 判断 [start,end) 加缓冲后是否与忙碌时段冲突。
func conflictsWithBusy(busy []busySpan, start, end time.Time, bufferBefore, bufferAfter time.Duration) bool {
	for _, b := range busy {
		if start.Before(b.end.Add(bufferAfter)) && end.After(b.start.Add(-bufferBefore)) {
			return true
		}
	}
	return false
}

// rangeBounds 计算可预约范围（rolling/fixed），返回 [min,max) 可预约开始时刻。
func rangeBounds(et *EventType, set *AvailabilitySet, now time.Time) (time.Time, time.Time, bool) {
	loc := tzOrUTC(set.Timezone)
	if et.RangeType == "fixed" {
		if et.StartDate == nil || et.EndDate == nil {
			return time.Time{}, time.Time{}, false
		}
		startDay, err1 := time.ParseInLocation("2006-01-02", *et.StartDate, loc)
		endDay, err2 := time.ParseInLocation("2006-01-02", *et.EndDate, loc)
		if err1 != nil || err2 != nil {
			return time.Time{}, time.Time{}, false
		}
		return startDay, endDay.AddDate(0, 0, 1), true
	}
	days := et.RangeDays
	if days <= 0 {
		days = 60
	}
	return now, now.Add(time.Duration(days) * 24 * time.Hour), true
}

// availabilityIntervalContaining 返回完整包含 [start,end) 的可用区间（同一本地日期）。
func availabilityIntervalContaining(set *AvailabilitySet, start, end time.Time) *interval {
	loc := tzOrUTC(set.Timezone)
	for _, iv := range intervalsForLocalDate(set, start.In(loc)) {
		if !iv.start.After(start) && !end.After(iv.end) {
			return &iv
		}
	}
	// 跨日不匹配时再检查结束侧日期（跨午夜区间按日拆分，无法完整包含则无效）
	return nil
}

// ValidateSlot 依契约错误码顺序校验一个预约时刻。
// 顺序：outside_range → insufficient_notice → invalid_slot → slot_unavailable。
func ValidateSlot(ctx context.Context, q cal.DBTX, host *Host, et *EventType, set *AvailabilitySet,
	start time.Time, now time.Time, excludeEventID, excludeBookingID int64) error {
	end := start.Add(time.Duration(et.DurationMinutes) * time.Minute)
	minStart, maxStart, ok := rangeBounds(et, set, now)
	if !ok || start.Before(minStart) || !start.Before(maxStart) {
		return httpx.E("booking/outside_range", "超出可预约日期范围")
	}
	if start.Sub(now) < time.Duration(et.MinNoticeMinutes)*time.Minute {
		return httpx.E("booking/insufficient_notice", "距开始时间太近，无法预约")
	}
	if end.Sub(start) != time.Duration(et.DurationMinutes)*time.Minute || end.Before(start) {
		return httpx.E("booking/invalid_slot", "时间段无效")
	}
	if availabilityIntervalContaining(set, start, end) == nil {
		return httpx.E("booking/invalid_slot", "时间段无效")
	}
	busy, err := collectBusy(ctx, q, host.ID, start.Add(-24*time.Hour), end.Add(24*time.Hour), excludeEventID, excludeBookingID)
	if err != nil {
		return err
	}
	if conflictsWithBusy(busy, start, end,
		time.Duration(et.BufferBefore)*time.Minute, time.Duration(et.BufferAfter)*time.Minute) {
		return httpx.E("booking/slot_unavailable", "该时间段已被占用，请选择其他时间")
	}
	return nil
}

// DaySlots 是一天的分桶槽位输出。
type DaySlots struct {
	Date  string       `json:"date"`
	Slots []SlotOutput `json:"slots"`
}

// SlotOutput 是单个槽位。
type SlotOutput struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	Available bool   `json:"available"`
}

// ComputeSlots 计算从 startDay（grid 时区本地日期）起 days 天的可用槽位。
func ComputeSlots(ctx context.Context, q cal.DBTX, host *Host, et *EventType, set *AvailabilitySet,
	gridTZ string, startDay time.Time, days int, now time.Time) ([]DaySlots, error) {
	grid := tzOrUTC(gridTZ)
	dur := time.Duration(et.DurationMinutes) * time.Minute
	step := dur
	if et.SlotInterval > 0 {
		step = time.Duration(et.SlotInterval) * time.Minute
	}
	rangeStart := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, grid)
	rangeEnd := rangeStart.Add(time.Duration(days) * 24 * time.Hour)
	minStart, maxStart, ok := rangeBounds(et, set, now)
	if !ok {
		return []DaySlots{}, nil
	}
	notice := time.Duration(et.MinNoticeMinutes) * time.Minute

	busy, err := collectBusy(ctx, q, host.ID, rangeStart, rangeEnd, 0, 0)
	if err != nil {
		return nil, err
	}
	bufBefore := time.Duration(et.BufferBefore) * time.Minute
	bufAfter := time.Duration(et.BufferAfter) * time.Minute

	// 按可用时区逐日生成候选（覆盖 grid 时区跨日范围）
	setLoc := tzOrUTC(set.Timezone)
	day := rangeStart.In(setLoc)
	endDay := rangeEnd.In(setLoc)
	type bucketKey struct{ y, m, d int }
	buckets := map[bucketKey]*DaySlots{}
	var keys []bucketKey
	for d := 0; d < days; d++ {
		gd := rangeStart.Add(time.Duration(d) * 24 * time.Hour)
		k := bucketKey{gd.Year(), int(gd.Month()), gd.Day()}
		buckets[k] = &DaySlots{Date: gd.Format("2006-01-02"), Slots: []SlotOutput{}}
		keys = append(keys, k)
	}
	for !day.After(endDay) {
		for _, iv := range intervalsForLocalDate(set, day) {
			for s := iv.start; !s.Add(dur).After(iv.end); s = s.Add(step) {
				if s.Before(rangeStart) || !s.Before(rangeEnd) {
					continue
				}
				if s.Before(now.Add(notice)) || s.Before(minStart) || !s.Before(maxStart) {
					continue
				}
				if conflictsWithBusy(busy, s, s.Add(dur), bufBefore, bufAfter) {
					continue
				}
				gd := s.In(grid)
				k := bucketKey{gd.Year(), int(gd.Month()), gd.Day()}
				if b, ok := buckets[k]; ok {
					b.Slots = append(b.Slots, SlotOutput{
						Start: cal.UTCString(s), End: cal.UTCString(s.Add(dur)), Available: true,
					})
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	out := make([]DaySlots, 0, len(keys))
	for _, k := range keys {
		out = append(out, *buckets[k])
	}
	return out, nil
}
