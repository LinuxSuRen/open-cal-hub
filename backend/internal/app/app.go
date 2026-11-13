// Package app 装配共享依赖，供各功能包注册路由使用。
package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

type App struct {
	Cfg    *config.Config
	DB     *sql.DB
	Log    *slog.Logger
	Mail   mail.Sender
	Signer *auth.Signer
	// LoadUser 由 user 包在注册时注入（避免循环依赖）。
	LoadUser auth.UserLoader
	// BasicAuth 由 user 包注入（dav 包 Basic 认证：用户名/邮箱 + 登录密码或应用密码）。
	BasicAuth func(ctx context.Context, login, password string) (*auth.User, bool)
	// Mux 由 cmd/calhub/routes.go 装配（各功能包路由挂载点，可再包一层中间件），server.New 消费。
	Mux http.Handler
	// Metrics HTTP 指标聚合器；server.New 惰性创建，功能包可向其扩展观测。
	Metrics *httpx.Metrics
	// OnUserRegistered 注册成功后的钩子（booking 包用来创建默认可用时间集）；
	// 钩子失败只记日志，不影响注册结果。
	OnUserRegistered func(ctx context.Context, userID int64, timezone string) error
}

// User 实现按 ID 加载活跃用户（user 包注册真实实现前的基础占位，返回 nil 表示不存在）。
func (a *App) User(ctx context.Context, id int64) (*auth.User, error) {
	if a.LoadUser != nil {
		return a.LoadUser(ctx, id)
	}
	return nil, sql.ErrNoRows
}

// ---- settings 键值（契约 §6 白名单）----

// 设置键白名单；新增键需先在契约登记。
const (
	SettingSiteName                = "site_name"
	SettingRegistrationMode        = "registration_mode"
	SettingRequireEmailVerification = "require_email_verification"
	SettingBookingHorizonDays      = "booking_horizon_days"
)

// ErrSettingInvalid 设置键不在白名单或值非法。
var ErrSettingInvalid = errors.New("invalid setting")

// Setting 读取一个设置；无记录时返回 fallback。
func (a *App) Setting(ctx context.Context, key, fallback string) (string, error) {
	var v string
	err := a.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting 写入设置（upsert）。
func (a *App) SetSetting(ctx context.Context, key, value string) error {
	_, err := a.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// EffectiveRequireEmailVerification：settings 有值用之，否则用配置默认。
func (a *App) EffectiveRequireEmailVerification(ctx context.Context) (bool, error) {
	v, err := a.Setting(ctx, SettingRequireEmailVerification, strconv.FormatBool(a.Cfg.Auth.RequireEmailValidation))
	if err != nil {
		return a.Cfg.Auth.RequireEmailValidation, err
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return a.Cfg.Auth.RequireEmailValidation, nil
	}
	return b, nil
}

// EffectiveRegistrationMode：open | invite。
func (a *App) EffectiveRegistrationMode(ctx context.Context) (string, error) {
	v, err := a.Setting(ctx, SettingRegistrationMode, "open")
	if err != nil || (v != "open" && v != "invite") {
		return "open", err
	}
	return v, nil
}

// SiteName 站点名（默认 CalHub）。
func (a *App) SiteName(ctx context.Context) string {
	v, err := a.Setting(ctx, SettingSiteName, "CalHub")
	if err != nil || v == "" {
		return "CalHub"
	}
	return v
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
