package cal

import (
	"net/http"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// pubShareInfo GET /pub/share/{token}
func (h *handlers) pubShareInfo(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	c, err := ResolveShare(r.Context(), h.a.DB, token)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	owner, err := loadOwnerCtx(h.a, r.Context(), c.OwnerID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	site := SiteName(r.Context(), h.a.DB)
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"site_name": site,
		"calendar": map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"color":       c.Color,
		},
		"owner_display_name": owner.DisplayName,
		"timezone":           c.Timezone,
	})
}

// pubShareEvents GET /pub/share/{token}/events?from&to
func (h *handlers) pubShareEvents(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	c, err := ResolveShare(r.Context(), h.a.DB, token)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	from, to, ok := parseRange(w, h.a, r)
	if !ok {
		return
	}
	rows, err := LoadEventRowsInRange(r.Context(), h.a.DB, []int64{c.ID}, from, to)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, ExpandRange(rows, from, to, true))
}

// pubShareICS GET /pub/share/{token}/ics — 过去 30 天至未来 365 天展开。
func (h *handlers) pubShareICS(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	c, err := ResolveShare(r.Context(), h.a.DB, token)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	now := nowUTC()
	from, to := now.Add(-30*24*time.Hour), now.Add(365*24*time.Hour)
	rows, err := LoadEventRowsInRange(r.Context(), h.a.DB, []int64{c.ID}, from, to)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	instances := ExpandRange(rows, from, to, false)
	var sb strings.Builder
	if err := ics.NewEncoder(&crlfWriter{w: &sb}).Encode(BuildFeedCalendar(instances, rows)); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="calendar.ics"`)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// BuildFeedCalendar 把展开实例合成为 METHOD:PUBLISH 的 VCALENDAR（订阅源）。
func BuildFeedCalendar(instances []Instance, rows []*EventRow) *ics.Calendar {
	byID := make(map[int64]*EventRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	cal := ics.NewCalendar()
	cal.Props.SetText(ics.PropProductID, ProductID)
	cal.Props.SetText(ics.PropVersion, "2.0")
	cal.Props.SetText(ics.PropMethod, "PUBLISH")
	cal.Props.SetText(ics.PropCalendarScale, "GREGORIAN")
	stamp := nowUTC()
	for _, inst := range instances {
		ev := ics.NewEvent()
		ev.Props.SetText(ics.PropUID, inst.UID)
		ev.Props.SetDateTime(ics.PropDateTimeStamp, stamp)
		ev.Props.SetText(ics.PropSummary, inst.Summary)
		if inst.Location != "" {
			ev.Props.SetText(ics.PropLocation, inst.Location)
		}
		if inst.Description != "" {
			ev.Props.SetText(ics.PropDescription, inst.Description)
		}
		start, err1 := parseTime(inst.Start)
		end, err2 := parseTime(inst.End)
		if err1 != nil || err2 != nil {
			continue
		}
		tz := inst.TZ
		loc := tzLocationOrUTC(tz)
		if inst.AllDay {
			ev.Props.SetDate(ics.PropDateTimeStart, start.UTC())
			ev.Props.SetDate(ics.PropDateTimeEnd, end.UTC())
		} else {
			ev.Props.SetDateTime(ics.PropDateTimeStart, start.In(loc))
			ev.Props.SetDateTime(ics.PropDateTimeEnd, end.In(loc))
		}
		if inst.RecurrenceID != nil {
			if rid, err := parseTime(*inst.RecurrenceID); err == nil {
				p := ics.NewProp(ics.PropRecurrenceID)
				p.SetDateTime(rid.In(loc))
				ev.Props.Set(p)
			}
		}
		if inst.Status != StatusConfirmed {
			ev.Props.SetText(ics.PropStatus, strings.ToUpper(inst.Status))
		}
		cal.Children = append(cal.Children, ev.Component)
	}
	if len(cal.Children) == 0 {
		// 编码器要求 VCALENDAR 至少一个子组件
		ev := ics.NewEvent()
		ev.Props.SetText(ics.PropUID, NewUID())
		ev.Props.SetDateTime(ics.PropDateTimeStamp, stamp)
		ev.Props.SetText(ics.PropSummary, "(空日历)")
		ev.Props.SetDate(ics.PropDateTimeStart, nowUTC().Truncate(24*time.Hour))
		ev.Props.SetDate(ics.PropDateTimeEnd, nowUTC().Truncate(24*time.Hour).Add(24*time.Hour))
		cal.Children = append(cal.Children, ev.Component)
	}
	return cal
}
