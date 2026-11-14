// user 包路由注册：/api/v1/auth/*、/api/v1/me/*、/api/v1/meta，并回填 app 装配点。
package user

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// Register 挂载路由并注入 LoadUser / BasicAuth（避免包循环依赖）。
func Register(mux *http.ServeMux, a *app.App) {
	h := newHandler(a)
	a.LoadUser = loadActiveUser(a)
	a.BasicAuth = func(ctx context.Context, login, password string) (*auth.User, bool) {
		u, ok := AuthenticateBasic(ctx, a.DB, login, password)
		if !ok || u == nil {
			return nil, false
		}
		return &auth.User{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role}, true
	}

	// 限流：登录 10/min/IP；注册/忘记密码/重发验证 5/h/IP（契约 §0/架构 §5）。
	// 环境变量可放宽（e2e/联调多用户场景）：CALHUB_RATELIMIT_LOGIN_PER_MIN、
	// CALHUB_RATELIMIT_REGISTER_PER_HOUR；未设置时保持线上默认。
	limLogin := httpx.NewRateLimiter(rateLimitEvents("CALHUB_RATELIMIT_LOGIN_PER_MIN", 10), time.Minute)
	limRegister := httpx.NewRateLimiter(rateLimitEvents("CALHUB_RATELIMIT_REGISTER_PER_HOUR", 5), time.Hour)

	authMW := auth.Middleware(a.Signer, a.LoadUser)
	hd := func(f http.HandlerFunc) http.Handler { return f }

	// 公开：meta
	mux.HandleFunc("GET /api/v1/meta", h.meta)

	// 公开：认证流程（带限流）
	mux.Handle("POST /api/v1/auth/register", limRegister.Middleware("register", hd(h.register)))
	mux.Handle("POST /api/v1/auth/login", limLogin.Middleware("login", hd(h.login)))
	mux.Handle("POST /api/v1/auth/refresh", hd(h.refresh)) // 凭 Cookie 令牌本身即凭证，无需 IP 限流
	mux.Handle("POST /api/v1/auth/logout", hd(h.logout))
	mux.Handle("POST /api/v1/auth/verify-email", hd(h.verifyEmail))
	mux.Handle("POST /api/v1/auth/resend-verification", limRegister.Middleware("resend", hd(h.resendVerification)))
	mux.Handle("POST /api/v1/auth/forgot-password", limRegister.Middleware("forgot", hd(h.forgotPassword)))
	mux.Handle("POST /api/v1/auth/reset-password", limRegister.Middleware("reset", hd(h.resetPassword)))

	// OAuth 登录（provider 未配置时端点返回 404）
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/authorize", h.oauthAuthorize)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", h.oauthCallback)

	// OAuth 绑定管理（登录态）
	mux.Handle("GET /api/v1/me/oauth", authMW(hd(h.listOAuthIdentities)))
	mux.Handle("DELETE /api/v1/me/oauth/{provider}", authMW(hd(h.unbindOAuth)))

	// 登录态：会话管理
	mux.Handle("GET /api/v1/auth/sessions", authMW(hd(h.listSessions)))
	mux.Handle("DELETE /api/v1/auth/sessions/{id}", authMW(hd(h.revokeSession)))

	// 登录态：账号
	mux.Handle("GET /api/v1/me", authMW(hd(h.meGet)))
	mux.Handle("PATCH /api/v1/me", authMW(hd(h.mePatch)))
	mux.Handle("POST /api/v1/me/password", authMW(hd(h.changePassword)))
	mux.Handle("GET /api/v1/me/app-passwords", authMW(hd(h.listAppPasswords)))
	mux.Handle("POST /api/v1/me/app-passwords", authMW(hd(h.createAppPassword)))
	mux.Handle("DELETE /api/v1/me/app-passwords/{id}", authMW(hd(h.deleteAppPassword)))

	// /api 下未匹配路径的 JSON 404（防 mux 默认文本 404 破坏包络约定）。
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, a.Log, httpx.NotFound())
	})
}

// loadActiveUser 认证中间件的用户加载：仅 active 用户可通过。
func loadActiveUser(a *app.App) auth.UserLoader {
	return func(ctx context.Context, id int64) (*auth.User, error) {
		u, err := UserByID(ctx, a.DB, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		if u.Status != "active" {
			return nil, sql.ErrNoRows // 停用/删除等同未登录
		}
		return &auth.User{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role}, nil
	}
}

// rateLimitEvents 读限流事件数的环境覆盖；非法或缺省返回 def。
func rateLimitEvents(envKey string, def int) int {
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
