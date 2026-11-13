// user 包路由注册：/api/v1/auth/*、/api/v1/me/*、/api/v1/meta，并回填 app 装配点。
package user

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
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
	limLogin := httpx.NewRateLimiter(10, time.Minute)
	limRegister := httpx.NewRateLimiter(5, time.Hour)

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
