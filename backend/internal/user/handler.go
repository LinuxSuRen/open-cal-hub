// user 包 HTTP 处理器共用骨架：依赖注入、Cookie 与验证/重置邮件发送。
package user

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

const refreshCookie = "calhub_rt"

type handler struct {
	a        *app.App
	sessions *auth.Sessions
	audit    *httpx.Audit
}

func newHandler(a *app.App) *handler {
	return &handler{a: a, sessions: &auth.Sessions{DB: a.DB}, audit: &httpx.Audit{DB: a.DB}}
}

// ---- 刷新令牌 Cookie ----

func (h *handler) setRefreshCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    token,
		Path:     "/api/v1/auth",
		MaxAge:   int(h.a.Cfg.Auth.RefreshTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookie(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/api/v1/auth",
		MaxAge: -1, HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) secureCookie() bool {
	return strings.HasPrefix(h.a.Cfg.Server.BaseURL, "https://")
}

func refreshCookieValue(r *http.Request) string {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// ---- 邮件 ----

func (h *handler) sendVerifyEmail(ctx context.Context, u *User) error {
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := auth.InvalidateUserTokens(ctx, h.a.DB, u.ID, "verify_email"); err != nil {
		return err
	}
	if err := auth.SaveToken(ctx, h.a.DB, u.ID, "verify_email", hash, "", auth.VerifyEmailTTL); err != nil {
		return err
	}
	subject, text := mail.RenderVerifyEmail(h.siteName(), h.a.Cfg.Server.BaseURL, token)
	return h.a.Mail.Send(ctx, mail.Message{To: u.Email, Subject: subject, Text: text, Kind: "verify_email"})
}

// SendResetEmail 供 admin 包触发的重置密码邮件（同一模板与令牌规则）。
func SendResetEmail(ctx context.Context, a *app.App, u *User) error {
	h := &handler{a: a, sessions: &auth.Sessions{DB: a.DB}, audit: &httpx.Audit{DB: a.DB}}
	return h.sendResetEmail(ctx, u)
}

func (h *handler) sendResetEmail(ctx context.Context, u *User) error {
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := auth.InvalidateUserTokens(ctx, h.a.DB, u.ID, "reset_password"); err != nil {
		return err
	}
	if err := auth.SaveToken(ctx, h.a.DB, u.ID, "reset_password", hash, "", auth.ResetPasswordTTL); err != nil {
		return err
	}
	subject, text := mail.RenderResetPassword(h.siteName(), h.a.Cfg.Server.BaseURL, token)
	return h.a.Mail.Send(ctx, mail.Message{To: u.Email, Subject: subject, Text: text, Kind: "reset_password"})
}

func (h *handler) siteName() string {
	name := h.a.SiteName(context.Background())
	if name == "" {
		return "CalHub"
	}
	return name
}

// securityAudit 记录安全事件（登录失败、令牌重用等），尽力而为。
func (h *handler) securityAudit(r *http.Request, actorID int64, action, targetType, targetID string, detail map[string]any) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := h.audit.Log(ctx, actorID, action, targetType, targetID, httpx.ClientIP(r), detail); err != nil {
		h.a.Log.Error("audit_write_failed", "action", action, "error", err)
	}
}

// requireVerified 登录前检查系统级邮箱验证要求。
func (h *handler) requireVerified(ctx context.Context, u *User) *httpx.Error {
	required, err := h.a.EffectiveRequireEmailVerification(ctx)
	if err != nil {
		h.a.Log.Error("read_setting_failed", "key", app.SettingRequireEmailVerification, "error", err)
		required = h.a.Cfg.Auth.RequireEmailValidation
	}
	if required && (!u.EmailVerifiedAt.Valid || u.EmailVerifiedAt.String == "") {
		return httpx.E("auth/email_not_verified", "请先完成邮箱验证")
	}
	return nil
}

// mapUserErr DB 错误映射：唯一约束冲突转语义错误码，其余透传。
func mapUserErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed: users.email"):
		return httpx.E("auth/email_taken", "邮箱已被注册")
	case strings.Contains(msg, "UNIQUE constraint failed: users.username"):
		return httpx.E("auth/username_taken", "用户名已被使用")
	}
	return err
}
