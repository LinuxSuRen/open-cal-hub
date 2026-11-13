// 公开预约端点（无认证，限流由上层 pubGuard 施加）：
// /pub/u/{username}、/pub/u/{username}/{slug}[/slots|/bookings]、
// /pub/b/{token}[/cancel|/reschedule]。
package booking

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

var dateOnlyRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// pubHostProfile GET /api/v1/pub/u/{username}
func (h *handler) pubHostProfile(w http.ResponseWriter, r *http.Request) {
	host, err := LoadHostByUsername(r.Context(), h.a.DB, r.PathValue("username"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT slug, name, duration_minutes, color FROM event_types WHERE user_id = ? AND active = 1 ORDER BY id`, host.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer rows.Close()
	types := []map[string]any{}
	for rows.Next() {
		var slug, name, color string
		var duration int
		if err := rows.Scan(&slug, &name, &duration, &color); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		types = append(types, map[string]any{
			"slug": slug, "name": name, "duration_minutes": duration, "color": color,
		})
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"display_name": host.DisplayName, "bio": host.Bio, "timezone": host.Timezone,
		"event_types": types,
	})
}

// pubEventType GET /api/v1/pub/u/{username}/{slug}
func (h *handler) pubEventType(w http.ResponseWriter, r *http.Request) {
	host, err := LoadHostByUsername(r.Context(), h.a.DB, r.PathValue("username"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	et, err := EventTypeBySlug(r.Context(), h.a.DB, host, r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if !et.Active {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/event_type_not_found", "预约类型不存在"))
		return
	}
	availability := map[string]any{"timezone": host.Timezone, "rules": []Rule{}, "dates": []DateRule{}}
	if et.AvailabilityID != nil {
		if set, err := LoadAvailabilityByID(r.Context(), h.a.DB, *et.AvailabilityID); err == nil && set != nil {
			availability = map[string]any{
				"timezone": set.Timezone,
				"rules":    set.Rules,
				"dates":    set.Dates,
			}
		}
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"slug": et.Slug, "name": et.Name, "description": et.Description,
		"duration_minutes": et.DurationMinutes, "color": et.Color,
		"location_type": et.LocationType, "range_type": et.RangeType,
		"range_days": et.RangeDays, "start_date": et.StartDate, "end_date": et.EndDate,
		"availability": availability,
		"buffers":      map[string]int{"before": et.BufferBefore, "after": et.BufferAfter},
		"slot_interval": et.SlotInterval, "min_notice_minutes": et.MinNoticeMinutes,
	})
}

// pubSlots GET /api/v1/pub/u/{username}/{slug}/slots?start=YYYY-MM-DD&days=7&tz=
func (h *handler) pubSlots(w http.ResponseWriter, r *http.Request) {
	host, et, ok := h.loadPublicEventType(w, r)
	if !ok {
		return
	}
	_, set, err := eventTypeWithSet(r.Context(), h.a.DB, et)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	q := r.URL.Query()
	startStr, daysStr, tz := q.Get("start"), q.Get("days"), q.Get("tz")
	if !dateOnlyRe.MatchString(startStr) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"start": "开始日期必须为 YYYY-MM-DD"}))
		return
	}
	days := 7
	if daysStr != "" {
		n, err := strconv.Atoi(daysStr)
		if err != nil || n < 1 || n > 31 {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"days": "天数必须在 1..31"}))
			return
		}
		days = n
	}
	if tz == "" {
		tz = set.Timezone
	}
	grid, err := time.LoadLocation(tz)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"tz": "时区无效"}))
		return
	}
	startDay, err := time.ParseInLocation("2006-01-02", startStr, grid)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"start": "开始日期无效"}))
		return
	}
	daySlots, err := ComputeSlots(r.Context(), h.a.DB, host, et, set, tz, startDay, days, time.Now().UTC())
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if daySlots == nil {
		daySlots = []DaySlots{}
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{"days": daySlots})
}

// pubCreateBooking POST /api/v1/pub/u/{username}/{slug}/bookings
func (h *handler) pubCreateBooking(w http.ResponseWriter, r *http.Request) {
	host, et, ok := h.loadPublicEventType(w, r)
	if !ok {
		return
	}
	var req struct {
		Start    string `json:"start"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Note     string `json:"note"`
		Timezone string `json:"timezone"`
		Company  string `json:"company"` // 蜜罐
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	// 蜜罐命中：静默成功，不入库
	if strings.TrimSpace(req.Company) != "" {
		httpx.WriteData(w, http.StatusCreated, map[string]any{
			"booking": BookingGuestView{
				ID: 0, EventTypeName: et.Name, HostDisplayName: host.DisplayName,
				Start: req.Start, End: req.Start, Status: "confirmed", CreatedAt: cal.NowUTCString(),
			},
			"manage_url": "/b/" + cal.NewToken(),
		})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"name": "姓名不能为空"}))
		return
	}
	if len(req.Name) > 100 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"name": "姓名过长"}))
		return
	}
	req.Email = user.NormalizeEmail(req.Email)
	if err := user.ValidateEmail(req.Email); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if len(req.Note) > 2000 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"note": "备注过长"}))
		return
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
		return
	}
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"start": "开始时间格式无效"}))
		return
	}
	b, token, err := createBooking(r.Context(), h.a.DB, host, et, req.Name, req.Email, req.Note, req.Timezone, start.UTC())
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	h.notifyConfirmed(r.Context(), host, et, b, token)
	httpx.WriteData(w, http.StatusCreated, map[string]any{
		"booking": BookingGuestView{
			ID: b.ID, EventTypeName: et.Name, HostDisplayName: host.DisplayName,
			Start: cal.UTCString(b.Start), End: cal.UTCString(b.End),
			Status: b.Status, CreatedAt: cal.UTCString(b.CreatedAt),
		},
		"manage_url": "/b/" + token,
	})
}

// loadPublicEventType 校验主人存在、类型存在且 active。
func (h *handler) loadPublicEventType(w http.ResponseWriter, r *http.Request) (*Host, *EventType, bool) {
	host, err := LoadHostByUsername(r.Context(), h.a.DB, r.PathValue("username"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return nil, nil, false
	}
	et, err := EventTypeBySlug(r.Context(), h.a.DB, host, r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return nil, nil, false
	}
	if !et.Active {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/event_type_not_found", "预约类型不存在"))
		return nil, nil, false
	}
	return host, et, true
}

// pubManageView GET /api/v1/pub/b/{token}
func (h *handler) pubManageView(w http.ResponseWriter, r *http.Request) {
	b, err := bookingByManageToken(r.Context(), h.a.DB, r.PathValue("token"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	et, host, ok := h.bookingContext(w, r, b)
	if !ok {
		return
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"event_type": map[string]any{
			"name": et.Name, "slug": et.Slug, "duration_minutes": et.DurationMinutes,
		},
		"host_username":     host.Username,
		"host_display_name": host.DisplayName,
		"start":             cal.UTCString(b.Start),
		"end":               cal.UTCString(b.End),
		"status":            b.Status,
		"guest_name":        b.GuestName,
		"guest_note":        b.GuestNote,
		"timezone":          b.GuestTZ,
	})
}

// bookingContext 取预约的类型与主人（类型被删时返回占位）。
func (h *handler) bookingContext(w http.ResponseWriter, r *http.Request, b *Booking) (*EventType, *Host, bool) {
	et, err := EventTypeByID(r.Context(), h.a.DB, b.EventTypeID, b.HostID)
	if err != nil {
		// 类型已删除：占位展示
		et = &EventType{ID: b.EventTypeID, Name: "预约", DurationMinutes: int(b.End.Sub(b.Start).Minutes())}
	}
	var host Host
	err = h.a.DB.QueryRowContext(r.Context(),
		`SELECT id, username, display_name, bio, timezone, email FROM users WHERE id = ?`, b.HostID).
		Scan(&host.ID, &host.Username, &host.DisplayName, &host.Bio, &host.Timezone, &host.Email)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/not_found", "预约不存在"))
		return nil, nil, false
	}
	return et, &host, true
}

// pubCancel POST /api/v1/pub/b/{token}/cancel（客人取消）
func (h *handler) pubCancel(w http.ResponseWriter, r *http.Request) {
	b, err := bookingByManageToken(r.Context(), h.a.DB, r.PathValue("token"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
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
	out, err := cancelBooking(r.Context(), h.a.DB, b, "guest", req.Reason)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	h.notifyCancelled(r.Context(), out)
	httpx.WriteData(w, http.StatusOK, nil)
}

// pubReschedule POST /api/v1/pub/b/{token}/reschedule（客人改期）
func (h *handler) pubReschedule(w http.ResponseWriter, r *http.Request) {
	b, err := bookingByManageToken(r.Context(), h.a.DB, r.PathValue("token"))
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	var req struct {
		Start string `json:"start"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"start": "开始时间格式无效"}))
		return
	}
	et, host, ok := h.bookingContext(w, r, b)
	if !ok {
		return
	}
	oldStart, oldEnd := b.Start, b.End
	out, err := rescheduleBooking(r.Context(), h.a.DB, host, et, b, start.UTC())
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	h.notifyRescheduled(r.Context(), host, et, out, oldStart, oldEnd)
	httpx.WriteData(w, http.StatusOK, BookingGuestView{
		ID: out.ID, EventTypeName: et.Name, HostDisplayName: host.DisplayName,
		Start: cal.UTCString(out.Start), End: cal.UTCString(out.End),
		Status: out.Status, CreatedAt: cal.UTCString(out.CreatedAt),
	})
}
