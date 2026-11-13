// Package booking 提供预约域能力：可用时间集、预约类型、外部 ICS 源同步、
// 槽位计算引擎、公开预约端点与下单/取消/改期事务。
package booking

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// Host 是预约主人（公开页展示字段）。
type Host struct {
	ID          int64
	Username    string
	DisplayName string
	Bio         string
	Timezone    string
	Email       string
}

// LoadHostByUsername 加载活跃用户作为预约主人。
func LoadHostByUsername(ctx context.Context, q cal.DBTX, username string) (*Host, error) {
	var h Host
	err := q.QueryRowContext(ctx, `SELECT id, username, display_name, bio, timezone, email FROM users
		WHERE username = ? AND status = 'active'`, username).
		Scan(&h.ID, &h.Username, &h.DisplayName, &h.Bio, &h.Timezone, &h.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("booking/host_not_found", "用户不存在")
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// EventType 是预约类型（全量字段）。
type EventType struct {
	ID               int64   `json:"id"`
	Slug             string  `json:"slug"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	DurationMinutes  int     `json:"duration_minutes"`
	LocationType     string  `json:"location_type"`
	LocationValue    string  `json:"location_value"`
	Color            string  `json:"color"`
	BufferBefore     int     `json:"buffer_before"`
	BufferAfter      int     `json:"buffer_after"`
	SlotInterval     int     `json:"slot_interval"`
	MinNoticeMinutes int     `json:"min_notice_minutes"`
	RangeType        string  `json:"range_type"`
	RangeDays        int     `json:"range_days"`
	StartDate        *string `json:"start_date"`
	EndDate          *string `json:"end_date"`
	AvailabilityID   *int64  `json:"availability_id"`
	CalendarID       int64   `json:"calendar_id"`
	Active           bool    `json:"active"`
	BookingURL       string  `json:"booking_url"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

const eventTypeCols = `id, user_id, slug, name, description, duration_minutes, location_type, location_value,
 color, buffer_before, buffer_after, slot_interval, min_notice_minutes, range_type, range_days,
 start_date, end_date, availability_id, calendar_id, active, created_at, updated_at`

func scanEventType(scan func(dest ...any) error, username string) (*EventType, error) {
	var (
		et           EventType
		userID       int64
		startDate    sql.NullString
		endDate      sql.NullString
		availability sql.NullInt64
	)
	if err := scan(&et.ID, &userID, &et.Slug, &et.Name, &et.Description, &et.DurationMinutes,
		&et.LocationType, &et.LocationValue, &et.Color, &et.BufferBefore, &et.BufferAfter,
		&et.SlotInterval, &et.MinNoticeMinutes, &et.RangeType, &et.RangeDays,
		&startDate, &endDate, &availability, &et.CalendarID, &et.Active, &et.CreatedAt, &et.UpdatedAt); err != nil {
		return nil, err
	}
	if startDate.Valid {
		s := startDate.String
		et.StartDate = &s
	}
	if endDate.Valid {
		s := endDate.String
		et.EndDate = &s
	}
	if availability.Valid {
		v := availability.Int64
		et.AvailabilityID = &v
	}
	et.BookingURL = "/u/" + username + "/" + et.Slug
	return &et, nil
}

// EventTypeByID 按 ID 取预约类型并校验属主。
func EventTypeByID(ctx context.Context, q cal.DBTX, id, userID int64) (*EventType, error) {
	row := q.QueryRowContext(ctx, `SELECT `+eventTypeCols+` FROM event_types WHERE id = ?`, id)
	et, err := scanEventType(row.Scan, "")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("booking/event_type_not_found", "预约类型不存在")
	}
	if err != nil {
		return nil, err
	}
	return et, nil
}

// EventTypeBySlug 按主人用户名与 slug 取类型。
func EventTypeBySlug(ctx context.Context, q cal.DBTX, host *Host, slug string) (*EventType, error) {
	row := q.QueryRowContext(ctx, `SELECT `+eventTypeCols+` FROM event_types WHERE user_id = ? AND slug = ?`,
		host.ID, slug)
	et, err := scanEventType(row.Scan, host.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("booking/event_type_not_found", "预约类型不存在")
	}
	if err != nil {
		return nil, err
	}
	return et, nil
}

// AvailabilitySet 是可用时间集。
type AvailabilitySet struct {
	ID        int64      `json:"id"`
	OwnerID   int64      `json:"-"`
	Name      string     `json:"name"`
	Timezone  string     `json:"timezone"`
	IsDefault bool       `json:"is_default"`
	Rules     []Rule     `json:"rules"`
	Dates     []DateRule `json:"dates"`
}

// Rule 是每周重复的可用时段；weekday 0=周日，分钟 0..1439。
type Rule struct {
	Weekday     int `json:"weekday"`
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

// DateRule 是特定日期的覆盖：available 追加、unavailable 扣除。
type DateRule struct {
	Date        string `json:"date"`
	Kind        string `json:"kind"`
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
}

// LoadAvailability 取可用时间集；不存在或非属主返回 nil。
func LoadAvailability(ctx context.Context, q cal.DBTX, id, userID int64) (*AvailabilitySet, error) {
	var (
		set   AvailabilitySet
		owner int64
	)
	err := q.QueryRowContext(ctx, `SELECT id, user_id, name, timezone, is_default FROM availability_sets WHERE id = ?`, id).
		Scan(&set.ID, &owner, &set.Name, &set.Timezone, &set.IsDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, nil
	}
	set.OwnerID = owner
	if err := loadSetDetails(ctx, q, &set); err != nil {
		return nil, err
	}
	return &set, nil
}

// LoadAvailabilityByID 不校验属主（公开页读取主人配置）。
func LoadAvailabilityByID(ctx context.Context, q cal.DBTX, id int64) (*AvailabilitySet, error) {
	var (
		set   AvailabilitySet
		owner int64
	)
	err := q.QueryRowContext(ctx, `SELECT id, user_id, name, timezone, is_default FROM availability_sets WHERE id = ?`, id).
		Scan(&set.ID, &owner, &set.Name, &set.Timezone, &set.IsDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	set.OwnerID = owner
	if err := loadSetDetails(ctx, q, &set); err != nil {
		return nil, err
	}
	return &set, nil
}

func loadSetDetails(ctx context.Context, q cal.DBTX, set *AvailabilitySet) error {
	set.Rules = []Rule{}
	set.Dates = []DateRule{}
	rows, err := q.QueryContext(ctx,
		`SELECT weekday, start_minute, end_minute FROM availability_rules WHERE availability_id = ? ORDER BY weekday, start_minute`, set.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.Weekday, &r.StartMinute, &r.EndMinute); err != nil {
			return err
		}
		set.Rules = append(set.Rules, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	dates, err := q.QueryContext(ctx,
		`SELECT date, kind, start_minute, end_minute FROM availability_dates WHERE availability_id = ? ORDER BY date, start_minute`, set.ID)
	if err != nil {
		return err
	}
	defer dates.Close()
	for dates.Next() {
		var d DateRule
		if err := dates.Scan(&d.Date, &d.Kind, &d.StartMinute, &d.EndMinute); err != nil {
			return err
		}
		set.Dates = append(set.Dates, d)
	}
	return dates.Err()
}

// SaveRules 整体替换规则与日期。
func SaveRules(ctx context.Context, q cal.DBTX, setID int64, rules []Rule, dates []DateRule) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM availability_rules WHERE availability_id = ?`, setID); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM availability_dates WHERE availability_id = ?`, setID); err != nil {
		return err
	}
	for _, r := range rules {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO availability_rules (availability_id, weekday, start_minute, end_minute) VALUES (?,?,?,?)`,
			setID, r.Weekday, r.StartMinute, r.EndMinute); err != nil {
			return err
		}
	}
	for _, d := range dates {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO availability_dates (availability_id, date, kind, start_minute, end_minute) VALUES (?,?,?,?,?)`,
			setID, d.Date, d.Kind, d.StartMinute, d.EndMinute); err != nil {
			return err
		}
	}
	return nil
}

func validTimezone(tz string) bool {
	if tz == "" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
