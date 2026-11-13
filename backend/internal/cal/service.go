package cal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// CreateEvent 校验并创建事件（REST POST）。
func CreateEvent(ctx context.Context, q DBTX, calendarID int64, req *CreateEventRequest) (*EventRow, error) {
	cal, err := GetCalendar(ctx, q, calendarID)
	if err != nil {
		return nil, err
	}
	in, err := req.normalize(cal.Timezone)
	if err != nil {
		return nil, err
	}
	return CreateEventFromInput(ctx, q, calendarID, "", in)
}

// CreateEventRequest 是 POST /calendars/{id}/events 的请求体。
type CreateEventRequest struct {
	Summary     string          `json:"summary"`
	Start       string          `json:"start"`
	End         string          `json:"end"`
	AllDay      bool            `json:"all_day"`
	TZ          string          `json:"tz"`
	Location    string          `json:"location"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Reminders   []int           `json:"reminders"`
	RRule       json.RawMessage `json:"rrule"`
}

// normalize 校验并归一化为 EventInput。
func (r *CreateEventRequest) normalize(calTZ string) (EventInput, error) {
	in := EventInput{
		Summary: strings.TrimSpace(r.Summary), Location: r.Location, Description: r.Description,
		AllDay: r.AllDay, TZ: r.TZ, Status: r.Status, Reminders: r.Reminders,
	}
	if in.Summary == "" {
		return in, httpx.Validation(map[string]string{"summary": "标题不能为空"})
	}
	if in.TZ == "" {
		in.TZ = calTZ
	}
	var err error
	if in.AllDay {
		in.Start, err = parseDateUTC(r.Start)
		if err != nil {
			return in, httpx.Validation(map[string]string{"start": "全天事件的开始时间必须是日期（YYYY-MM-DD）"})
		}
		in.End, err = parseDateUTC(r.End)
		if err != nil {
			in.End = in.Start.AddDate(0, 0, 1)
		}
		if !in.End.After(in.Start) {
			in.End = in.Start.AddDate(0, 0, 1)
		}
	} else {
		if in.Start, err = parseTime(r.Start); err != nil {
			return in, httpx.Validation(map[string]string{"start": "开始时间格式无效"})
		}
		if in.End, err = parseTime(r.End); err != nil {
			return in, httpx.Validation(map[string]string{"end": "结束时间格式无效"})
		}
		if !in.End.After(in.Start) {
			return in, httpx.Validation(map[string]string{"end": "结束时间必须晚于开始时间"})
		}
	}
	if _, err := time.LoadLocation(in.TZ); err != nil {
		return in, httpx.Validation(map[string]string{"tz": "时区无效"})
	}
	if in.AllDay {
		in.TZ = "UTC"
	}
	if in.Status == "" {
		in.Status = StatusConfirmed
	}
	if !ValidStatus(in.Status) {
		return in, httpx.Validation(map[string]string{"status": "状态无效"})
	}
	if err := validateReminders(in.Reminders); err != nil {
		return in, err
	}
	text, _, err := ParseRRuleInput(r.RRule)
	if err != nil {
		return in, httpx.E("cal/invalid_rrule", "重复规则无效").WithCause(err)
	}
	in.RRule = text
	return in, nil
}

func validateReminders(ms []int) error {
	if len(ms) > MaxReminders {
		return httpx.Validation(map[string]string{"reminders": "提醒数量过多"})
	}
	for _, m := range ms {
		if m < 0 || m > MaxReminderMinutes {
			return httpx.Validation(map[string]string{"reminders": "提醒提前分钟数无效"})
		}
	}
	return nil
}

func parseDateUTC(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// EventPatch 是 PATCH /events/{id} 的请求体（指针字段区分未提供）。
type EventPatch struct {
	Summary     *string         `json:"summary"`
	Location    *string         `json:"location"`
	Description *string         `json:"description"`
	Start       *string         `json:"start"`
	End         *string         `json:"end"`
	AllDay      *bool           `json:"all_day"`
	TZ          *string         `json:"tz"`
	Status      *string         `json:"status"`
	Reminders   *[]int          `json:"reminders"`
	RRule       json.RawMessage `json:"rrule"`
	RecurrenceID string         `json:"recurrence_id"`
}

// PatchEvent 支持 scope=all|single（single 生成/更新 recurrence_id 覆盖行）。
func PatchEvent(ctx context.Context, q DBTX, eventID int64, scope string, patch *EventPatch) (*EventRow, error) {
	row, err := GetEvent(ctx, q, eventID)
	if err != nil {
		return nil, err
	}
	master := row
	if row.RecurrenceID != nil {
		master, err = masterRowOf(ctx, q, row)
		if err != nil {
			return nil, err
		}
	}
	if scope == "single" && master.RRule != "" && row.RecurrenceID == nil {
		return patchSingle(ctx, q, master, patch)
	}
	// scope=all：直接改主事件行；目标行本身是覆盖时改其所属系列的主事件
	return patchMaster(ctx, q, master, patch)
}

func masterRowOf(ctx context.Context, q DBTX, row *EventRow) (*EventRow, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM events WHERE calendar_id=? AND uid=? AND recurrence_id IS NULL`,
		row.CalendarID, row.UID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	if err != nil {
		return nil, err
	}
	return GetEvent(ctx, q, id)
}

// applyToInput 把当前行与补丁合并为规范输入。
func applyToInput(row *EventRow, patch *EventPatch) (EventInput, error) {
	in := EventInput{
		Summary: row.Summary, Location: row.Location, Description: describeRow(row),
		Start: row.Start, End: row.End, AllDay: row.AllDay, TZ: row.TZ,
		Status: row.Status, Reminders: row.Reminders, RRule: row.RRule,
	}
	if patch.Summary != nil {
		in.Summary = strings.TrimSpace(*patch.Summary)
		if in.Summary == "" {
			return in, httpx.Validation(map[string]string{"summary": "标题不能为空"})
		}
	}
	if patch.Location != nil {
		in.Location = *patch.Location
	}
	if patch.Description != nil {
		in.Description = *patch.Description
	}
	if patch.Status != nil && *patch.Status != "" {
		if !ValidStatus(*patch.Status) {
			return in, httpx.Validation(map[string]string{"status": "状态无效"})
		}
		in.Status = *patch.Status
	}
	if patch.Reminders != nil {
		in.Reminders = *patch.Reminders
	}
	var err error
	if patch.TZ != nil && *patch.TZ != "" {
		if _, lerr := time.LoadLocation(*patch.TZ); lerr != nil {
			return in, httpx.Validation(map[string]string{"tz": "时区无效"})
		}
		in.TZ = *patch.TZ
	}
	if patch.AllDay != nil {
		in.AllDay = *patch.AllDay
	}
	parseStart := func(s string) (time.Time, error) {
		if in.AllDay {
			if t, err := parseDateUTC(s); err == nil {
				return t, nil
			}
			return time.Time{}, errors.New("invalid date")
		}
		return parseTime(s)
	}
	if patch.Start != nil {
		if in.Start, err = parseStart(*patch.Start); err != nil {
			return in, httpx.Validation(map[string]string{"start": "开始时间格式无效"})
		}
	}
	if patch.End != nil {
		if in.End, err = parseStart(*patch.End); err != nil {
			return in, httpx.Validation(map[string]string{"end": "结束时间格式无效"})
		}
	}
	if patch.Start != nil || patch.End != nil {
		if !in.AllDay && !in.End.After(in.Start) {
			return in, httpx.Validation(map[string]string{"end": "结束时间必须晚于开始时间"})
		}
		if in.AllDay && !in.End.After(in.Start) {
			in.End = in.Start.AddDate(0, 0, 1)
		}
	}
	if in.AllDay {
		in.TZ = "UTC"
	}
	if err := validateReminders(in.Reminders); err != nil {
		return in, err
	}
	text, clear, err := ParseRRuleInput(patch.RRule)
	if err != nil {
		return in, httpx.E("cal/invalid_rrule", "重复规则无效").WithCause(err)
	}
	switch {
	case clear:
		in.RRule = ""
	case text != "":
		in.RRule = text
	}
	return in, nil
}

// patchMaster 更新主事件行（scope=all）。
func patchMaster(ctx context.Context, q DBTX, master *EventRow, patch *EventPatch) (*EventRow, error) {
	in, err := applyToInput(master, patch)
	if err != nil {
		return nil, err
	}
	cal, err := BuildVCalendar(master.UID, in, master.Sequence, nowUTC())
	if err != nil {
		return nil, err
	}
	data, err := EncodeCalendar(cal)
	if err != nil {
		return nil, err
	}
	master.Data = data
	master.Summary, master.Location = in.Summary, in.Location
	master.Start, master.End = in.Start, in.End
	master.TZ, master.AllDay = in.TZ, in.AllDay
	master.RRule, master.Status = in.RRule, in.Status
	master.Reminders = sortedUnique(in.Reminders)
	if err := updateEventData(ctx, q, master); err != nil {
		return nil, err
	}
	if in.RRule == "" {
		// 系列取消重复后，遗留覆盖不再有意义
		if _, err := q.ExecContext(ctx,
			`DELETE FROM events WHERE calendar_id=? AND uid=? AND recurrence_id IS NOT NULL`,
			master.CalendarID, master.UID); err != nil {
			return nil, err
		}
	}
	return master, nil
}

// patchSingle 生成或更新某实例的覆盖行（scope=single）。
func patchSingle(ctx context.Context, q DBTX, master *EventRow, patch *EventPatch) (*EventRow, error) {
	rid, err := parseTime(patch.RecurrenceID)
	if err != nil {
		return nil, httpx.Validation(map[string]string{"recurrence_id": "缺少实例时间（recurrence_id）"})
	}
	// 实例必须属于该系列
	occs := expandOccurrences(master, rid.Add(-time.Second), rid.Add(time.Second), 2)
	found := false
	for _, o := range occs {
		if o.Unix() == rid.Unix() {
			found = true
			break
		}
	}
	if !found {
		return nil, httpx.E("cal/event_not_found", "事件实例不存在")
	}
	var existing *EventRow
	rows, err := EventsByUID(ctx, q, master.CalendarID, master.UID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.RecurrenceID != nil && r.RecurrenceID.Unix() == rid.Unix() {
			existing = r
			break
		}
	}
	base := existing
	if base == nil {
		// 新建覆盖行：时间基准锚定在被覆盖的实例（保持系列时长），
		// 而不是主事件的首次出现——否则仅改标题也会把实例挪回主事件时间。
		shifted := *master
		shifted.Start = rid
		shifted.End = rid.Add(master.End.Sub(master.Start))
		base = &shifted
	}
	in, err := applyToInput(base, patch)
	if err != nil {
		return nil, err
	}
	in.RRule = "" // 覆盖行不重复
	cal, err := BuildVCalendar(master.UID, in, base.Sequence, nowUTC())
	if err != nil {
		return nil, err
	}
	// 覆盖行需要 RECURRENCE-ID 属性
	if err := addRecurrenceIDProp(cal, rid, in.TZ); err != nil {
		return nil, err
	}
	data, err := EncodeCalendar(cal)
	if err != nil {
		return nil, err
	}
	out := &EventRow{
		CalendarID: master.CalendarID, UID: master.UID,
		RecurrenceID: &rid, Data: data,
		Summary: in.Summary, Location: in.Location,
		Start: in.Start, End: in.End, TZ: in.TZ, AllDay: in.AllDay,
		RRule: "", Reminders: sortedUnique(in.Reminders), Status: in.Status,
		Sequence: base.Sequence,
	}
	if existing != nil {
		out.ID = existing.ID
		if err := updateEventData(ctx, q, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	if err := insertEvent(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

func addRecurrenceIDProp(cal *ics.Calendar, rid time.Time, tz string) error {
	for _, c := range cal.Children {
		if c.Name != ics.CompEvent {
			continue
		}
		prop := ics.NewProp(ics.PropRecurrenceID)
		prop.SetDateTime(rid.In(tzLocationOrUTC(tz)))
		c.Props.Set(prop)
	}
	return nil
}

func tzLocationOrUTC(tz string) *time.Location {
	l, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return l
}

// DeleteEvent 支持 scope=all|single；single 在重复系列上生成取消覆盖。
func DeleteEvent(ctx context.Context, q DBTX, eventID int64, scope, recurrenceID string) error {
	row, err := GetEvent(ctx, q, eventID)
	if err != nil {
		return err
	}
	if scope == "single" && row.RecurrenceID == nil {
		master := row
		if master.RRule == "" {
			// 非重复事件：直接删除
			_, err := q.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, eventID)
			return err
		}
		rid, perr := parseTime(recurrenceID)
		if perr != nil {
			return httpx.Validation(map[string]string{"recurrence_id": "缺少实例时间（recurrence_id）"})
		}
		cancelled := *master
		cancelled.ID = 0
		cancelled.RecurrenceID = &rid
		cancelled.RRule = ""
		cancelled.Status = StatusCancelled
		cancelled.Start = rid
		cancelled.End = rid.Add(master.End.Sub(master.Start))
		in := EventInput{
			Summary: master.Summary, Location: master.Location, Description: describeRow(master),
			Start: cancelled.Start, End: cancelled.End, AllDay: master.AllDay, TZ: master.TZ,
			Status: StatusCancelled, Reminders: master.Reminders,
		}
		cal, err := BuildVCalendar(master.UID, in, master.Sequence, nowUTC())
		if err != nil {
			return err
		}
		if err := addRecurrenceIDProp(cal, rid, master.TZ); err != nil {
			return err
		}
		data, err := EncodeCalendar(cal)
		if err != nil {
			return err
		}
		cancelled.Data = data
		// 已有覆盖行则更新为取消，否则插入
		existing, err := EventsByUID(ctx, q, master.CalendarID, master.UID)
		if err != nil {
			return err
		}
		for _, r := range existing {
			if r.RecurrenceID != nil && r.RecurrenceID.Unix() == rid.Unix() {
				r.Data = data
				r.Status = StatusCancelled
				r.Summary, r.Start, r.End = cancelled.Summary, cancelled.Start, cancelled.End
				return updateEventData(ctx, q, r)
			}
		}
		return insertEvent(ctx, q, &cancelled)
	}
	if scope == "single" && row.RecurrenceID != nil {
		// 删除覆盖行（实例恢复为系列原状）
		_, err := q.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, eventID)
		return err
	}
	// scope=all：删除整个系列（含覆盖）
	return DeleteEventsByUID(ctx, q, row.CalendarID, row.UID)
}

// ImportICS 解析 ICS 文本并按 (calendar_id, uid, recurrence_id) 去重导入。
func ImportICS(ctx context.Context, q DBTX, calendarID int64, icsText string) (imported, skipped int, err error) {
	cal, err := ics.NewDecoder(strings.NewReader(icsText)).Decode()
	if err != nil {
		return 0, 0, httpx.E("cal/import_failed", "导入失败：文件不是有效的日历数据").WithCause(err)
	}
	events := cal.Events()
	if len(events) == 0 {
		return 0, 0, httpx.E("cal/import_failed", "导入失败：文件不是有效的日历数据")
	}
	for _, ev := range events {
		d, derr := DeriveEvent(ev.Component, "UTC")
		if derr != nil || d.UID == "" {
			skipped++
			continue
		}
		if d.Start.IsZero() {
			skipped++
			continue
		}
		if d.End.Before(d.Start) || d.End.Equal(d.Start) {
			d.End = d.Start.Add(time.Hour)
		}
		var rec sql.NullString
		if d.RecurrenceID != nil {
			rec = sql.NullString{String: fmtTime(*d.RecurrenceID), Valid: true}
		}
		var exists int
		if qerr := q.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM events WHERE calendar_id=? AND uid=? AND (recurrence_id IS ? OR recurrence_id = ?)`,
			calendarID, d.UID, rec, rec.String).Scan(&exists); qerr != nil {
			return imported, skipped, qerr
		}
		if exists > 0 {
			skipped++
			continue
		}
		in := EventInput{
			Summary: d.Summary, Location: d.Location, Description: d.Description,
			Start: d.Start, End: d.End, AllDay: d.AllDay, TZ: d.TZ,
			Status: d.Status, Reminders: d.Reminders, RRule: d.RRule,
		}
		vcal, berr := BuildVCalendar(d.UID, in, 0, nowUTC())
		if berr != nil {
			skipped++
			continue
		}
		if d.RecurrenceID != nil {
			if aerr := addRecurrenceIDProp(vcal, *d.RecurrenceID, d.TZ); aerr != nil {
				skipped++
				continue
			}
		}
		data, eerr := EncodeCalendar(vcal)
		if eerr != nil {
			skipped++
			continue
		}
		row := &EventRow{
			CalendarID: calendarID, UID: d.UID, Data: data,
			Summary: d.Summary, Location: d.Location,
			Start: d.Start, End: d.End, TZ: d.TZ, AllDay: d.AllDay,
			RRule: d.RRule, Reminders: d.Reminders, Status: d.Status,
			RecurrenceID: d.RecurrenceID,
		}
		if ierr := insertEvent(ctx, q, row); ierr != nil {
			return imported, skipped, ierr
		}
		imported++
	}
	return imported, skipped, nil
}
