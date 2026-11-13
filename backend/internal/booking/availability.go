package booking

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

var slugRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// EnsureDefaultAvailability 注册时自动创建“工作时间”默认集（周一至周五 09:00-17:00），幂等。
func EnsureDefaultAvailability(ctx context.Context, q cal.DBTX, userID int64, timezone string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx,
		`SELECT id FROM availability_sets WHERE user_id = ? ORDER BY is_default DESC, id LIMIT 1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if !validTimezone(timezone) {
		timezone = "UTC"
	}
	now := cal.NowUTCString()
	res, err := q.ExecContext(ctx,
		`INSERT INTO availability_sets (user_id, name, timezone, is_default, created_at, updated_at)
		 VALUES (?,?,?,?,?,?)`, userID, "工作时间", timezone, 1, now, now)
	if err != nil {
		return 0, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for wd := 1; wd <= 5; wd++ {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO availability_rules (availability_id, weekday, start_minute, end_minute) VALUES (?,?,?,?)`,
			id, wd, 9*60, 17*60); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// availabilityDTO 校验输入并生成规则列表。
type availabilityInput struct {
	Name      *string     `json:"name"`
	Timezone  *string     `json:"timezone"`
	IsDefault *bool       `json:"is_default"`
	Rules     *[]Rule     `json:"rules"`
	Dates     *[]DateRule `json:"dates"`
}

func validateRules(rules []Rule) error {
	for _, r := range rules {
		if r.Weekday < 0 || r.Weekday > 6 {
			return httpx.Validation(map[string]string{"rules": "星期必须在 0（周日）到 6 之间"})
		}
		if r.StartMinute < 0 || r.StartMinute > 1439 || r.EndMinute < 1 || r.EndMinute > 1440 {
			return httpx.Validation(map[string]string{"rules": "分钟数必须在 0..1440 之间"})
		}
		if r.StartMinute >= r.EndMinute {
			return httpx.Validation(map[string]string{"rules": "开始分钟必须早于结束分钟"})
		}
	}
	return nil
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validateDates(dates []DateRule) error {
	for _, d := range dates {
		if !dateRe.MatchString(d.Date) {
			return httpx.Validation(map[string]string{"dates": "日期格式必须为 YYYY-MM-DD"})
		}
		if _, err := time.Parse("2006-01-02", d.Date); err != nil {
			return httpx.Validation(map[string]string{"dates": "日期无效"})
		}
		if d.Kind != "available" && d.Kind != "unavailable" {
			return httpx.Validation(map[string]string{"dates": "类型必须为 available 或 unavailable"})
		}
		if d.StartMinute < 0 || d.StartMinute > 1439 || d.EndMinute < 1 || d.EndMinute > 1440 || d.StartMinute >= d.EndMinute {
			return httpx.Validation(map[string]string{"dates": "分钟区间无效"})
		}
	}
	return nil
}

// createAvailability POST /availability-sets
func (h *handler) createAvailability(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	var in availabilityInput
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if in.Name == nil || *in.Name == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"name": "名称不能为空"}))
		return
	}
	tz := "UTC"
	if in.Timezone != nil && *in.Timezone != "" {
		tz = *in.Timezone
	}
	if !validTimezone(tz) {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
		return
	}
	rules := []Rule{}
	if in.Rules != nil {
		rules = *in.Rules
	}
	dates := []DateRule{}
	if in.Dates != nil {
		dates = *in.Dates
	}
	if err := validateRules(rules); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := validateDates(dates); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	isDefault := false
	if in.IsDefault != nil {
		isDefault = *in.IsDefault
	}
	ctx := r.Context()
	now := cal.NowUTCString()
	var tx *sql.Tx
	tx, err := h.a.DB.Begin()
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer tx.Rollback()
	if isDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE availability_sets SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, u.ID); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO availability_sets (user_id, name, timezone, is_default, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		u.ID, *in.Name, tz, isDefault, now, now)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := SaveRules(ctx, tx, id, rules, dates); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	set, err := LoadAvailability(ctx, h.a.DB, id, u.ID)
	if err != nil || set == nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusCreated, set)
}

// listAvailability GET /availability-sets
func (h *handler) listAvailability(w http.ResponseWriter, r *http.Request) {
	u, ok := h.user(w, r)
	if !ok {
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT id FROM availability_sets WHERE user_id = ? ORDER BY is_default DESC, id`, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		ids = append(ids, id)
	}
	out := make([]*AvailabilitySet, 0, len(ids))
	for _, id := range ids {
		set, err := LoadAvailability(r.Context(), h.a.DB, id, u.ID)
		if err != nil || set == nil {
			continue
		}
		out = append(out, set)
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) loadOwnedSet(w http.ResponseWriter, r *http.Request) (*AvailabilitySet, bool) {
	u, ok := h.user(w, r)
	if !ok {
		return nil, false
	}
	id, ok := idParam(w, r)
	if !ok {
		return nil, false
	}
	set, err := LoadAvailability(r.Context(), h.a.DB, id, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return nil, false
	}
	if set == nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/not_found", "可用时间集不存在"))
		return nil, false
	}
	return set, true
}

// patchAvailability PATCH /availability-sets/{id}（rules/dates 整体替换）
func (h *handler) patchAvailability(w http.ResponseWriter, r *http.Request) {
	set, ok := h.loadOwnedSet(w, r)
	if !ok {
		return
	}
	var in availabilityInput
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if in.Name != nil && *in.Name != "" {
		set.Name = *in.Name
	}
	if in.Timezone != nil && *in.Timezone != "" {
		if !validTimezone(*in.Timezone) {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"timezone": "时区无效"}))
			return
		}
		set.Timezone = *in.Timezone
	}
	rules, dates := set.Rules, set.Dates
	if in.Rules != nil {
		rules = *in.Rules
	}
	if in.Dates != nil {
		dates = *in.Dates
	}
	if err := validateRules(rules); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := validateDates(dates); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	ctx := r.Context()
	tx, err := h.a.DB.Begin()
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	defer tx.Rollback()
	now := cal.NowUTCString()
	if in.IsDefault != nil && *in.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE availability_sets SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, set.OwnerID); err != nil {
			httpx.WriteError(w, h.a.Log, err)
			return
		}
		set.IsDefault = true
	}
	_, err = tx.ExecContext(ctx, `UPDATE availability_sets SET name=?, timezone=?, is_default=?, updated_at=? WHERE id=?`,
		set.Name, set.Timezone, set.IsDefault, now, set.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := SaveRules(ctx, tx, set.ID, rules, dates); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	updated, err := LoadAvailabilityByID(r.Context(), h.a.DB, set.ID)
	if err != nil || updated == nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusOK, updated)
}

// deleteAvailability DELETE /availability-sets/{id}（被预约类型引用时拒绝）
func (h *handler) deleteAvailability(w http.ResponseWriter, r *http.Request) {
	set, ok := h.loadOwnedSet(w, r)
	if !ok {
		return
	}
	var refs int
	if err := h.a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM event_types WHERE availability_id = ?`, set.ID).Scan(&refs); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if refs > 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "该可用时间集正被预约类型引用"}))
		return
	}
	if _, err := h.a.DB.ExecContext(r.Context(), `DELETE FROM availability_sets WHERE id = ?`, set.ID); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}
