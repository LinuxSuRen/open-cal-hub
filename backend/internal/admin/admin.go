// Package admin：管理端 API（role=admin），全部操作写审计日志。
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

type handler struct {
	a        *app.App
	sessions *auth.Sessions
	audit    *httpx.Audit
}

// Register 挂载 /api/v1/admin/*（RequireAdmin 保护）。
func Register(mux *http.ServeMux, a *app.App) {
	h := &handler{a: a, sessions: &auth.Sessions{DB: a.DB}, audit: &httpx.Audit{DB: a.DB}}
	mw := auth.RequireAdmin(a.Signer, a.LoadUser)
	hd := func(f http.HandlerFunc) http.Handler { return f }

	mux.Handle("GET /api/v1/admin/stats", mw(hd(h.stats)))
	mux.Handle("GET /api/v1/admin/users", mw(hd(h.listUsers)))
	mux.Handle("PATCH /api/v1/admin/users/{id}", mw(hd(h.patchUser)))
	mux.Handle("DELETE /api/v1/admin/users/{id}", mw(hd(h.deleteUser)))
	mux.Handle("POST /api/v1/admin/users/{id}/send-reset", mw(hd(h.sendReset)))
	mux.Handle("GET /api/v1/admin/bookings", mw(hd(h.listBookings)))
	mux.Handle("GET /api/v1/admin/audit-logs", mw(hd(h.listAuditLogs)))
	mux.Handle("GET /api/v1/admin/settings", mw(hd(h.getSettings)))
	mux.Handle("PUT /api/v1/admin/settings", mw(hd(h.putSettings)))
	mux.Handle("GET /api/v1/admin/emails", mw(hd(h.listEmails)))
	mux.Handle("POST /api/v1/admin/emails/{id}/retry", mw(hd(h.retryEmail)))
	mux.Handle("GET /api/v1/admin/invite-codes", mw(hd(h.listInviteCodes)))
	mux.Handle("POST /api/v1/admin/invite-codes", mw(hd(h.createInviteCodes)))
	mux.Handle("DELETE /api/v1/admin/invite-codes/{code}", mw(hd(h.deleteInviteCode)))
}

// adminAudit 管理操作审计；失败记日志（写审计失败不改变操作结果，但必须可发现）。
func (h *handler) adminAudit(r *http.Request, action, targetType, targetID string, detail map[string]any) {
	actor, _ := auth.From(r.Context())
	var actorID int64
	if actor != nil {
		actorID = actor.ID
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := h.audit.Log(ctx, actorID, action, targetType, targetID, httpx.ClientIP(r), detail); err != nil {
		h.a.Log.Error("audit_write_failed", "action", action, "error", err)
	}
}

// ---- GET /admin/stats ----

func (h *handler) stats(w http.ResponseWriter, r *http.Request) {
	cutoff30d := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
	var users, active30d, calendars, events, bookings30d, pendingOutbox int64
	type q struct {
		sql  string
		dst  *int64
		args []any
	}
	for _, item := range []q{
		{`SELECT COUNT(*) FROM users WHERE status<>'deleted'`, &users, nil},
		{`SELECT COUNT(*) FROM users WHERE status<>'deleted' AND last_login_at>=?`, &active30d, []any{cutoff30d}},
		{`SELECT COUNT(*) FROM calendars WHERE deleted_at IS NULL`, &calendars, nil},
		{`SELECT COUNT(*) FROM events`, &events, nil},
		{`SELECT COUNT(*) FROM bookings WHERE created_at>=?`, &bookings30d, []any{cutoff30d}},
		{`SELECT COUNT(*) FROM mail_outbox WHERE status='pending'`, &pendingOutbox, nil},
	} {
		if err := h.a.DB.QueryRowContext(r.Context(), item.sql, item.args...).Scan(item.dst); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"users":            users,
		"active_users_30d": active30d,
		"calendars":        calendars,
		"events":           events,
		"bookings_30d":     bookings30d,
		"pending_outbox":   pendingOutbox,
	})
}

// ---- GET/PUT /admin/settings ----

// settingsWhitelist 键与校验。
var settingsWhitelist = []struct {
	key  string
	kind string // string | mode | bool | int
	min  int
	max  int
}{
	{app.SettingSiteName, "string", 1, 100},
	{app.SettingRegistrationMode, "mode", 0, 0},
	{app.SettingRequireEmailVerification, "bool", 0, 0},
	{app.SettingBookingHorizonDays, "int", 1, 3650},
}

func (h *handler) getSettings(w http.ResponseWriter, r *http.Request) {
	site := h.a.SiteName(r.Context())
	mode, _ := h.a.EffectiveRegistrationMode(r.Context())
	require, _ := h.a.EffectiveRequireEmailVerification(r.Context())
	horizon, _ := h.a.Setting(r.Context(), app.SettingBookingHorizonDays, "365")
	if n, err := strconv.Atoi(horizon); err != nil || n <= 0 {
		horizon = "365"
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"site_name":                  site,
		"registration_mode":          mode,
		"require_email_verification": require,
		"booking_horizon_days":       horizon,
	})
}

func (h *handler) putSettings(w http.ResponseWriter, r *http.Request) {
	// 手工解析：未知键必须给 admin/invalid_setting，不能用 DisallowUnknownFields。
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"body": "请求格式不正确"}))
		return
	}
	if len(body) == 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"body": "请求体不能为空"}))
		return
	}
	updated := map[string]any{}
	for key, raw := range body {
		var def *struct {
			key  string
			kind string
			min  int
			max  int
		}
		for i := range settingsWhitelist {
			if settingsWhitelist[i].key == key {
				def = &settingsWhitelist[i]
				break
			}
		}
		if def == nil {
			httpx.WriteError(w, h.a.Log, httpx.E("admin/invalid_setting", "设置项无效"))
			return
		}
		var value string
		switch def.kind {
		case "string":
			var s string
			if err := json.Unmarshal(raw, &s); err != nil || len(s) < def.min || len(s) > def.max {
				httpx.WriteError(w, h.a.Log, httpx.E("admin/invalid_setting", "设置项无效"))
				return
			}
			value = s
		case "mode":
			var s string
			if err := json.Unmarshal(raw, &s); err != nil || (s != "open" && s != "invite") {
				httpx.WriteError(w, h.a.Log, httpx.E("admin/invalid_setting", "设置项无效"))
				return
			}
			value = s
		case "bool":
			var b bool
			if err := json.Unmarshal(raw, &b); err != nil {
				httpx.WriteError(w, h.a.Log, httpx.E("admin/invalid_setting", "设置项无效"))
				return
			}
			value = strconv.FormatBool(b)
		case "int":
			var n int
			if err := json.Unmarshal(raw, &n); err != nil || n < def.min || n > def.max {
				httpx.WriteError(w, h.a.Log, httpx.E("admin/invalid_setting", "设置项无效"))
				return
			}
			value = strconv.Itoa(n)
		}
		if err := h.a.SetSetting(r.Context(), key, value); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		switch def.kind {
		case "bool":
			b, _ := strconv.ParseBool(value)
			updated[key] = b
		case "int":
			n, _ := strconv.Atoi(value)
			updated[key] = n
		default:
			updated[key] = value
		}
	}
	h.adminAudit(r, "settings.update", "settings", "", updated)
	h.getSettings(w, r)
}

// ---- GET /admin/audit-logs ----

func (h *handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := httpx.PageParams(r)
	var total int64
	if err := h.a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT id, actor_id, action, target_type, target_id, detail, ip, created_at
		 FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var actor sql.NullInt64
		var action, targetType, targetID, detail, ip, createdAt string
		if err := rows.Scan(&id, &actor, &action, &targetType, &targetID, &detail, &ip, &createdAt); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		var detailObj map[string]any
		if err := json.Unmarshal([]byte(detail), &detailObj); err != nil {
			detailObj = map[string]any{"raw": detail}
		}
		var actorOut any
		if actor.Valid {
			actorOut = actor.Int64
		}
		out = append(out, map[string]any{
			"id": id, "actor_id": actorOut, "action": action,
			"target_type": targetType, "target_id": targetID, "detail": detailObj,
			"ip": ip, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteList(w, page, pageSize, int(total), out)
}
