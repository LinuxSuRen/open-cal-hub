// POST /api/v1/auth/register：注册（含首个用户 admin、邀请码模式、验证邮件）。
package user

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

type registerReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	Locale      string `json:"locale"`
	InviteCode  string `json:"invite_code"`
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	req.Email = NormalizeEmail(req.Email)
	if e := ValidateEmail(req.Email); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	if e := ValidatePasswordStrength(req.Password); e != nil {
		httpx.WriteError(w, h.a.Log, e)
		return
	}
	if req.Username != "" {
		if e := ValidateUsername(req.Username); e != nil {
			httpx.WriteError(w, h.a.Log, e)
			return
		}
	}
	timezone := "UTC"
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
			return
		}
		timezone = req.Timezone
	}
	locale := req.Locale
	if locale == "" {
		locale = "zh-CN"
	}
	if len(locale) > 16 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"locale": "语言代码无效"}))
		return
	}

	// 邀请码模式（settings.registration_mode=invite）。
	mode, err := h.a.EffectiveRegistrationMode(r.Context())
	if err != nil {
		h.a.Log.Error("read_setting_failed", "key", "registration_mode", "error", err)
		mode = "open"
	}
	inviteValid := false
	if mode == "invite" {
		code := strings.TrimSpace(req.InviteCode)
		if code == "" {
			httpx.WriteError(w, h.a.Log, httpx.E("auth/invite_required", "当前仅支持邀请注册"))
			return
		}
		ok, err := h.checkInviteCode(r.Context(), code)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		if !ok {
			httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_invite", "邀请码无效或已使用"))
			return
		}
		inviteValid = true
	}

	// 用户名：未提供则由邮箱前缀生成。
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		username, err = GenerateUsername(r.Context(), h.a.DB, req.Email)
		if err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
	} else if taken, err := usernameTaken(r.Context(), h.a.DB, username); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	} else if taken {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/username_taken", "用户名已被使用"))
		return
	}
	if _, err := UserByEmail(r.Context(), h.a.DB, req.Email); err == nil {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/email_taken", "邮箱已被注册"))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}

	role := "user"
	if h.a.Cfg.Auth.FirstUserAdmin {
		if n, err := CountUsers(r.Context(), h.a.DB); err == nil && n == 0 {
			role = "admin"
		} else if err != nil {
			h.a.Log.Error("count_users_failed", "error", err)
		}
	}

	hash, err := auth.HashPassword(req.Password)
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
	u := &User{
		Email: req.Email, Username: username, PasswordHash: hash,
		DisplayName: displayName, Timezone: timezone, Locale: locale,
		Role: role, Status: "active",
	}
	if err := CreateUser(r.Context(), h.a.DB, u); err != nil {
		httpx.WriteError(w, h.a.Log, mapUserErr(err))
		return
	}

	if inviteValid {
		if err := h.consumeInviteCode(r.Context(), req.InviteCode, u.ID); err != nil {
			// 邀请码消费失败不回滚用户（避免唯一约束竞态导致注册失败），记日志待查。
			h.a.Log.Error("consume_invite_failed", "code_len", len(req.InviteCode), "user_id", u.ID, "error", err)
		}
	}

	// 发送验证邮件（发送失败不影响注册结果，用户可重发）。
	if err := h.sendVerifyEmail(r.Context(), u); err != nil {
		h.a.Log.Error("send_verify_email_failed", "user_id", u.ID, "error", err)
	}

	// 注册后钩子（booking 包创建默认可用时间集等）。
	if h.a.OnUserRegistered != nil {
		if err := h.a.OnUserRegistered(r.Context(), u.ID, u.Timezone); err != nil {
			h.a.Log.Error("on_user_registered_failed", "user_id", u.ID, "error", err)
		}
	}

	h.securityAudit(r, u.ID, "user.register", "user", strconv.FormatInt(u.ID, 10), map[string]any{
		"username": u.Username, "email": u.Email, "role": u.Role, "mode": mode,
	})

	verified, err := h.a.EffectiveRequireEmailVerification(r.Context())
	if err != nil {
		verified = h.a.Cfg.Auth.RequireEmailValidation
	}
	httpx.WriteData(w, http.StatusCreated, map[string]any{
		"user":                  u.JSON(),
		"verification_required": verified,
	})
}

func (h *handler) checkInviteCode(ctx context.Context, code string) (bool, error) {
	var usedAt, expires sql.NullString
	err := h.a.DB.QueryRowContext(ctx, `SELECT used_at, expires_at FROM invite_codes WHERE code=?`, code).Scan(&usedAt, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if usedAt.Valid {
		return false, nil
	}
	if expires.Valid && expires.String != "" && expires.String < nowUTC() {
		return false, nil
	}
	return true, nil
}

func (h *handler) consumeInviteCode(ctx context.Context, code string, userID int64) error {
	res, err := h.a.DB.ExecContext(ctx,
		`UPDATE invite_codes SET used_by=?, used_at=? WHERE code=? AND used_at IS NULL`,
		userID, nowUTC(), strings.TrimSpace(code))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("invite code already consumed")
	}
	return nil
}
