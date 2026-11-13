// 管理端全平台预约（只读 join bookings）。数据由 booking 包写入；
// 本端点为读侧实现，bookings 表结构在 0001 已定义，联调查验可先返回空列表。
// TODO(backend-cal): 若预约列表需要补充字段（如日历名），在此扩展 SELECT 列。
package admin

import (
	"net/http"
	"strings"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

func (h *handler) listBookings(w http.ResponseWriter, r *http.Request) {
	page, pageSize := httpx.PageParams(r)
	q := r.URL.Query()
	status, search := q.Get("status"), strings.TrimSpace(q.Get("query"))
	if status != "" && status != "confirmed" && status != "cancelled" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"status": "状态筛选无效"}))
		return
	}

	where := " WHERE 1=1"
	args := []any{}
	if status != "" {
		where += " AND b.status=?"
		args = append(args, status)
	}
	if search != "" {
		like := "%" + search + "%"
		where += " AND (b.guest_name LIKE ? OR b.guest_email LIKE ? OR u.username LIKE ? OR u.display_name LIKE ?)"
		args = append(args, like, like, like, like)
	}

	var total int64
	if err := h.a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM bookings b JOIN users u ON u.id=b.host_id`+where, args...).Scan(&total); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := h.a.DB.QueryContext(r.Context(), `
		SELECT b.id, b.event_type_id, et.name, et.slug, b.host_id, u.username, u.display_name,
		       b.guest_name, b.guest_email, b.guest_note, b.start_utc, b.end_utc, b.status, b.created_at
		FROM bookings b
		JOIN event_types et ON et.id=b.event_type_id
		JOIN users u ON u.id=b.host_id`+where+`
		ORDER BY b.id DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, etID, hostID int64
		var etName, etSlug, hostUsername, hostDisplay, guestName, guestEmail, guestNote, start, end, statusV, createdAt string
		if err := rows.Scan(&id, &etID, &etName, &etSlug, &hostID, &hostUsername, &hostDisplay,
			&guestName, &guestEmail, &guestNote, &start, &end, &statusV, &createdAt); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		out = append(out, map[string]any{
			"id": id,
			"event_type": map[string]any{"id": etID, "name": etName, "slug": etSlug},
			"host":       map[string]any{"id": hostID, "username": hostUsername, "display_name": hostDisplay},
			"guest_name": guestName, "guest_email": guestEmail, "guest_note": guestNote,
			"start": start, "end": end, "status": statusV, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteList(w, page, pageSize, int(total), out)
}
