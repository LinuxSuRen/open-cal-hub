// 预约成交链路：下单事务（重校验槽位 + 唯一索引兜底 + 写 VEVENT）、
// 取消/改期（同步事件状态与时间、双方邮件）、主人视角列表。
package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// ics 组件操作别名（取消/改期复用 cal.MutateEvent）。
type icsComponent = ics.Component

const statusProp = ics.PropStatus

// setDTStartEnd 重写 VEVENT 的 DTSTART/DTEND（按事件时区渲染）。
func setDTStartEnd(ev *ics.Component, start, end time.Time, tz string) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	ev.Props.SetDateTime(ics.PropDateTimeStart, start.In(loc))
	ev.Props.SetDateTime(ics.PropDateTimeEnd, end.In(loc))
}

// Booking 是 bookings 表的一行。
type Booking struct {
	ID           int64
	EventTypeID  int64
	HostID       int64
	CalendarID   int64
	EventID      *int64
	GuestName    string
	GuestEmail   string
	GuestNote    string
	GuestTZ      string
	Start        time.Time
	End          time.Time
	Status       string
	CancelledAt  *time.Time
	CancelledBy  string
	CancelReason string
	ManageToken  string // 哈希
	ICSUID       string
	CreatedAt    time.Time
}

// BookingGuestView 是契约 §4 的客人视角对象。
type BookingGuestView struct {
	ID             int64  `json:"id"`
	EventTypeName  string `json:"event_type_name"`
	HostDisplayName string `json:"host_display_name"`
	Start          string `json:"start"`
	End            string `json:"end"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

const bookingCols = `id, event_type_id, host_id, calendar_id, event_id, guest_name, guest_email,
	guest_note, guest_timezone, start_utc, end_utc, status, cancelled_at, cancelled_by,
	cancel_reason, manage_token_hash, ics_uid, created_at`

func scanBooking(scan func(dest ...any) error) (*Booking, error) {
	var (
		b           Booking
		eventID     sql.NullInt64
		cancelledAt sql.NullString
		cancelledBy sql.NullString
		start, end  string
		created     string
	)
	if err := scan(&b.ID, &b.EventTypeID, &b.HostID, &b.CalendarID, &eventID,
		&b.GuestName, &b.GuestEmail, &b.GuestNote, &b.GuestTZ,
		&start, &end, &b.Status, &cancelledAt, &cancelledBy, &b.CancelReason,
		&b.ManageToken, &b.ICSUID, &created); err != nil {
		return nil, err
	}
	var err error
	if b.Start, err = time.Parse(time.RFC3339, start); err != nil {
		return nil, fmt.Errorf("start_utc %q: %w", start, err)
	}
	if b.End, err = time.Parse(time.RFC3339, end); err != nil {
		return nil, fmt.Errorf("end_utc %q: %w", end, err)
	}
	if b.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return nil, err
	}
	if eventID.Valid {
		v := eventID.Int64
		b.EventID = &v
	}
	if cancelledAt.Valid && cancelledAt.String != "" {
		t, err := time.Parse(time.RFC3339, cancelledAt.String)
		if err == nil {
			b.CancelledAt = &t
		}
	}
	b.CancelledBy = cancelledBy.String
	return &b, nil
}

func bookingByID(ctx context.Context, q cal.DBTX, id int64) (*Booking, error) {
	row := q.QueryRowContext(ctx, `SELECT `+bookingCols+` FROM bookings WHERE id = ?`, id)
	b, err := scanBooking(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("booking/not_found", "预约不存在")
	}
	return b, err
}

func bookingByManageToken(ctx context.Context, q cal.DBTX, token string) (*Booking, error) {
	row := q.QueryRowContext(ctx, `SELECT `+bookingCols+` FROM bookings WHERE manage_token_hash = ?`,
		cal.HashToken(token))
	b, err := scanBooking(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("booking/not_found", "预约不存在")
	}
	return b, err
}

// eventTypeWithSet 载入预约类型与其可用时间集（缺省时空集）。
func eventTypeWithSet(ctx context.Context, q cal.DBTX, et *EventType) (*EventType, *AvailabilitySet, error) {
	if et.AvailabilityID == nil {
		return et, &AvailabilitySet{Timezone: "UTC"}, nil
	}
	set, err := LoadAvailabilityByID(ctx, q, *et.AvailabilityID)
	if err != nil {
		return nil, nil, err
	}
	if set == nil {
		set = &AvailabilitySet{Timezone: "UTC"}
	}
	return et, set, nil
}

// createBooking 下单事务：重校验槽位 → 写 VEVENT → 写 bookings。
// 返回预约行与明文管理令牌（仅此一次可见）。
func createBooking(ctx context.Context, db *sql.DB, host *Host, et *EventType,
	guestName, guestEmail, guestNote, guestTZ string, start time.Time) (*Booking, string, error) {
	_, set, err := eventTypeWithSet(ctx, db, et)
	if err != nil {
		return nil, "", err
	}
	end := start.Add(time.Duration(et.DurationMinutes) * time.Minute)
	if _, err := time.LoadLocation(guestTZ); err != nil {
		guestTZ = "UTC"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	// 重校验：窗口外的变化（并发下单、新忙碌）在此拦截
	if err := ValidateSlot(ctx, tx, host, et, set, start, time.Now().UTC(), 0, 0); err != nil {
		return nil, "", err
	}

	// 事件写入主人日历（CalDAV 客户端可见）
	location := ""
	if et.LocationValue != "" {
		location = et.LocationValue
	}
	icsUID := cal.NewUID()
	ev, err := cal.CreateEventFromInput(ctx, tx, et.CalendarID, icsUID, cal.EventInput{
		Summary:     et.Name + " · " + guestName,
		Location:    location,
		Description: guestNote,
		Start:       start, End: end,
		TZ:     set.Timezone,
		Status: cal.StatusConfirmed,
	})
	if err != nil {
		return nil, "", err
	}

	token := cal.NewToken()
	now := cal.NowUTCString()
	res, err := tx.ExecContext(ctx, `INSERT INTO bookings
		(event_type_id, host_id, calendar_id, event_id, guest_name, guest_email, guest_note, guest_timezone,
		 start_utc, end_utc, status, manage_token_hash, ics_uid, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		et.ID, host.ID, et.CalendarID, ev.ID, guestName, guestEmail, guestNote, guestTZ,
		cal.UTCString(start), cal.UTCString(end), "confirmed", cal.HashToken(token), icsUID, now, now)
	if err != nil {
		if isUniqueErr(err) {
			// idx_bookings_slot 部分唯一索引：同类型同槽位只允许一个已确认预约
			return nil, "", httpx.E("booking/slot_unavailable", "该时间段已被占用，请选择其他时间")
		}
		return nil, "", err
	}
	bid, err := res.LastInsertId()
	if err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	b := &Booking{
		ID: bid, EventTypeID: et.ID, HostID: host.ID, CalendarID: et.CalendarID,
		EventID: &ev.ID, GuestName: guestName, GuestEmail: guestEmail,
		GuestNote: guestNote, GuestTZ: guestTZ,
		Start: start, End: end, Status: "confirmed",
		ManageToken: cal.HashToken(token), ICSUID: icsUID,
		CreatedAt: time.Now().UTC(),
	}
	return b, token, nil
}

// cancelBooking 取消预约（byWho: host|guest）：预约行置 cancelled、
// 关联事件置 STATUS:CANCELLED，返回更新后的行。
func cancelBooking(ctx context.Context, db *sql.DB, b *Booking, byWho, reason string) (*Booking, error) {
	if b.Status == "cancelled" {
		return nil, httpx.E("booking/already_cancelled", "预约已取消")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := cal.NowUTCString()
	if _, err := tx.ExecContext(ctx,
		`UPDATE bookings SET status='cancelled', cancelled_at=?, cancelled_by=?, cancel_reason=?, updated_at=? WHERE id=?`,
		now, byWho, reason, now, b.ID); err != nil {
		return nil, err
	}
	if b.EventID != nil {
		if _, err := cal.MutateEvent(ctx, tx, *b.EventID, func(ev *icsComponent) error {
			ev.Props.SetText(statusProp, "CANCELLED")
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339, now)
	out := *b
	out.Status = "cancelled"
	out.CancelledAt = &t
	out.CancelledBy = byWho
	out.CancelReason = reason
	return &out, nil
}

// rescheduleBooking 改期（重校验槽位，排除自身事件与预约占位）。
func rescheduleBooking(ctx context.Context, db *sql.DB, host *Host, et *EventType, b *Booking, start time.Time) (*Booking, error) {
	if b.Status == "cancelled" {
		return nil, httpx.E("booking/already_cancelled", "预约已取消")
	}
	_, set, err := eventTypeWithSet(ctx, db, et)
	if err != nil {
		return nil, err
	}
	end := start.Add(time.Duration(et.DurationMinutes) * time.Minute)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var excludeEvent int64
	if b.EventID != nil {
		excludeEvent = *b.EventID
	}
	if err := ValidateSlot(ctx, tx, host, et, set, start, time.Now().UTC(), excludeEvent, b.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bookings SET start_utc=?, end_utc=?, updated_at=? WHERE id=?`,
		cal.UTCString(start), cal.UTCString(end), cal.NowUTCString(), b.ID); err != nil {
		return nil, err
	}
	if b.EventID != nil {
		tz := set.Timezone
		if _, err := cal.MutateEvent(ctx, tx, *b.EventID, func(ev *icsComponent) error {
			setDTStartEnd(ev, start, end, tz)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := *b
	out.Start, out.End = start, end
	return &out, nil
}

// ---- 主人视角 REST ----

// meBookings GET /api/v1/me/bookings?from&to&status&page&page_size
func (h *handler) meBookings(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	page, pageSize := httpx.PageParams(r)
	q := r.URL.Query()
	where := " WHERE b.host_id = ?"
	args := []any{u.ID}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"from": "开始时间格式无效"}))
			return
		}
		where += " AND b.start_utc >= ?"
		args = append(args, cal.UTCString(t))
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"to": "结束时间格式无效"}))
			return
		}
		where += " AND b.start_utc < ?"
		args = append(args, cal.UTCString(t))
	}
	if v := q.Get("status"); v != "" {
		if v != "confirmed" && v != "cancelled" {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"status": "状态只能为 confirmed 或 cancelled"}))
			return
		}
		where += " AND b.status = ?"
		args = append(args, v)
	}

	var total int64
	if err := h.a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM bookings b`+where, args...).Scan(&total); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(), `
		SELECT b.id, b.guest_name, b.guest_email, b.guest_note, b.start_utc, b.end_utc, b.status, b.created_at,
		       t.id, t.name, t.slug
		FROM bookings b JOIN event_types t ON t.id = b.event_type_id`+where+`
		ORDER BY b.start_utc DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, args...), pageSize, (page-1)*pageSize)...)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, typeID int64
		var guestName, guestEmail, guestNote, start, end, status, createdAt, typeName, typeSlug string
		if err := rows.Scan(&id, &guestName, &guestEmail, &guestNote, &start, &end, &status, &createdAt,
			&typeID, &typeName, &typeSlug); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		out = append(out, map[string]any{
			"id": id, "guest_name": guestName, "guest_email": guestEmail, "guest_note": guestNote,
			"start": start, "end": end, "status": status, "created_at": createdAt,
			"event_type": map[string]any{"id": typeID, "name": typeName, "slug": typeSlug},
		})
	}
	httpx.WriteList(w, page, pageSize, int(total), out)
}

// hostCancel POST /api/v1/bookings/{id}/cancel
func (h *handler) hostCancel(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	b, err := bookingByID(r.Context(), h.a.DB, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if b.HostID != u.ID {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/not_found", "预约不存在"))
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := httpx.ReadJSON(r, &req); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) > 500 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"reason": "取消原因过长"}))
		return
	}
	out, err := cancelBooking(r.Context(), h.a.DB, b, "host", req.Reason)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	h.notifyCancelled(r.Context(), out)
	httpx.WriteData(w, http.StatusOK, nil)
}
