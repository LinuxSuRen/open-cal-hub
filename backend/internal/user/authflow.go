// 认证流程端点：登录/刷新/登出/会话管理/邮箱验证/忘记与重置密码。
package user

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Email))
	if login == "" || req.Password == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"email": "邮箱与密码不能为空", "password": "邮箱与密码不能为空"}))
		return
	}
	u, err := UserByLogin(r.Context(), h.a.DB, login)
	if errors.Is(err, sql.ErrNoRows) {
		h.securityAudit(r, 0, "auth.login_failed", "user", login, map[string]any{"reason": "unknown_login"})
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_credentials", "邮箱或密码不正确"))
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	switch u.Status {
	case "deleted":
		h.securityAudit(r, u.ID, "auth.login_failed", "user", strconv.FormatInt(u.ID, 10), map[string]any{"reason": "deleted"})
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_credentials", "邮箱或密码不正确"))
		return
	case "disabled":
		h.securityAudit(r, u.ID, "auth.login_failed", "user", strconv.FormatInt(u.ID, 10), map[string]any{"reason": "disabled"})
		httpx.WriteError(w, h.a.Log, httpx.E("auth/account_disabled", "账号已被停用"))
		return
	}
	if !auth.CheckPassword(req.Password, u.PasswordHash) {
		h.securityAudit(r, u.ID, "auth.login_failed", "user", strconv.FormatInt(u.ID, 10), map[string]any{"reason": "bad_password"})
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_credentials", "邮箱或密码不正确"))
		return
	}
	if e := h.requireVerified(r.Context(), u); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	h.issueSession(w, r, u)
}

// issueSession 建会话 + 发访问令牌 + 更新 last_login_at。
func (h *handler) issueSession(w http.ResponseWriter, r *http.Request, u *User) {
	token, _, err := h.sessions.Create(r.Context(), u.ID, r.UserAgent(), httpx.ClientIP(r), h.a.Cfg.Auth.RefreshTokenTTL)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := TouchLogin(r.Context(), h.a.DB, u.ID); err != nil {
		h.a.Log.Error("touch_login_failed", "user_id", u.ID, "error", err)
	}
	access, err := h.a.Signer.Sign(u.ID, u.Role, u.Username)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	h.setRefreshCookie(w, r, token)
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"access_token": access,
		"expires_in":   int(h.a.Signer.TTL().Seconds()),
		"user":         u.JSON(),
	})
}

func (h *handler) refresh(w http.ResponseWriter, r *http.Request) {
	plain := refreshCookieValue(r)
	if plain == "" {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	newToken, uid, err := h.sessions.Rotate(r.Context(), plain, h.a.Cfg.Auth.RefreshTokenTTL)
	if errors.Is(err, auth.ErrTokenReuse) {
		h.securityAudit(r, uid, "auth.refresh_reuse", "session", strconv.FormatInt(auth.SessionIDFromToken(plain), 10), map[string]any{"action": "family_revoked"})
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	if errors.Is(err, auth.ErrSessionExpired) || errors.Is(err, auth.ErrSessionNotFound) {
		h.clearRefreshCookie(w)
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	u, err := UserByID(r.Context(), h.a.DB, uid)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && u.Status != "active") {
		h.clearRefreshCookie(w)
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	access, err := h.a.Signer.Sign(u.ID, u.Role, u.Username)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	h.setRefreshCookie(w, r, newToken)
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"access_token": access,
		"expires_in":   int(h.a.Signer.TTL().Seconds()),
		"user":         u.JSON(),
	})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if plain := refreshCookieValue(r); plain != "" {
		if sid := auth.SessionIDFromToken(plain); sid > 0 {
			if err := h.sessions.Revoke(r.Context(), sid); err != nil {
				h.a.Log.Error("logout_revoke_failed", "session_id", sid, "error", err)
			}
		}
	}
	h.clearRefreshCookie(w)
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) listSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.From(r.Context())
	list, err := h.sessions.List(r.Context(), u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	current := auth.SessionIDFromToken(refreshCookieValue(r))
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		var last any
		if s.LastUsedAt.Valid && s.LastUsedAt.String != "" {
			last = s.LastUsedAt.String
		}
		out = append(out, map[string]any{
			"id": s.ID, "user_agent": s.UserAgent, "ip": s.IP,
			"created_at": s.CreatedAt, "last_used_at": last, "current": s.ID == current,
		})
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) revokeSession(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.From(r.Context())
	sid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || sid <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "会话 ID 无效"}))
		return
	}
	ok, err := h.sessions.RevokeByIDForUser(r.Context(), u.ID, sid)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if !ok {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

// ---- 邮箱验证 / 忘记密码 / 重置密码 ----

func (h *handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "链接无效或已使用"))
		return
	}
	uid, err := auth.ConsumeToken(r.Context(), h.a.DB, "verify_email", req.Token)
	if errors.Is(err, auth.ErrTokenInvalid) {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "链接无效或已使用"))
		return
	}
	if errors.Is(err, auth.ErrTokenExpired) {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/token_expired", "链接已过期，请重新发起"))
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := MarkEmailVerified(r.Context(), h.a.DB, uid); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	h.securityAudit(r, uid, "auth.email_verified", "user", strconv.FormatInt(uid, 10), nil)
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	// 不泄露邮箱是否存在：统一成功。
	if u, err := UserByLogin(r.Context(), h.a.DB, NormalizeEmail(req.Email)); err == nil && u.Status == "active" &&
		(!u.EmailVerifiedAt.Valid || u.EmailVerifiedAt.String == "") {
		if err := h.sendVerifyEmail(r.Context(), u); err != nil {
			h.a.Log.Error("send_verify_email_failed", "user_id", u.ID, "error", err)
		}
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if u, err := UserByLogin(r.Context(), h.a.DB, NormalizeEmail(req.Email)); err == nil && u.Status == "active" {
		if err := h.sendResetEmail(r.Context(), u); err != nil {
			h.a.Log.Error("send_reset_email_failed", "user_id", u.ID, "error", err)
		}
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if e := ValidatePasswordStrength(req.NewPassword); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	uid, err := auth.ConsumeToken(r.Context(), h.a.DB, "reset_password", req.Token)
	if errors.Is(err, auth.ErrTokenInvalid) {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "链接无效或已使用"))
		return
	}
	if errors.Is(err, auth.ErrTokenExpired) {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/token_expired", "链接已过期，请重新发起"))
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if err := SetPasswordHash(r.Context(), h.a.DB, uid, hash); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	// 重置密码后吊销全部会话（含被盗 family）。
	if err := h.sessions.RevokeAll(r.Context(), uid); err != nil {
		h.a.Log.Error("revoke_sessions_after_reset_failed", "user_id", uid, "error", err)
	}
	h.securityAudit(r, uid, "auth.password_reset", "user", strconv.FormatInt(uid, 10), nil)
	httpx.WriteData(w, http.StatusOK, nil)
}
