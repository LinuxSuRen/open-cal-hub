package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/admin"
	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/booking"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/dav"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/user"
)

// registerRoutes 挂载各功能包路由，并套上公共中间件：
// 1) 尽力认证注入（公开路由放行、受保护路由由处理器兜底 401）；
// 2) 公开端点限流（/api/v1/pub/ 前缀 120/min/IP，其中预约提交 10/h/IP）。
func registerRoutes(a *app.App) {
	mux := http.NewServeMux()

	user.Register(mux, a)   // /api/v1/auth/*、/api/v1/me/*、/api/v1/meta；回填 a.LoadUser 与 a.BasicAuth
	admin.Register(mux, a)  // /api/v1/admin/*
	cal.Register(mux, a)    // /api/v1/calendars*、/api/v1/events/*、/api/v1/pub/share/*
	booking.Register(mux, a) // /api/v1/availability-sets*、/api/v1/event-types*、/api/v1/external-feeds*
	dav.Register(mux, a)    // /dav/*（Basic 认证用 a.BasicAuth）

	// 注册钩子：默认日历 + 工作时间可用集（幂等）
	prev := a.OnUserRegistered
	a.OnUserRegistered = func(ctx context.Context, userID int64, timezone string) error {
		if _, err := cal.EnsureDefaultCalendar(ctx, a.DB, userID, timezone); err != nil {
			return err
		}
		if _, err := booking.EnsureDefaultAvailability(ctx, a.DB, userID, timezone); err != nil {
			return err
		}
		if prev != nil {
			return prev(ctx, userID, timezone)
		}
		return nil
	}

	a.Mux = pubGuard(a)(auth.OptionalMiddleware(a.Signer, a.User)(mux))
}

// pubGuard 公开端点限流：/api/v1/pub/ 120/min/IP；预约提交 10/h/IP。
func pubGuard(a *app.App) func(http.Handler) http.Handler {
	pubLimiter := httpx.NewRateLimiter(120, time.Minute)
	bookLimiter := httpx.NewRateLimiter(10, time.Hour)
	ipOf := func(r *http.Request) string {
		if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
			return r.RemoteAddr[:i]
		}
		return r.RemoteAddr
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			if strings.HasPrefix(p, "/api/v1/pub/") {
				if strings.HasSuffix(p, "/bookings") && r.Method == http.MethodPost {
					if !bookLimiter.Allow("pubbook:" + ipOf(r)) {
						httpx.WriteError(w, a.Log, &httpx.Error{Code: "common/rate_limited", Message: "请求过于频繁，请稍后再试"})
						return
					}
				} else if !pubLimiter.Allow("pub:" + ipOf(r)) {
					httpx.WriteError(w, a.Log, &httpx.Error{Code: "common/rate_limited", Message: "请求过于频繁，请稍后再试"})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
