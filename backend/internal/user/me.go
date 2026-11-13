// 账号自助端点：/me 读写、改密、应用密码（CalDAV 用）。
package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

func (h *handler) meGet(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.From(r.Context())
	full, err := UserByID(r.Context(), h.a.DB, u.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusOK, full.JSON())
}

type patchMeReq struct {
	DisplayName *string `json:"display_name"`
	Timezone    *string `json:"timezone"`
	Locale      *string `json:"locale"`
	Bio         *string `json:"bio"`
}

func (h *handler) mePatch(w http.ResponseWriter, r *http.Request) {
	var req patchMeReq
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.DisplayName != nil {
		if len(*req.DisplayName) > 100 {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"display_name": "长度不能超过 100"}))
			return
		}
	}
	if req.Timezone != nil && *req.Timezone != "" {
		if _, err := time.LoadLocation(*req.Timezone); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
			return
		}
	}
	if req.Locale != nil {
		if len(*req.Locale) > 16 {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"locale": "语言代码无效"}))
			return
		}
	}
	if req.Bio != nil && len(*req.Bio) > 500 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"bio": "长度不能超过 500"}))
		return
	}
	u, _ := auth.From(r.Context())
	if err := UpdateProfile(r.Context(), h.a.DB, u.ID, req.DisplayName, req.Timezone, req.Locale, req.Bio); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	full, err := UserByID(r.Context(), h.a.DB, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusOK, full.JSON())
}

func (h *handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if e := ValidatePasswordStrength(req.NewPassword); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	au, _ := auth.From(r.Context())
	u, err := UserByID(r.Context(), h.a.DB, au.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if !auth.CheckPassword(req.CurrentPassword, u.PasswordHash) {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_credentials", "当前密码不正确"))
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := SetPasswordHash(r.Context(), h.a.DB, u.ID, hash); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	// 吊销其他会话，保留当前登录。
	if sid := auth.SessionIDFromToken(refreshCookieValue(r)); sid > 0 {
		if err := h.sessions.RevokeOthers(r.Context(), u.ID, sid); err != nil {
			h.a.Log.Error("revoke_others_failed", "user_id", u.ID, "error", err)
		}
	} else if err := h.sessions.RevokeAll(r.Context(), u.ID); err != nil {
		h.a.Log.Error("revoke_all_failed", "user_id", u.ID, "error", err)
	}
	h.securityAudit(r, u.ID, "user.password_change", "user", strconv.FormatInt(u.ID, 10), nil)
	httpx.WriteData(w, http.StatusOK, nil)
}

// ---- 应用密码 ----

type appPasswordJSON struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func (h *handler) listAppPasswords(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.From(r.Context())
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT id, label, created_at, last_used_at FROM app_passwords WHERE user_id=? ORDER BY id DESC`, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []appPasswordJSON{}
	for rows.Next() {
		var p appPasswordJSON
		var last sql.NullString
		if err := rows.Scan(&p.ID, &p.Label, &p.CreatedAt, &last); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		if last.Valid && last.String != "" {
			s := last.String
			p.LastUsedAt = &s
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) createAppPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if len(req.Label) < 1 || len(req.Label) > 50 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"label": "名称长度需为 1-50 个字符"}))
		return
	}
	plain, err := newAppPasswordSecret()
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	hash, err := auth.HashPassword(plain)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	u, _ := auth.From(r.Context())
	res, err := h.a.DB.ExecContext(r.Context(),
		`INSERT INTO app_passwords (user_id, label, password_hash, created_at) VALUES (?,?,?,?)`,
		u.ID, req.Label, hash, nowUTC())
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	id, _ := res.LastInsertId()
	h.securityAudit(r, u.ID, "app_password.create", "app_password", strconv.FormatInt(id, 10), map[string]any{"label": req.Label})
	// 明文仅此一次返回。
	httpx.WriteData(w, http.StatusCreated, map[string]any{"id": id, "password": plain})
}

func (h *handler) deleteAppPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "ID 无效"}))
		return
	}
	u, _ := auth.From(r.Context())
	res, err := h.a.DB.ExecContext(r.Context(), `DELETE FROM app_passwords WHERE id=? AND user_id=?`, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	h.securityAudit(r, u.ID, "app_password.delete", "app_password", strconv.FormatInt(id, 10), nil)
	httpx.WriteData(w, http.StatusOK, nil)
}

// ---- 校验（dav 包复用） ----

// VerifyPassword 校验登录密码或任一应用密码（命中应用密码时更新 last_used_at）。
// status != active 直接返回 false。签名按团队约定导出，db 显式传入便于测试。
func VerifyPassword(ctx context.Context, db *sql.DB, u *User, password string) bool {
	if u == nil || u.Status != "active" || password == "" {
		return false
	}
	if auth.CheckPassword(password, u.PasswordHash) {
		return true
	}
	rows, err := db.QueryContext(ctx, `SELECT id, password_hash FROM app_passwords WHERE user_id=?`, u.ID)
	if err != nil {
		return false
	}
	// 注意：连接池为单连接，rows 未关闭前不得执行其他语句；先收集结果再更新。
	type pair struct {
		id   int64
		hash string
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.hash); err == nil {
			pairs = append(pairs, p)
		}
	}
	rows.Close()
	for _, p := range pairs {
		if auth.CheckPassword(password, p.hash) {
			_, _ = db.ExecContext(ctx, `UPDATE app_passwords SET last_used_at=? WHERE id=?`, nowUTC(), p.id)
			return true
		}
	}
	return false
}

// AuthenticateBasic 以「用户名或邮箱 + 密码/应用密码」完整认证（dav Basic 认证入口）。
func AuthenticateBasic(ctx context.Context, db *sql.DB, login, password string) (*User, bool) {
	u, err := UserByLogin(ctx, db, login)
	if err != nil {
		return nil, false
	}
	if !VerifyPassword(ctx, db, u, password) {
		return nil, false
	}
	return u, true
}

// newAppPasswordSecret 生成形如 ab3d-ef7g-hij4-klm9-opq2 的应用密码（无歧义字符集）。
func newAppPasswordSecret() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	raw, err := randomBytes(20)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, 24)
	for i, b := range raw {
		if i > 0 && i%5 == 0 {
			out = append(out, '-')
		}
		out = append(out, alphabet[int(b)%len(alphabet)])
	}
	return string(out), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
