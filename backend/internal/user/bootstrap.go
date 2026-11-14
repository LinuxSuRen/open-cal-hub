// 初始管理员引导：空库首启时按配置创建 admin 账号（issue #13）。
// 适合脚本化部署（docker run / compose / systemd）——不必依赖"首注册用户
// 自动成为 admin"的交互路径；账号创建后建议立即改密并移除环境变量。
package user

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
)

// BootstrapAdmin 在 users 表为空时按配置创建初始管理员。
// 返回是否创建；库中已有用户时幂等跳过（不改动任何数据）。
func BootstrapAdmin(ctx context.Context, a *app.App, email, password string) (bool, error) {
	email = NormalizeEmail(email)
	if e := ValidateEmail(email); e != nil {
		return false, errors.New("bootstrap admin: " + e.Message)
	}
	if e := ValidatePasswordStrength(password); e != nil {
		return false, errors.New("bootstrap admin: " + e.Message)
	}
	n, err := CountUsers(ctx, a.DB)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if _, err := UserByEmail(ctx, a.DB, email); err == nil {
		return false, nil // 理论不可达（CountUsers=0），防御性返回
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	username, err := GenerateUsername(ctx, a.DB, email)
	if err != nil {
		return false, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	u := &User{
		Email: email, Username: username, PasswordHash: hash,
		DisplayName: username, Timezone: "UTC", Locale: "zh-CN",
		Role: "admin", Status: "active",
	}
	if err := CreateUser(ctx, a.DB, u); err != nil {
		return false, err
	}
	// CreateUser 不写 email_verified_at；运维显式配置的账号视为已验证
	if _, err := a.DB.ExecContext(ctx, `UPDATE users SET email_verified_at=? WHERE id=?`,
		nowUTC(), u.ID); err != nil {
		return false, err
	}
	if a.OnUserRegistered != nil {
		if err := a.OnUserRegistered(ctx, u.ID, u.Timezone); err != nil {
			a.Log.Error("bootstrap_on_user_registered_failed", "user_id", u.ID, "error", err)
		}
	}
	return true, nil
}

// BootstrapAdminFromConfig 从配置读取引导参数（其一为空视为未启用）。
// 只配置其一返回错误（避免半配置状态静默失效）。
func BootstrapAdminFromConfig(ctx context.Context, a *app.App, email, password string) (bool, error) {
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	switch {
	case email == "" && password == "":
		return false, nil
	case email == "" || password == "":
		return false, errors.New("bootstrap admin 需要同时配置 bootstrap_admin_email 与 bootstrap_admin_password")
	default:
		return BootstrapAdmin(ctx, a, email, password)
	}
}
