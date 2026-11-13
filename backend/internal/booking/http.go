// booking 包 HTTP 层：handler 类型、助手与路由注册。
// 注意（交接）：本文件为收尾时的最小接线——只挂已实现的端点。
// bookings.go（下单事务/取消/改期/GET /me/bookings）与 pub.go（/pub/u/*、/pub/b/*）
// 尚未实现，见仓库 issue 清单。
package booking

import (
	"net/http"
	"strconv"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

type handler struct{ a *app.App }

// user 提取上下文用户（认证注入由上层 OptionalMiddleware 完成），未认证时写 401。
func (h *handler) user(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	u, ok := auth.From(r.Context())
	if !ok {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return nil, false
	}
	return u, true
}

// idParam 解析路径参数 {id}，非法时写 400 并返回 false。
func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, nil, httpx.Validation(map[string]string{"id": "id 无效"}))
		return 0, false
	}
	return id, true
}

// Register 挂载预约域已实现的路由（认证注入由上层中间件保证，处理器内兜底）。
func Register(mux *http.ServeMux, a *app.App) {
	h := &handler{a: a}

	mux.HandleFunc("GET /api/v1/availability-sets", h.listAvailability)
	mux.HandleFunc("POST /api/v1/availability-sets", h.createAvailability)
	mux.HandleFunc("PATCH /api/v1/availability-sets/{id}", h.patchAvailability)
	mux.HandleFunc("DELETE /api/v1/availability-sets/{id}", h.deleteAvailability)

	mux.HandleFunc("GET /api/v1/event-types", h.listEventTypes)
	mux.HandleFunc("POST /api/v1/event-types", h.createEventType)
	mux.HandleFunc("PATCH /api/v1/event-types/{id}", h.patchEventType)
	mux.HandleFunc("DELETE /api/v1/event-types/{id}", h.deleteEventType)

	mux.HandleFunc("GET /api/v1/external-feeds", h.listFeeds)
	mux.HandleFunc("POST /api/v1/external-feeds", h.createFeed)
	mux.HandleFunc("DELETE /api/v1/external-feeds/{id}", h.deleteFeed)
	mux.HandleFunc("POST /api/v1/external-feeds/{id}/sync", h.syncFeed)

	// TODO(接手): bookings.go / pub.go 完成后补挂：
	//   GET/POST /api/v1/me/bookings、POST /api/v1/bookings/{id}/cancel、
	//   /api/v1/pub/u/{username}[...]、/api/v1/pub/b/{token}[...]
}
