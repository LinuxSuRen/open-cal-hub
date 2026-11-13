package booking

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// ExternalFeed 是外部 ICS 源。
type ExternalFeed struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	Color        string `json:"color"`
	LastSyncedAt string `json:"last_synced_at"`
	LastError    string `json:"last_error"`
}

// feedSyncMaxEvents 单次同步写入的忙碌时段上限（防 RRULE 炸弹）。
const feedSyncMaxEvents = 2000

// SyncFeed 拉取远程 ICS、展开 RRULE 至 +120d 并重建 feed_events。
func SyncFeed(ctx context.Context, q cal.DBTX, userID, feedID int64) (events int, lastErr string) {
	var (
		name, url string
	)
	err := q.QueryRowContext(ctx, `SELECT name, url FROM external_feeds WHERE id = ? AND user_id = ?`, feedID, userID).
		Scan(&name, &url)
	if err == sql.ErrNoRows {
		return 0, "feed not found"
	}
	if err != nil {
		return 0, err.Error()
	}
	now := time.Now().UTC()
	windowFrom, windowTo := now.Add(-7*24*time.Hour), now.Add(120*24*time.Hour)

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return recordSyncError(ctx, q, feedID, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return recordSyncError(ctx, q, feedID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return recordSyncError(ctx, q, feedID, fmt.Errorf("http status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return recordSyncError(ctx, q, feedID, err)
	}
	vcal, err := ics.NewDecoder(strings.NewReader(string(body))).Decode()
	if err != nil {
		return recordSyncError(ctx, q, feedID, fmt.Errorf("invalid ics: %v", err))
	}

	type span struct {
		uid, summary string
		start, end   time.Time
	}
	var spans []span
	for _, ev := range vcal.Events() {
		if status, _ := ev.Props.Text(ics.PropStatus); strings.EqualFold(status, "CANCELLED") {
			continue
		}
		transp := "OPAQUE"
		if t, err := ev.Props.Text(ics.PropTransparency); err == nil {
			transp = strings.ToUpper(t)
		}
		if transp == "TRANSPARENT" {
			continue
		}
		uid, _ := ev.Props.Text(ics.PropUID)
		summary, _ := ev.Props.Text(ics.PropSummary)
		fallback := time.UTC
		if p := ev.Props.Get(ics.PropDateTimeStart); p != nil {
			if tzid := p.Params.Get(ics.PropTimezoneID); tzid != "" {
				if loc, err := time.LoadLocation(tzid); err == nil {
					fallback = loc
				}
			}
		}
		start, err := ev.DateTimeStart(fallback)
		if err != nil || start.IsZero() {
			continue
		}
		end, err := ev.DateTimeEnd(time.UTC)
		if err != nil || end.Before(start) || end.Equal(start) {
			end = start.Add(time.Hour)
		}
		rset, rerr := ev.Component.RecurrenceSet(time.UTC)
		if rerr == nil && rset != nil {
			occs := rset.Between(windowFrom, windowTo, true)
			if len(occs) > cal.MaxExpandInstances {
				occs = occs[:cal.MaxExpandInstances]
			}
			dur := end.Sub(start)
			for _, occ := range occs {
				spans = append(spans, span{uid: uid, summary: summary, start: occ.UTC(), end: occ.UTC().Add(dur)})
			}
			continue
		}
		if end.Before(windowFrom) || start.After(windowTo) {
			continue
		}
		spans = append(spans, span{uid: uid, summary: summary, start: start.UTC(), end: end.UTC()})
	}
	if len(spans) > feedSyncMaxEvents {
		spans = spans[:feedSyncMaxEvents]
	}

	del, err := q.ExecContext(ctx, `DELETE FROM feed_events WHERE feed_id = ?`, feedID)
	if err != nil {
		return recordSyncError(ctx, q, feedID, err)
	}
	_ = del
	for _, s := range spans {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO feed_events (feed_id, uid, summary, busy, dtstart, dtend) VALUES (?,?,?,1,?,?)`,
			feedID, s.uid, s.summary, cal.UTCString(s.start), cal.UTCString(s.end)); err != nil {
			return recordSyncError(ctx, q, feedID, err)
		}
	}
	if _, err := q.ExecContext(ctx, `UPDATE external_feeds SET last_synced_at = ?, last_error = '' WHERE id = ?`,
		cal.NowUTCString(), feedID); err != nil {
		return recordSyncError(ctx, q, feedID, err)
	}
	return len(spans), ""
}

func recordSyncError(ctx context.Context, q cal.DBTX, feedID int64, err error) (int, string) {
	msg := err.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = q.ExecContext(ctx, `UPDATE external_feeds SET last_error = ?, last_synced_at = ? WHERE id = ?`,
		msg, cal.NowUTCString(), feedID)
	return 0, msg
}

// --- REST 处理器 ---

func (h *handler) listFeeds(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT id, name, url, color, last_synced_at, last_error FROM external_feeds WHERE user_id = ? ORDER BY id`, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer rows.Close()
	out := []*ExternalFeed{}
	for rows.Next() {
		var f ExternalFeed
		var synced, lastErr sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.Color, &synced, &lastErr); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		if synced.Valid {
			f.LastSyncedAt = synced.String
		}
		f.LastError = lastErr.String
		out = append(out, &f)
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) createFeed(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Color string `json:"color"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if req.Name == "" || !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"url": "名称与有效的 http(s) 地址为必填"}))
		return
	}
	if req.Color == "" {
		req.Color = "#8A8F98"
	}
	res, err := h.a.DB.ExecContext(r.Context(),
		`INSERT INTO external_feeds (user_id, name, url, color, created_at) VALUES (?,?,?,?,?)`,
		u.ID, req.Name, req.URL, req.Color, cal.NowUTCString())
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	id, _ := res.LastInsertId()
	httpx.WriteData(w, http.StatusCreated, &ExternalFeed{ID: id, Name: req.Name, URL: req.URL, Color: req.Color})
}

func (h *handler) deleteFeed(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	res, err := h.a.DB.ExecContext(r.Context(), `DELETE FROM external_feeds WHERE id = ? AND user_id = ?`, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, h.a.Log, httpx.E("common/not_found", "外部日历源不存在"))
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}

func (h *handler) syncFeed(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var owner int64
	err := h.a.DB.QueryRowContext(r.Context(), `SELECT user_id FROM external_feeds WHERE id = ?`, id).Scan(&owner)
	if err == sql.ErrNoRows || (err == nil && owner != u.ID) {
		httpx.WriteError(w, h.a.Log, httpx.E("common/not_found", "外部日历源不存在"))
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	// 同步在后台执行完毕后再返回统计（同步是幂等重建）
	events, lastErr := SyncFeed(r.Context(), h.a.DB, u.ID, id)
	httpx.WriteData(w, http.StatusOK, map[string]any{"events": events, "last_error": lastErr})
}
