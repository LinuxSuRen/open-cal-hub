// 管理员新增用户（issue #24）：POST /api/v1/admin/users。
// 密码两种交付：请求体显式提供（线下告知），或缺省生成随机密码并发送
// 重置密码邮件（用户自行设置，无需明文二次传递）。账号视为已验证。
package admin

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

// createUser POST /api/v1/admin/users
func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Timezone    string `json:"timezone"`
		Role        string `json:"role"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	req.Email = user.NormalizeEmail(req.Email)
	if e := user.ValidateEmail(req.Email); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	role := "user"
	if req.Role != "" {
		if req.Role != "user" && req.Role != "admin" {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"role": "角色只能为 user 或 admin"}))
			return
		}
		role = req.Role
	}
	timezone := "UTC"
	if req.Timezone != "" {
		if !validTimezone(req.Timezone) {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
			return
		}
		timezone = req.Timezone
	}

	// 用户名：提供则校验，缺省由邮箱前缀生成
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username != "" {
		if _, err := user.UserByLogin(r.Context(), h.a.DB, username); err == nil {
			httpx.WriteError(w, h.a.Log, httpx.E("auth/username_taken", "用户名已被使用"))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
	} else {
		var err error
		username, err = user.GenerateUsername(r.Context(), h.a.DB, req.Email)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
	}
	if _, err := user.UserByEmail(r.Context(), h.a.DB, req.Email); err == nil {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/email_taken", "邮箱已被注册"))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}

	// 密码：显式提供则校验强度；缺省生成随机并发重置邮件
	resetEmailSent := false
	password := req.Password
	if password == "" {
		password = randomAdminPassword()
		resetEmailSent = true
	} else if e := user.ValidatePasswordStrength(password); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 100 {
		displayName = displayName[:100]
	}
	u := &user.User{
		Email: req.Email, Username: username, PasswordHash: hash,
		DisplayName: displayName, Timezone: timezone, Locale: "zh-CN",
		Role: role, Status: "active",
	}
	if err := user.CreateUser(r.Context(), h.a.DB, u); err != nil {
		httpx.WriteError(w, h.a.Log, mapUserCreateErr(err))
		return
	}
	// 管理员添加即视为已验证
	if _, err := h.a.DB.ExecContext(r.Context(),
		`UPDATE users SET email_verified_at=? WHERE id=?`, user.NowUTC(), u.ID); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if h.a.OnUserRegistered != nil {
		if err := h.a.OnUserRegistered(r.Context(), u.ID, u.Timezone); err != nil {
			h.a.Log.Error("admin_create_on_user_registered_failed", "user_id", u.ID, "error", err)
		}
	}
	if resetEmailSent {
		if err := user.SendResetEmail(r.Context(), h.a, u); err != nil {
			// 邮件失败不回滚用户（管理员可重发），但要可见
			h.a.Log.Error("admin_create_reset_email_failed", "user_id", u.ID, "error", err)
		}
	}
	h.adminAudit(r, "user.create", "user", itoaUser(u.ID), map[string]any{
		"username": u.Username, "email": u.Email, "role": u.Role,
		"password_delivery": map[bool]string{true: "reset_email", false: "explicit"}[resetEmailSent],
	})
	full, _ := user.UserByID(r.Context(), h.a.DB, u.ID)
	resp := map[string]any{"user": full.JSON(), "reset_email_sent": resetEmailSent}
	httpx.WriteData(w, http.StatusCreated, resp)
}

func mapUserCreateErr(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed: users.email"):
		return httpx.E("auth/email_taken", "邮箱已被注册")
	case strings.Contains(msg, "UNIQUE constraint failed: users.username"):
		return httpx.E("auth/username_taken", "用户名已被使用")
	}
	return err
}

// randomAdminPassword 生成 16 位强密码（大小写字母+数字）。
func randomAdminPassword() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func validTimezone(tz string) bool {
	_, err := time.LoadLocation(tz)
	return err == nil
}

func itoaUser(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
