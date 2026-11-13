package cal

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// Register 挂载日历域路由（认证由上层中间件保证；处理器内仍做兜底校验）。
func Register(mux *http.ServeMux, a *app.App) {
	h := &handlers{a: a}

	mux.HandleFunc("GET /api/v1/calendars", h.listCalendars)
	mux.HandleFunc("POST /api/v1/calendars", h.createCalendar)
	mux.HandleFunc("PATCH /api/v1/calendars/{id}", h.patchCalendar)
	mux.HandleFunc("DELETE /api/v1/calendars/{id}", h.deleteCalendar)
	mux.HandleFunc("GET /api/v1/calendars/{id}/events", h.listEvents)
	mux.HandleFunc("POST /api/v1/calendars/{id}/events", h.createEvent)
	mux.HandleFunc("POST /api/v1/calendars/{id}/import", h.importICS)
	mux.HandleFunc("PATCH /api/v1/events/{id}", h.patchEvent)
	mux.HandleFunc("DELETE /api/v1/events/{id}", h.deleteEvent)

	mux.HandleFunc("POST /api/v1/calendars/{id}/share-links", h.createShareLink)
	mux.HandleFunc("GET /api/v1/calendars/{id}/share-links", h.listShareLinks)
	mux.HandleFunc("DELETE /api/v1/share-links/{id}", h.revokeShareLink)

	mux.HandleFunc("GET /api/v1/calendars/{id}/members", h.listMembers)
	mux.HandleFunc("POST /api/v1/calendars/{id}/members", h.addMember)
	mux.HandleFunc("DELETE /api/v1/calendars/{id}/members/{user_id}", h.removeMember)

	// 公开访问（无认证；限流由 core 中间件按 /api/v1/pub/ 前缀施加）
	mux.HandleFunc("GET /api/v1/pub/share/{token}", h.pubShareInfo)
	mux.HandleFunc("GET /api/v1/pub/share/{token}/events", h.pubShareEvents)
	mux.HandleFunc("GET /api/v1/pub/share/{token}/ics", h.pubShareICS)
}

type handlers struct{ a *app.App }

func (h *handlers) user(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	u, ok := auth.From(r.Context())
	if !ok || u == nil || u.ID == 0 {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return nil, false
	}
	return u, true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v := r.PathValue(name)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, nil, httpx.Validation(map[string]string{name: "ID 无效"}))
		return 0, false
	}
	return id, true
}

// calendarDTO 是契约 §3 的日历对象。
type calendarDTO struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Color       string      `json:"color"`
	Timezone    string      `json:"timezone"`
	IsDefault   bool        `json:"is_default"`
	Owner       ownerDTO    `json:"owner"`
	Role        string      `json:"role"`
	CreatedAt   string      `json:"created_at"`
}

type ownerDTO struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func (h *handlers) listCalendars(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	own, err := ListCalendarsByOwner(ctx, h.a.DB, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	shared, err := SharedWithMe(ctx, h.a.DB, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	out := make([]calendarDTO, 0, len(own)+len(shared))
	for _, c := range own {
		out = append(out, calendarDTO{
			ID: c.ID, Name: c.Name, Description: c.Description, Color: c.Color,
			Timezone: c.Timezone, IsDefault: c.IsDefault,
			Owner: ownerDTO{ID: u.ID, Username: u.Username, DisplayName: displayNameOf(h.a, u)},
			Role: RoleOwner, CreatedAt: fmtTime(c.CreatedAt),
		})
	}
	for calID, role := range shared {
		c, err := GetCalendar(ctx, h.a.DB, calID)
		if err != nil {
			continue
		}
		owner, err := loadOwnerCtx(h.a, ctx, c.OwnerID)
		if err != nil {
			continue
		}
		out = append(out, calendarDTO{
			ID: c.ID, Name: c.Name, Description: c.Description, Color: c.Color,
			Timezone: c.Timezone, IsDefault: c.IsDefault, Owner: owner,
			Role: role, CreatedAt: fmtTime(c.CreatedAt),
		})
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func displayNameOf(_ *app.App, u *auth.User) string {
	if u == nil {
		return ""
	}
	return u.Username
}

func loadOwnerCtx(a *app.App, ctx context.Context, id int64) (ownerDTO, error) {
	var o ownerDTO
	err := a.DB.QueryRowContext(ctx, `SELECT id, username, display_name FROM users WHERE id = ?`, id).
		Scan(&o.ID, &o.Username, &o.DisplayName)
	return o, err
}

func (h *handlers) createCalendar(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
		Timezone    string `json:"timezone"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.Name == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"name": "名称不能为空"}))
		return
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
		return
	}
	c, err := CreateCalendar(r.Context(), h.a.DB, u.ID, req.Name, req.Description, req.Color, req.Timezone)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusCreated, calendarDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, Color: c.Color,
		Timezone: c.Timezone, IsDefault: c.IsDefault,
		Owner: h.owner(w, r, u.ID), Role: RoleOwner, CreatedAt: fmtTime(c.CreatedAt),
	})
}

func (h *handlers) owner(w http.ResponseWriter, r *http.Request, id int64) ownerDTO {
	o, err := loadOwnerCtx(h.a, r.Context(), id)
	if err != nil {
		return ownerDTO{ID: id}
	}
	return o
}

func (h *handlers) patchCalendar(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	role, err := Access(r.Context(), h.a.DB, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if role != RoleOwner {
		httpx.WriteError(w, h.a.Log, httpx.Forbidden())
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
		Timezone    *string `json:"timezone"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	c, err := UpdateCalendar(r.Context(), h.a.DB, id, req.Name, req.Description, req.Color, req.Timezone)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, calendarDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, Color: c.Color,
		Timezone: c.Timezone, IsDefault: c.IsDefault,
		Owner: h.owner(w, r, c.OwnerID), Role: RoleOwner, CreatedAt: fmtTime(c.CreatedAt),
	})
}

func (h *handlers) deleteCalendar(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	role, err := Access(r.Context(), h.a.DB, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if role != RoleOwner {
		httpx.WriteError(w, h.a.Log, httpx.Forbidden())
		return
	}
	if err := DeleteCalendar(r.Context(), h.a.DB, id); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handlers) requireRole(w http.ResponseWriter, r *http.Request, calendarID int64, roles ...string) (string, bool) {
	u, ok := h.user(w, r)
	if !ok {
		return "", false
	}
	role, err := Access(r.Context(), h.a.DB, calendarID, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return "", false
	}
	for _, want := range roles {
		if role == want {
			return role, true
		}
	}
	if role == "" {
		httpx.WriteError(w, h.a.Log, httpx.E("cal/calendar_not_found", "日历不存在"))
	} else {
		httpx.WriteError(w, h.a.Log, httpx.Forbidden())
	}
	return "", false
}

// parseRange 解析 from/to 查询参数并校验跨度 ≤62 天。
func parseRange(w http.ResponseWriter, a *app.App, r *http.Request) (from, to time.Time, ok bool) {
	q := r.URL.Query()
	fs, ts := q.Get("from"), q.Get("to")
	var err error
	if from, err = parseTime(fs); err != nil {
		httpx.WriteError(w, a.Log, httpx.Validation(map[string]string{"from": "开始时间格式无效"}))
		return
	}
	if to, err = parseTime(ts); err != nil {
		httpx.WriteError(w, a.Log, httpx.Validation(map[string]string{"to": "结束时间格式无效"}))
		return
	}
	if !to.After(from) {
		httpx.WriteError(w, a.Log, httpx.E("cal/invalid_range", "时间范围无效"))
		return
	}
	if to.Sub(from) > 62*24*time.Hour {
		httpx.WriteError(w, a.Log, httpx.E("cal/invalid_range", "时间范围无效（跨度上限 62 天）"))
		return
	}
	return from, to, true
}

func (h *handlers) listEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner, RoleEditor, RoleViewer); !ok {
		return
	}
	from, to, ok := parseRange(w, h.a, r)
	if !ok {
		return
	}
	includeCancelled := true
	if v := r.URL.Query().Get("include_cancelled"); v == "false" || v == "0" {
		includeCancelled = false
	}
	rows, err := LoadEventRowsInRange(r.Context(), h.a.DB, []int64{id}, from, to)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, ExpandRange(rows, from, to, includeCancelled))
}

func (h *handlers) createEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner, RoleEditor); !ok {
		return
	}
	var req CreateEventRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	row, err := CreateEvent(r.Context(), h.a.DB, id, &req)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusCreated, baseInstance(row))
}

func (h *handlers) patchEvent(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	row, err := GetEvent(r.Context(), h.a.DB, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	role, err := Access(r.Context(), h.a.DB, row.CalendarID, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if !CanEditEvents(role) {
		if role == "" {
			httpx.WriteError(w, h.a.Log, httpx.E("cal/event_not_found", "事件不存在"))
		} else {
			httpx.WriteError(w, h.a.Log, httpx.Forbidden())
		}
		return
	}
	var req EventPatch
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "single" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"scope": "scope 必须为 all 或 single"}))
		return
	}
	if req.RecurrenceID == "" {
		req.RecurrenceID = r.URL.Query().Get("recurrence_id")
	}
	out, err := PatchEvent(r.Context(), h.a.DB, id, scope, &req)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	master, _ := masterRowOf(r.Context(), h.a.DB, out)
	httpx.WriteData(w, http.StatusOK, rowInstance(master, out, out.RecurrenceID != nil))
}

func (h *handlers) deleteEvent(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	row, err := GetEvent(r.Context(), h.a.DB, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	role, err := Access(r.Context(), h.a.DB, row.CalendarID, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if !CanEditEvents(role) {
		if role == "" {
			httpx.WriteError(w, h.a.Log, httpx.E("cal/event_not_found", "事件不存在"))
		} else {
			httpx.WriteError(w, h.a.Log, httpx.Forbidden())
		}
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "all"
	}
	if err := DeleteEvent(r.Context(), h.a.DB, id, scope, r.URL.Query().Get("recurrence_id")); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handlers) importICS(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner, RoleEditor); !ok {
		return
	}
	var req struct {
		ICS string `json:"ics"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.ICS == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"ics": "缺少日历数据"}))
		return
	}
	imported, skipped, err := ImportICS(r.Context(), h.a.DB, id, req.ICS)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped})
}

func (h *handlers) createShareLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner); !ok {
		return
	}
	var req struct {
		Label     string `json:"label"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	var exp *time.Time
	if req.ExpiresAt != "" {
		t, err := parseTime(req.ExpiresAt)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"expires_at": "过期时间格式无效"}))
			return
		}
		exp = &t
	}
	sl, err := CreateShareLink(r.Context(), h.a.DB, id, req.Label, exp)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusCreated, sl)
}

func (h *handlers) listShareLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner); !ok {
		return
	}
	links, err := ListShareLinks(r.Context(), h.a.DB, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if links == nil {
		links = []*ShareLink{}
	}
	httpx.WriteData(w, http.StatusOK, links)
}

func (h *handlers) revokeShareLink(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := RevokeShareLink(r.Context(), h.a.DB, id, u.ID); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handlers) listMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner); !ok {
		return
	}
	ms, err := ListMembers(r.Context(), h.a.DB, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if ms == nil {
		ms = []Member{}
	}
	httpx.WriteData(w, http.StatusOK, ms)
}

func (h *handlers) addMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner); !ok {
		return
	}
	var req struct {
		UsernameOrEmail string `json:"username_or_email"`
		Role            string `json:"role"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.UsernameOrEmail == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"username_or_email": "用户名或邮箱不能为空"}))
		return
	}
	if req.Role == "" {
		req.Role = RoleViewer
	}
	uid, _, _, err := ResolveUser(r.Context(), h.a.DB, req.UsernameOrEmail)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := AddMember(r.Context(), h.a.DB, id, uid, req.Role); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusCreated, nil)
}

func (h *handlers) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireRole(w, r, id, RoleOwner); !ok {
		return
	}
	uid, ok := pathID(w, r, "user_id")
	if !ok {
		return
	}
	if err := RemoveMember(r.Context(), h.a.DB, id, uid); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}
