package cal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// EventRow 是 events 表的一行（主事件或实例覆盖）。
type EventRow struct {
	ID           int64
	CalendarID   int64
	UID          string
	RecurrenceID *time.Time // 实例覆盖的 RECURRENCE-ID（UTC）；主事件为 nil
	Data         string     // 完整 VCALENDAR 原文
	Summary      string
	Location     string
	Start        time.Time
	End          time.Time
	TZ           string
	AllDay       bool
	RRule        string
	Reminders    []int
	Status       string
	Sequence     int
	ETag         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Instance 是展开后的 API 事件实例（契约 §3）。
type Instance struct {
	ID           int64   `json:"id"`
	UID          string  `json:"uid"`
	CalendarID   int64   `json:"calendar_id"`
	Summary      string  `json:"summary"`
	Location     string  `json:"location"`
	Description  string  `json:"description"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	AllDay       bool    `json:"all_day"`
	TZ           string  `json:"tz"`
	Status       string  `json:"status"`
	RRule        string  `json:"rrule"`
	Recurring    bool    `json:"recurring"`
	RecurrenceID *string `json:"recurrence_id"`
	Reminders    []int   `json:"reminders"`
	MasterID     int64   `json:"master_id"`
}

const eventCols = `id, calendar_id, uid, recurrence_id, data, summary, location, dtstart, dtend,
 tzid, all_day, rrule, reminders, status, sequence, etag, created_at, updated_at`

// EventColumns 暴露查询列（dav 包复用）。
func EventColumns() string { return eventCols }

// ScanEventRows 从 *sql.Rows 批量扫描事件行（dav 包复用）。
func ScanEventRows(rows *sql.Rows) ([]*EventRow, error) {
	var out []*EventRow
	for rows.Next() {
		r, err := scanEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanEvent(scan func(dest ...any) error) (*EventRow, error) {
	var (
		r            EventRow
		recurrenceID sql.NullString
		reminders    string
		start, end   string
		created, upd string
	)
	if err := scan(&r.ID, &r.CalendarID, &r.UID, &recurrenceID, &r.Data, &r.Summary, &r.Location,
		&start, &end, &r.TZ, &r.AllDay, &r.RRule, &reminders, &r.Status, &r.Sequence, &r.ETag,
		&created, &upd); err != nil {
		return nil, err
	}
	var err error
	if r.Start, err = parseTime(start); err != nil {
		return nil, fmt.Errorf("dtstart %q: %w", start, err)
	}
	if r.End, err = parseTime(end); err != nil {
		return nil, fmt.Errorf("dtend %q: %w", end, err)
	}
	if r.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if r.UpdatedAt, err = parseTime(upd); err != nil {
		return nil, err
	}
	if recurrenceID.Valid && recurrenceID.String != "" {
		t, err := parseTime(recurrenceID.String)
		if err != nil {
			return nil, fmt.Errorf("recurrence_id %q: %w", recurrenceID.String, err)
		}
		r.RecurrenceID = &t
	}
	r.Reminders = []int{}
	_ = json.Unmarshal([]byte(reminders), &r.Reminders)
	if r.Reminders == nil {
		r.Reminders = []int{}
	}
	return &r, nil
}

// GetEvent 按 ID 取事件行。
func GetEvent(ctx context.Context, q DBTX, id int64) (*EventRow, error) {
	row := q.QueryRowContext(ctx, `SELECT `+eventCols+` FROM events WHERE id = ?`, id)
	r, err := scanEvent(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("cal/event_not_found", "事件不存在")
	}
	return r, err
}

// EventsByUID 取某日历下同一 UID 的全部行（主事件 + 覆盖）。
func EventsByUID(ctx context.Context, q DBTX, calendarID int64, uid string) ([]*EventRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+eventCols+` FROM events
		WHERE calendar_id = ? AND uid = ? ORDER BY recurrence_id IS NULL DESC, recurrence_id`, calendarID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*EventRow
	for rows.Next() {
		r, err := scanEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// insertEvent 写入一行事件并回填 ID。
func insertEvent(ctx context.Context, q DBTX, r *EventRow) error {
	reminders, _ := json.Marshal(r.Reminders)
	var rec sql.NullString
	if r.RecurrenceID != nil {
		rec = sql.NullString{String: fmtTime(*r.RecurrenceID), Valid: true}
	}
	now := fmtTime(nowUTC())
	etag := calcETag(r.Data, now)
	res, err := q.ExecContext(ctx, `INSERT INTO events
		(calendar_id, uid, recurrence_id, data, summary, location, dtstart, dtend, tzid, all_day,
		 rrule, reminders, status, sequence, etag, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.CalendarID, r.UID, rec, r.Data, r.Summary, r.Location, fmtTime(r.Start), fmtTime(r.End),
		r.TZ, r.AllDay, r.RRule, string(reminders), r.Status, r.Sequence, etag, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	r.ID = id
	r.ETag = etag
	return nil
}

// InsertEventRow 导出写入路径（dav PUT 重建资源用）。
func InsertEventRow(ctx context.Context, q DBTX, r *EventRow) error { return insertEvent(ctx, q, r) }

// updateEventData 按行重写原文与索引列（sequence+1）。
func updateEventData(ctx context.Context, q DBTX, r *EventRow) error {
	reminders, _ := json.Marshal(r.Reminders)
	now := fmtTime(nowUTC())
	r.Sequence++
	etag := calcETag(r.Data, now)
	_, err := q.ExecContext(ctx, `UPDATE events SET data=?, summary=?, location=?, dtstart=?, dtend=?,
		tzid=?, all_day=?, rrule=?, reminders=?, status=?, sequence=?, etag=?, updated_at=? WHERE id=?`,
		r.Data, r.Summary, r.Location, fmtTime(r.Start), fmtTime(r.End), r.TZ, r.AllDay,
		r.RRule, string(reminders), r.Status, r.Sequence, etag, now, r.ID)
	r.ETag = etag
	return err
}

// CreateEventFromInput 以规范输入创建事件（REST POST 与预约写入共用）。
func CreateEventFromInput(ctx context.Context, q DBTX, calendarID int64, uid string, in EventInput) (*EventRow, error) {
	if uid == "" {
		uid = NewUID()
	}
	if in.TZ == "" {
		in.TZ = "UTC"
	}
	if in.Status == "" {
		in.Status = StatusConfirmed
	}
	cal, err := BuildVCalendar(uid, in, 0, nowUTC())
	if err != nil {
		return nil, err
	}
	data, err := EncodeCalendar(cal)
	if err != nil {
		return nil, err
	}
	rrule := in.RRule
	row := &EventRow{
		CalendarID: calendarID, UID: uid, Data: data,
		Summary: in.Summary, Location: in.Location,
		Start: in.Start, End: in.End, TZ: in.TZ, AllDay: in.AllDay,
		RRule: rrule, Reminders: sortedUnique(in.Reminders), Status: in.Status,
	}
	if err := insertEvent(ctx, q, row); err != nil {
		return nil, err
	}
	return row, nil
}

// DeleteEventsByUID 删除某日历下该 UID 的全部行（主事件与覆盖）。
func DeleteEventsByUID(ctx context.Context, q DBTX, calendarID int64, uid string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM events WHERE calendar_id=? AND uid=?`, calendarID, uid)
	return err
}

// LoadEventRowsInRange 取 [from,to) 可能产生实例的行：
// 非重复行按时间重叠过滤；重复主事件放宽为 dtstart < to（历史起点也要展开）。
func LoadEventRowsInRange(ctx context.Context, q DBTX, calendarIDs []int64, from, to time.Time) ([]*EventRow, error) {
	if len(calendarIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(calendarIDs))
	args := make([]any, 0, len(calendarIDs)+2)
	for i, id := range calendarIDs {
		ids[i] = "?"
		args = append(args, id)
	}
	ph := strings.Join(ids, ",")
	args = append(args, fmtTime(to), fmtTime(from), fmtTime(to))
	rows, err := q.QueryContext(ctx, `SELECT `+eventCols+` FROM events
		WHERE calendar_id IN (`+ph+`)
		  AND ((rrule = '' AND dtstart < ? AND dtend > ?) OR (rrule <> '' AND recurrence_id IS NULL AND dtstart < ?))`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*EventRow
	for rows.Next() {
		r, err := scanEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExpandRange 把事件行展开为 [from,to) 内的实例序列：RRULE + RDATE/EXDATE + 实例覆盖。
func ExpandRange(rows []*EventRow, from, to time.Time, includeCancelled bool) []Instance {
	type group struct {
		master    *EventRow
		overrides map[int64]*EventRow // key: recurrence instant unix
	}
	groups := make(map[string]*group) // key: calendarID:uid
	for _, r := range rows {
		key := fmt.Sprintf("%d:%s", r.CalendarID, r.UID)
		g := groups[key]
		if g == nil {
			g = &group{overrides: map[int64]*EventRow{}}
			groups[key] = g
		}
		if r.RecurrenceID == nil {
			// 主事件行；异常数据下取 ID 最小者为准
			if g.master == nil || r.ID < g.master.ID {
				g.master = r
			}
			continue
		}
		g.overrides[r.RecurrenceID.Unix()] = r
	}
	var out []Instance
	for _, g := range groups {
		if g.master == nil {
			// 只有覆盖没有主事件（数据异常）：按独立事件输出
			for _, r := range g.overrides {
				out = append(out, rowInstance(r, r, false))
			}
			continue
		}
		m := g.master
		duration := m.End.Sub(m.Start)
		var occs []time.Time
		if m.RRule == "" {
			occs = []time.Time{m.Start}
		} else {
			occs = expandOccurrences(m, from.Add(-duration-time.Hour), to, MaxExpandInstances)
		}
		for _, occ := range occs {
			if !occ.Before(to) || !occ.Add(duration).After(from) {
				continue
			}
			if ov := g.overrides[occ.Unix()]; ov != nil {
				out = append(out, rowInstance(m, ov, true))
				continue
			}
			out = append(out, synthInstance(m, occ, duration))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].ID < out[j].ID
	})
	if !includeCancelled {
		filtered := out[:0]
		for _, inst := range out {
			if inst.Status != StatusCancelled {
				filtered = append(filtered, inst)
			}
		}
		out = filtered
	}
	return out
}

// expandOccurrences 解析主事件原文并展开 RRULE/RDATE/EXDATE（TZID 语义生效）。
func expandOccurrences(m *EventRow, from, to time.Time, limit int) []time.Time {
	cal, err := ics.NewDecoder(strings.NewReader(m.Data)).Decode()
	if err != nil {
		return []time.Time{m.Start}
	}
	var comp *ics.Component
	for _, c := range cal.Children {
		if c.Name == ics.CompEvent {
			comp = c
			break
		}
	}
	if comp == nil {
		return []time.Time{m.Start}
	}
	loc, err := tzLocation(m.TZ, "UTC")
	if err != nil {
		loc = time.UTC
	}
	rset, err := comp.RecurrenceSet(loc)
	if err != nil {
		// 兼容历史 VALUE=TEXT 写法（分号被转义导致 RECUR 解析失败）：归一化后重试
		if p := comp.Props.Get(ics.PropRecurrenceRule); p != nil && strings.Contains(p.Value, `\;`) {
			comp.Props.Set(&ics.Prop{Name: ics.PropRecurrenceRule, Value: strings.ReplaceAll(p.Value, `\;`, ";")})
			rset, err = comp.RecurrenceSet(loc)
		}
	}
	if err != nil || rset == nil {
		return []time.Time{m.Start}
	}
	occ := rset.Between(from, to, true)
	if len(occ) > limit {
		occ = occ[:limit]
	}
	return occ
}

// rowInstance 由行构造实例；master 提供系列字段，row 为实际数据行。
func rowInstance(master, row *EventRow, isOverride bool) Instance {
	desc := describeRow(row)
	inst := baseInstance(row)
	inst.Description = desc
	inst.RRule = master.RRule
	inst.Recurring = master.RRule != ""
	inst.MasterID = master.ID
	if isOverride {
		s := fmtTime(*row.RecurrenceID)
		inst.RecurrenceID = &s
		inst.RRule = ""
		inst.Recurring = false
	}
	return inst
}

func synthInstance(m *EventRow, occ time.Time, duration time.Duration) Instance {
	inst := baseInstance(m)
	inst.Description = describeRow(m)
	inst.Start = fmtTime(occ)
	inst.End = fmtTime(occ.Add(duration))
	inst.Recurring = m.RRule != ""
	s := fmtTime(occ)
	inst.RecurrenceID = &s
	return inst
}

func baseInstance(r *EventRow) Instance {
	return Instance{
		ID: r.ID, UID: r.UID, CalendarID: r.CalendarID,
		Summary: r.Summary, Location: r.Location,
		Start: fmtTime(r.Start), End: fmtTime(r.End),
		AllDay: r.AllDay, TZ: r.TZ, Status: r.Status, RRule: r.RRule,
		Reminders: append([]int{}, r.Reminders...),
		MasterID:  r.ID,
	}
}

// describeRow 从原文提取 DESCRIPTION（索引列没有该字段）。
func describeRow(r *EventRow) string {
	cal, err := ics.NewDecoder(strings.NewReader(r.Data)).Decode()
	if err != nil {
		return ""
	}
	for _, c := range cal.Children {
		if c.Name != ics.CompEvent {
			continue
		}
		if v, err := c.Props.Text(ics.PropDescription); err == nil {
			return v
		}
	}
	return ""
}

// MutateEvent 载入行、解码原文、修改 VEVENT 后整体回写（索引列同步重推导）。
// 由预约取消/改期等场景使用。
func MutateEvent(ctx context.Context, q DBTX, id int64, fn func(ev *ics.Component) error) (*EventRow, error) {
	row, err := GetEvent(ctx, q, id)
	if err != nil {
		return nil, err
	}
	cal, err := ics.NewDecoder(strings.NewReader(row.Data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode event data: %w", err)
	}
	var comp *ics.Component
	for _, c := range cal.Children {
		if c.Name == ics.CompEvent {
			comp = c
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf("event data has no VEVENT")
	}
	if err := fn(comp); err != nil {
		return nil, err
	}
	data, err := EncodeCalendar(cal)
	if err != nil {
		return nil, err
	}
	row.Data = data
	d, err := DeriveEvent(comp, row.TZ)
	if err != nil {
		return nil, err
	}
	applyDerived(row, d)
	if err := updateEventData(ctx, q, row); err != nil {
		return nil, err
	}
	return row, nil
}

// applyDerived 用解析结果刷新索引字段（保留行身份字段）。
func applyDerived(row *EventRow, d *Derived) {
	row.Summary = d.Summary
	row.Location = d.Location
	row.Start = d.Start
	row.End = d.End
	row.TZ = d.TZ
	row.AllDay = d.AllDay
	row.RRule = d.RRule
	row.Status = d.Status
	row.Reminders = d.Reminders
}
