// 管理端用户管理：列表 / 状态与角色 / 软删 / 发重置邮件。
package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := httpx.PageParams(r)
	q := r.URL.Query()
	status, role, search := q.Get("status"), q.Get("role"), strings.TrimSpace(q.Get("query"))

	where := " WHERE u.status<>'deleted'"
	args := []any{}
	if status != "" {
		if status != "active" && status != "disabled" && status != "deleted" {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"status": "状态筛选无效"}))
			return
		}
		where += " AND u.status=?"
		args = append(args, status)
	}
	if role != "" {
		if role != "user" && role != "admin" {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"role": "角色筛选无效"}))
			return
		}
		where += " AND u.role=?"
		args = append(args, role)
	}
	if search != "" {
		like := "%" + search + "%"
		where += " AND (u.email LIKE ? OR u.username LIKE ? OR u.display_name LIKE ?)"
		args = append(args, like, like, like)
	}

	var total int64
	if err := h.a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users u`+where, args...).Scan(&total); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := h.a.DB.QueryContext(r.Context(), `
		SELECT u.id, u.email, u.username, u.display_name, u.bio, u.timezone, u.locale, u.role, u.status,
		       u.email_verified_at, u.last_login_at, u.created_at,
		       (SELECT COUNT(*) FROM calendars c WHERE c.owner_id=u.id AND c.deleted_at IS NULL) AS calendars_count
		FROM users u`+where+` ORDER BY u.id LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, calendars int64
		var email, username, displayName, bio, tz, locale, roleV, statusV, createdAt string
		var verifiedAt, lastLogin sql.NullString
		if err := rows.Scan(&id, &email, &username, &displayName, &bio, &tz, &locale, &roleV, &statusV,
			&verifiedAt, &lastLogin, &createdAt, &calendars); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		var lastLoginOut any
		if lastLogin.Valid && lastLogin.String != "" {
			lastLoginOut = lastLogin.String
		}
		out = append(out, map[string]any{
			"id": id, "email": email, "username": username, "display_name": displayName,
			"bio": bio, "timezone": tz, "locale": locale, "role": roleV, "status": statusV,
			"email_verified": verifiedAt.Valid && verifiedAt.String != "",
			"created_at":     createdAt, "last_login_at": lastLoginOut,
			"calendars_count": calendars,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteList(w, page, pageSize, int(total), out)
}

func (h *handler) patchUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "用户 ID 无效"}))
		return
	}
	var req struct {
		Status *string `json:"status"`
		Role   *string `json:"role"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.Status == nil && req.Role == nil {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"body": "至少提供 status 或 role 之一"}))
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"status": "状态只能为 active 或 disabled"}))
		return
	}
	if req.Role != nil && *req.Role != "user" && *req.Role != "admin" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"role": "角色只能为 user 或 admin"}))
		return
	}
	actor, _ := auth.From(r.Context())
	if actor != nil && actor.ID == id {
		// 禁止对自己降级/停用（防止最后一个管理员自锁）。
		if (req.Role != nil && *req.Role != "admin") || (req.Status != nil && *req.Status != "active") {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "不能停用或降级自己的账号"}))
			return
		}
	}
	target, err := user.UserByID(r.Context(), h.a.DB, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if target.Status == "deleted" {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	detail := map[string]any{}
	if req.Status != nil && *req.Status != target.Status {
		detail["status_from"], detail["status_to"] = target.Status, *req.Status
		target.Status = *req.Status
	}
	if req.Role != nil && *req.Role != target.Role {
		detail["role_from"], detail["role_to"] = target.Role, *req.Role
		target.Role = *req.Role
	}
	if _, err := h.a.DB.ExecContext(r.Context(),
		`UPDATE users SET status=?, role=?, updated_at=? WHERE id=?`,
		target.Status, target.Role, user.NowUTC(), id); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if req.Status != nil && *req.Status == "disabled" {
		if err := h.sessions.RevokeAll(r.Context(), id); err != nil {
			h.a.Log.Error("revoke_sessions_on_disable_failed", "user_id", id, "error", err)
		}
	}
	action := "user.update"
	if v, ok := detail["status_to"]; ok {
		if v == "disabled" {
			action = "user.disable"
		} else if v == "active" {
			action = "user.enable"
		}
	}
	if _, ok := detail["role_to"]; ok {
		action = "user.role_change"
	}
	h.adminAudit(r, action, "user", strconv.FormatInt(id, 10), detail)
	full, _ := user.UserByID(r.Context(), h.a.DB, id)
	httpx.WriteData(w, http.StatusOK, full.JSON())
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "用户 ID 无效"}))
		return
	}
	actor, _ := auth.From(r.Context())
	if actor != nil && actor.ID == id {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "不能删除自己的账号"}))
		return
	}
	target, err := user.UserByID(r.Context(), h.a.DB, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && target.Status == "deleted") {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	// 软删：邮箱按契约改写；用户名同样改写以释放唯一约束（可重新注册同名）。
	deletedEmail := fmt.Sprintf("deleted+%d@invalid", id)
	deletedUsername := fmt.Sprintf("deleted_%d", id)
	if _, err := h.a.DB.ExecContext(r.Context(),
		`UPDATE users SET status='deleted', email=?, username=?, updated_at=? WHERE id=?`,
		deletedEmail, deletedUsername, user.NowUTC(), id); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := h.sessions.RevokeAll(r.Context(), id); err != nil {
		h.a.Log.Error("revoke_sessions_on_delete_failed", "user_id", id, "error", err)
	}
	h.adminAudit(r, "user.delete", "user", strconv.FormatInt(id, 10), map[string]any{
		"email": target.Email, "username": target.Username,
	})
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) sendReset(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "用户 ID 无效"}))
		return
	}
	target, err := user.UserByID(r.Context(), h.a.DB, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && target.Status == "deleted") {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := user.SendResetEmail(r.Context(), h.a, target); err != nil {
		h.a.Log.Error("send_reset_email_failed", "user_id", id, "error", err)
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "邮件发送失败，请稍后重试").WithCause(err))
		return
	}
	h.adminAudit(r, "user.send_reset", "user", strconv.FormatInt(id, 10), nil)
	httpx.WriteData(w, http.StatusOK, nil)
}
