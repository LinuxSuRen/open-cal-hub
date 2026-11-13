// Package server 组装 HTTP 路由与静态资源。
package server

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/web"
)

// New 构建 HTTP 服务。各功能包（auth/user/admin/cal/booking/dav）由
// cmd/calhub/routes.go 注册到 a.Mux，此处只负责公共骨架与中间件。
func New(a *app.App) http.Handler {
	if a.Mux == nil {
		a.Mux = http.NewServeMux()
	}
	if a.Metrics == nil {
		a.Metrics = httpx.NewMetrics()
	}

	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.DB.PingContext(r.Context()); err != nil {
			httpx.WriteError(w, a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Prometheus 指标（请求计数/时延直方图，见 internal/httpx/metrics.go）
	if a.Cfg.Metrics.Enabled {
		mux.Handle("GET /metrics", a.Metrics.Handler())
	}

	// /.well-known/caldav -> /dav/（CalDAV 实际路由由 dav 包注册在 a.Mux）
	mux.Handle("/.well-known/caldav", http.RedirectHandler("/dav/", http.StatusMovedPermanently))

	// API 与 CalDAV：功能包路由（a.Mux）
	mux.Handle("/api/", a.Mux)
	mux.Handle("/dav/", a.Mux)

	// 前端静态资源（embed）；SPA fallback 到 index.html
	static(mux, web.Dist())

	var h http.Handler = mux
	h = a.Metrics.Middleware(h) // /metrics 自身不计入（见 Middleware 内过滤）
	h = httpx.Timeout(60*time.Second, h)
	h = httpx.SecurityHeaders(h)
	h = httpx.RequestID(a.Log, h)
	h = httpx.Recover(a.Log, h)
	return h
}

// static 挂载前端资源：静态文件优先，其余非 API/DAV 路径回 index.html。
func static(mux *http.ServeMux, dist fs.FS) {
	fileServer := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// SPA fallback：非 /api、/dav、/.well-known 开头且非静态资源路径
			if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/dav/") && !strings.Contains(p, ".") {
				r2 := new(http.Request)
				*r2 = *r
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
