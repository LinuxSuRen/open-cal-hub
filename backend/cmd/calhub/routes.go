package main

import (
	"net/http"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
)

// registerRoutes 挂载各功能包路由。
// 骨架阶段为空实现；auth/user/admin/cal/booking/dav 包就绪后在此接入：
//
//	user.Register(mux, a)   // /api/v1/auth/*、/api/v1/me/*，并回填 a.LoadUser
//	admin.Register(mux, a)  // /api/v1/admin/*
//	cal.Register(mux, a)    // /api/v1/calendars、/api/v1/events、/api/v1/pub/share/*
//	booking.Register(mux, a) // /api/v1/event-types、/api/v1/availability-sets、/api/v1/pub/u/*、/api/v1/pub/b/*
//	dav.Register(mux, a)    // /dav/
func registerRoutes(a *app.App) {
	_ = http.NewServeMux() // 占位，避免未使用导入
	_ = a
}
