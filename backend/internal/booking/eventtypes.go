package booking

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// eventTypeInput 是创建/更新预约类型的请求体（指针字段区分未提供）。
type eventTypeInput struct {
	Slug             *string `json:"slug"`
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	DurationMinutes  *int    `json:"duration_minutes"`
	LocationType     *string `json:"location_type"`
	LocationValue    *string `json:"location_value"`
	Color            *string `json:"color"`
	BufferBefore     *int    `json:"buffer_before"`
	BufferAfter      *int    `json:"buffer_after"`
	SlotInterval     *int    `json:"slot_interval"`
	MinNoticeMinutes *int    `json:"min_notice_minutes"`
	RangeType        *string `json:"range_type"`
	RangeDays        *int    `json:"range_days"`
	StartDate        *string `json:"start_date"`
	EndDate          *string `json:"end_date"`
	AvailabilityID   *int64  `json:"availability_id"`
	CalendarID       *int64  `json:"calendar_id"`
	Active           *bool   `json:"active"`
}

var locationTypes = map[string]bool{"none": true, "online": true, "in_person": true, "phone": true}

func validateEventType(h *handler, w http.ResponseWriter, r *http.Request, userID int64,
	in eventTypeInput, creating bool) (*eventTypePatch, bool) {
	p := &eventTypePatch{}
	if creating {
		if in.Slug == nil || in.Name == nil || in.DurationMinutes == nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{
				"body": "slug、name、duration_minutes 为必填"}))
			return nil, false
		}
	}
	p.slug, p.name, p.description = in.Slug, in.Name, in.Description
	p.locationType, p.locationValue, p.color = in.LocationType, in.LocationValue, in.Color
	p.duration = in.DurationMinutes
	p.bufferBefore, p.bufferAfter, p.slotInterval, p.minNotice = in.BufferBefore, in.BufferAfter, in.SlotInterval, in.MinNoticeMinutes
	p.rangeType, p.rangeDays, p.startDate, p.endDate = in.RangeType, in.RangeDays, in.StartDate, in.EndDate
	p.availabilityID, p.calendarID, p.active = in.AvailabilityID, in.CalendarID, in.Active

	if p.slug != nil && !slugRe.MatchString(*p.slug) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"slug": "slug 只能包含小写字母、数字与连字符，长度 1..64"}))
		return nil, false
	}
	if p.name != nil && *p.name == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"name": "名称不能为空"}))
		return nil, false
	}
	if p.duration != nil && (*p.duration < 1 || *p.duration > 1440) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"duration_minutes": "时长必须在 1..1440 分钟"}))
		return nil, false
	}
	if p.locationType != nil && !locationTypes[*p.locationType] {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"location_type": "地点类型无效"}))
		return nil, false
	}
	for _, v := range []*int{p.bufferBefore, p.bufferAfter, p.minNotice} {
		if v != nil && (*v < 0 || *v > 60*24*90) {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"buffers": "缓冲或提前通知分钟数无效"}))
			return nil, false
		}
	}
	if p.slotInterval != nil && (*p.slotInterval < 0 || *p.slotInterval > 1440) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"slot_interval": "槽位间隔必须在 0..1440 分钟"}))
		return nil, false
	}
	if p.rangeType != nil && *p.rangeType != "rolling" && *p.rangeType != "fixed" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"range_type": "范围类型必须为 rolling 或 fixed"}))
		return nil, false
	}
	if p.rangeDays != nil && (*p.rangeDays < 1 || *p.rangeDays > 730) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"range_days": "可预约天数必须在 1..730"}))
		return nil, false
	}
	for _, d := range []*string{p.startDate, p.endDate} {
		if d != nil && *d != "" {
			if _, err := time.Parse("2006-01-02", *d); err != nil {
				httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"date": "日期格式必须为 YYYY-MM-DD"}))
				return nil, false
			}
		}
	}
	if p.startDate != nil && p.endDate != nil && *p.startDate != "" && *p.endDate != "" && *p.endDate < *p.startDate {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"end_date": "结束日期不能早于开始日期"}))
		return nil, false
	}
	if p.availabilityID != nil && *p.availabilityID != 0 {
		set, err := LoadAvailability(r.Context(), h.a.DB, *p.availabilityID, userID)
		if err != nil || set == nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"availability_id": "可用时间集不存在"}))
			return nil, false
		}
	}
	if p.calendarID != nil && *p.calendarID != 0 {
		role, err := cal.Access(r.Context(), h.a.DB, *p.calendarID, userID)
		if err != nil || role != cal.RoleOwner {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"calendar_id": "日历不存在或无权限"}))
			return nil, false
		}
	}
	return p, true
}

// eventTypePatch 是归一化后的可写字段集合（nil=不改）。
type eventTypePatch struct {
	slug, name, description, locationType, locationValue, color  *string
	duration, bufferBefore, bufferAfter, slotInterval, minNotice *int
	rangeType                                                    *string
	rangeDays                                                    *int
	startDate, endDate                                           *string
	availabilityID, calendarID                                   *int64
	active                                                       *bool
}

// createEventType POST /event-types
func (h *handler) createEventType(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	var in eventTypeInput
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	p, ok := validateEventType(h, w, r, u.ID, in, true)
	if !ok {
		return
	}
	ctx := r.Context()
	// 日历缺省：用默认日历（无则自动创建）
	var calID int64
	if p.calendarID != nil {
		calID = *p.calendarID
	} else {
		var err error
		calID, err = cal.EnsureDefaultCalendar(ctx, h.a.DB, u.ID, "UTC")
		if err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
	}
	var availID any
	if p.availabilityID != nil && *p.availabilityID != 0 {
		availID = *p.availabilityID
	}
	now := cal.NowUTCString()
	var startDate, endDate any
	if p.startDate != nil && *p.startDate != "" {
		startDate = *p.startDate
	}
	if p.endDate != nil && *p.endDate != "" {
		endDate = *p.endDate
	}
	res, err := h.a.DB.ExecContext(ctx, `INSERT INTO event_types
		(user_id, slug, name, description, duration_minutes, location_type, location_value, color,
		 buffer_before, buffer_after, slot_interval, min_notice_minutes, range_type, range_days,
		 start_date, end_date, availability_id, calendar_id, active, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, *p.slug, *p.name, strOr(p.description, ""), intOr(p.duration, 30),
		strOr(p.locationType, "none"), strOr(p.locationValue, ""), strOr(p.color, "#4F6EF7"),
		intOr(p.bufferBefore, 0), intOr(p.bufferAfter, 0), intOr(p.slotInterval, 0), intOr(p.minNotice, 0),
		strOr(p.rangeType, "rolling"), intOr(p.rangeDays, 60),
		startDate, endDate, availID, calID, boolOr(p.active, true), now, now)
	if err != nil {
		if isUniqueErr(err) {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"slug": "slug 已被使用"}))
			return
		}
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	et, err := EventTypeByID(ctx, h.a.DB, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	et.BookingURL = "/u/" + u.Username + "/" + et.Slug
	httpx.WriteData(w, http.StatusCreated, et)
}

func strOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// listEventTypes GET /event-types
func (h *handler) listEventTypes(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT `+eventTypeCols+` FROM event_types WHERE user_id = ? ORDER BY id`, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer rows.Close()
	out := []*EventType{}
	for rows.Next() {
		et, err := scanEventType(rows.Scan, u.Username)
		if err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		out = append(out, et)
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) loadOwnedEventType(w http.ResponseWriter, r *http.Request) (*EventType, bool) {
	u, ok := h.user(w, r)
	if !ok {
		return nil, false
	}
	id, ok := idParam(w, r)
	if !ok {
		return nil, false
	}
	var owner int64
	err := h.a.DB.QueryRowContext(r.Context(), `SELECT user_id FROM event_types WHERE id = ?`, id).Scan(&owner)
	if err == sql.ErrNoRows {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/event_type_not_found", "预约类型不存在"))
		return nil, false
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return nil, false
	}
	if owner != u.ID {
		httpx.WriteError(w, h.a.Log, httpx.E("booking/event_type_not_found", "预约类型不存在"))
		return nil, false
	}
	et, err := EventTypeByID(r.Context(), h.a.DB, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return nil, false
	}
	return et, true
}

// patchEventType PATCH /event-types/{id}
func (h *handler) patchEventType(w http.ResponseWriter, r *http.Request) {
	et, ok := h.loadOwnedEventType(w, r)
	if !ok {
		return
	}
	var in eventTypeInput
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	u, _ := h.user(w, r)
	p, ok := validateEventType(h, w, r, u.ID, in, false)
	if !ok {
		return
	}
	slug := et.Slug
	if p.slug != nil {
		slug = *p.slug
	}
	name := et.Name
	if p.name != nil {
		name = *p.name
	}
	description := et.Description
	if p.description != nil {
		description = *p.description
	}
	duration := et.DurationMinutes
	if p.duration != nil {
		duration = *p.duration
	}
	locationType := et.LocationType
	if p.locationType != nil {
		locationType = *p.locationType
	}
	locationValue := et.LocationValue
	if p.locationValue != nil {
		locationValue = *p.locationValue
	}
	color := et.Color
	if p.color != nil {
		color = *p.color
	}
	active := et.Active
	if p.active != nil {
		active = *p.active
	}
	rangeType := et.RangeType
	if p.rangeType != nil {
		rangeType = *p.rangeType
	}
	rangeDays := et.RangeDays
	if p.rangeDays != nil {
		rangeDays = *p.rangeDays
	}
	var startDate, endDate any
	if p.startDate != nil {
		if *p.startDate == "" {
			startDate = nil
		} else {
			startDate = *p.startDate
		}
	} else if et.StartDate != nil {
		startDate = *et.StartDate
	}
	if p.endDate != nil {
		if *p.endDate == "" {
			endDate = nil
		} else {
			endDate = *p.endDate
		}
	} else if et.EndDate != nil {
		endDate = *et.EndDate
	}
	var availID any
	if p.availabilityID != nil {
		if *p.availabilityID == 0 {
			availID = nil
		} else {
			availID = *p.availabilityID
		}
	} else if et.AvailabilityID != nil {
		availID = *et.AvailabilityID
	}
	_, err := h.a.DB.ExecContext(r.Context(), `UPDATE event_types SET slug=?, name=?, description=?, duration_minutes=?,
		location_type=?, location_value=?, color=?, buffer_before=?, buffer_after=?, slot_interval=?, min_notice_minutes=?,
		range_type=?, range_days=?, start_date=?, end_date=?, availability_id=?, active=?, updated_at=? WHERE id=?`,
		slug, name, description, duration,
		locationType, locationValue, color,
		intOrDefault(p.bufferBefore, et.BufferBefore), intOrDefault(p.bufferAfter, et.BufferAfter),
		intOrDefault(p.slotInterval, et.SlotInterval), intOrDefault(p.minNotice, et.MinNoticeMinutes),
		rangeType, rangeDays, startDate, endDate, availID, active, cal.NowUTCString(), et.ID)
	if err != nil {
		if isUniqueErr(err) {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"slug": "slug 已被使用"}))
			return
		}
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	updated, err := EventTypeByID(r.Context(), h.a.DB, et.ID, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	updated.BookingURL = "/u/" + u.Username + "/" + updated.Slug
	httpx.WriteData(w, http.StatusOK, updated)
}

func intOrDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// deleteEventType DELETE /event-types/{id}（预约保留，公开页 404）
func (h *handler) deleteEventType(w http.ResponseWriter, r *http.Request) {
	et, ok := h.loadOwnedEventType(w, r)
	if !ok {
		return
	}
	if _, err := h.a.DB.ExecContext(r.Context(), `DELETE FROM event_types WHERE id = ?`, et.ID); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}
