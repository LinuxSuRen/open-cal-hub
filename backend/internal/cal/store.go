package cal

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

// Calendar 是日历聚合根。
type Calendar struct {
	ID          int64
	OwnerID     int64
	Name        string
	Description string
	Color       string
	Timezone    string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Role 常量：owner 全权；editor 可增删改事件；viewer 只读事件。
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// GetCalendar 按 ID 取未删除日历。
func GetCalendar(ctx context.Context, q DBTX, id int64) (*Calendar, error) {
	row := q.QueryRowContext(ctx, `SELECT id, owner_id, name, description, color, timezone,
		is_default, created_at, updated_at FROM calendars WHERE id = ? AND deleted_at IS NULL`, id)
	var (
		c               Calendar
		created, updated string
	)
	err := row.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Color, &c.Timezone,
		&c.IsDefault, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("cal/calendar_not_found", "日历不存在")
	}
	if err != nil {
		return nil, err
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	c.UpdatedAt, _ = parseTime(updated)
	return &c, nil
}

// ListCalendarsByOwner 列出用户自有日历。
func ListCalendarsByOwner(ctx context.Context, q DBTX, userID int64) ([]*Calendar, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, owner_id, name, description, color, timezone,
		is_default, created_at, updated_at FROM calendars
		WHERE owner_id = ? AND deleted_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Calendar
	for rows.Next() {
		var (
			c                Calendar
			created, updated string
		)
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Color, &c.Timezone,
			&c.IsDefault, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = parseTime(created)
		c.UpdatedAt, _ = parseTime(updated)
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CalendarIDsByOwner 返回用户全部自有日历 ID（busy 计算用）。
func CalendarIDsByOwner(ctx context.Context, q DBTX, userID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id FROM calendars WHERE owner_id = ? AND deleted_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateCalendar 创建日历；若为用户首个日历自动设为默认。
func CreateCalendar(ctx context.Context, q DBTX, ownerID int64, name, description, color, timezone string) (*Calendar, error) {
	if color == "" {
		color = "#4F6EF7"
	}
	if timezone == "" {
		timezone = "UTC"
	}
	var count int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM calendars WHERE owner_id = ? AND deleted_at IS NULL`, ownerID).Scan(&count); err != nil {
		return nil, err
	}
	now := fmtTime(nowUTC())
	res, err := q.ExecContext(ctx, `INSERT INTO calendars (owner_id, name, description, color, timezone, is_default, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, ownerID, name, description, color, timezone, count == 0, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return GetCalendar(ctx, q, id)
}

// UpdateCalendar 更新日历元数据（owner only）。
func UpdateCalendar(ctx context.Context, q DBTX, id int64, name, description, color, timezone *string) (*Calendar, error) {
	c, err := GetCalendar(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if name != nil && *name != "" {
		c.Name = *name
	}
	if description != nil {
		c.Description = *description
	}
	if color != nil && *color != "" {
		c.Color = *color
	}
	if timezone != nil && *timezone != "" {
		if _, err := time.LoadLocation(*timezone); err != nil {
			return nil, httpx.Validation(map[string]string{"timezone": "时区无效"})
		}
		c.Timezone = *timezone
	}
	_, err = q.ExecContext(ctx, `UPDATE calendars SET name=?, description=?, color=?, timezone=?, updated_at=? WHERE id=?`,
		c.Name, c.Description, c.Color, c.Timezone, fmtTime(nowUTC()), id)
	if err != nil {
		return nil, err
	}
	return GetCalendar(ctx, q, id)
}

// DeleteCalendar 软删除日历；默认日历禁止删除。
func DeleteCalendar(ctx context.Context, q DBTX, id int64) error {
	c, err := GetCalendar(ctx, q, id)
	if err != nil {
		return err
	}
	if c.IsDefault {
		return httpx.Validation(map[string]string{"id": "默认日历不允许删除"})
	}
	_, err = q.ExecContext(ctx, `UPDATE calendars SET deleted_at=?, updated_at=? WHERE id=?`,
		fmtTime(nowUTC()), fmtTime(nowUTC()), id)
	return err
}

// EnsureDefaultCalendar 为新用户创建默认日历（注册钩子，幂等）。
func EnsureDefaultCalendar(ctx context.Context, q DBTX, userID int64, timezone string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx,
		`SELECT id FROM calendars WHERE owner_id = ? AND deleted_at IS NULL ORDER BY id LIMIT 1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	c, err := CreateCalendar(ctx, q, userID, "默认日历", "", "", timezone)
	if err != nil {
		return 0, err
	}
	return c.ID, nil
}

// Access 返回用户对日历的角色：owner/editor/viewer；无权限返回空串。
func Access(ctx context.Context, q DBTX, calendarID, userID int64) (string, error) {
	var ownerID int64
	err := q.QueryRowContext(ctx, `SELECT owner_id FROM calendars WHERE id = ? AND deleted_at IS NULL`, calendarID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", httpx.E("cal/calendar_not_found", "日历不存在")
	}
	if err != nil {
		return "", err
	}
	if ownerID == userID {
		return RoleOwner, nil
	}
	var role string
	err = q.QueryRowContext(ctx, `SELECT role FROM calendar_members WHERE calendar_id = ? AND user_id = ?`,
		calendarID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

// CanEditEvents 判断角色是否可写事件。
func CanEditEvents(role string) bool { return role == RoleOwner || role == RoleEditor }

// Member 是日历共享成员。
type Member struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   string    `json:"created_at"`
}

// ListMembers 列出日历成员（owner only 调用方保证）。
func ListMembers(ctx context.Context, q DBTX, calendarID int64) ([]Member, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.user_id, u.username, u.display_name, m.role, m.created_at
		FROM calendar_members m JOIN users u ON u.id = m.user_id
		WHERE m.calendar_id = ? ORDER BY m.created_at, m.user_id`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var created string
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role, &created); err != nil {
			return nil, err
		}
		if t, err := parseTime(created); err == nil {
			m.CreatedAt = fmtTime(t)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ResolveUser 按用户名或邮箱找活跃用户。
func ResolveUser(ctx context.Context, q DBTX, usernameOrEmail string) (id int64, username, displayName string, err error) {
	row := q.QueryRowContext(ctx, `SELECT id, username, display_name FROM users
		WHERE (username = ? OR email = ?) AND status = 'active' LIMIT 1`, usernameOrEmail, usernameOrEmail)
	if err := row.Scan(&id, &username, &displayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", "", httpx.Validation(map[string]string{"username_or_email": "用户不存在"})
		}
		return 0, "", "", err
	}
	return id, username, displayName, nil
}

// AddMember 添加共享成员；owner 不可被添加为成员。
func AddMember(ctx context.Context, q DBTX, calendarID, userID int64, role string) error {
	c, err := GetCalendar(ctx, q, calendarID)
	if err != nil {
		return err
	}
	if c.OwnerID == userID {
		return httpx.E("cal/member_exists", "该用户已在共享名单中")
	}
	if role != RoleViewer && role != RoleEditor {
		return httpx.Validation(map[string]string{"role": "角色必须为 viewer 或 editor"})
	}
	var exists int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM calendar_members WHERE calendar_id = ? AND user_id = ?`, calendarID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return httpx.E("cal/member_exists", "该用户已在共享名单中")
	}
	_, err = q.ExecContext(ctx, `INSERT INTO calendar_members (calendar_id, user_id, role, created_at) VALUES (?,?,?,?)`,
		calendarID, userID, role, fmtTime(nowUTC()))
	return err
}

// RemoveMember 移除共享成员。
func RemoveMember(ctx context.Context, q DBTX, calendarID, userID int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM calendar_members WHERE calendar_id = ? AND user_id = ?`, calendarID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return httpx.E("cal/member_not_found", "共享成员不存在")
	}
	return nil
}

// SharedWithMe 列出共享给用户的日历 ID 与角色。
func SharedWithMe(ctx context.Context, q DBTX, userID int64) (map[int64]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.calendar_id, m.role FROM calendar_members m
		JOIN calendars c ON c.id = m.calendar_id AND c.deleted_at IS NULL WHERE m.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		out[id] = role
	}
	return out, rows.Err()
}

// ShareLink 是公开分享链接（token 明文存储用于 URL 匹配）。
type ShareLink struct {
	ID         int64      `json:"id"`
	CalendarID int64      `json:"-"`
	Token      string     `json:"token"`
	URL        string     `json:"url"`
	Label      string     `json:"label"`
	CreatedAt  string     `json:"created_at"`
	ExpiresAt  *string    `json:"expires_at"`
	RevokedAt  *string    `json:"revoked_at,omitempty"`
}

// CreateShareLink 创建分享令牌。
func CreateShareLink(ctx context.Context, q DBTX, calendarID int64, label string, expiresAt *time.Time) (*ShareLink, error) {
	token := NewToken()
	now := fmtTime(nowUTC())
	var exp sql.NullString
	if expiresAt != nil {
		exp = sql.NullString{String: fmtTime(*expiresAt), Valid: true}
	}
	res, err := q.ExecContext(ctx, `INSERT INTO share_links (calendar_id, token, label, created_at, expires_at)
		VALUES (?,?,?,?,?)`, calendarID, token, label, now, exp)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	sl := &ShareLink{ID: id, CalendarID: calendarID, Token: token, URL: "/s/" + token,
		Label: label, CreatedAt: now}
	if exp.Valid {
		v := exp.String
		sl.ExpiresAt = &v
	}
	return sl, nil
}

// ListShareLinks 列出日历的分享链接。
func ListShareLinks(ctx context.Context, q DBTX, calendarID int64) ([]*ShareLink, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, calendar_id, token, label, created_at, expires_at, revoked_at
		FROM share_links WHERE calendar_id = ? ORDER BY id DESC`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ShareLink
	for rows.Next() {
		var sl ShareLink
		var exp, rev sql.NullString
		if err := rows.Scan(&sl.ID, &sl.CalendarID, &sl.Token, &sl.Label, &sl.CreatedAt, &exp, &rev); err != nil {
			return nil, err
		}
		sl.URL = "/s/" + sl.Token
		if exp.Valid {
			v := exp.String
			sl.ExpiresAt = &v
		}
		if rev.Valid {
			v := rev.String
			sl.RevokedAt = &v
		}
		out = append(out, &sl)
	}
	return out, rows.Err()
}

// RevokeShareLink 撤销分享链接。
func RevokeShareLink(ctx context.Context, q DBTX, id, ownerID int64) error {
	var calID int64
	err := q.QueryRowContext(ctx, `SELECT calendar_id FROM share_links WHERE id = ?`, id).Scan(&calID)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.E("cal/share_not_found", "分享不存在或已撤销")
	}
	if err != nil {
		return err
	}
	c, err := GetCalendar(ctx, q, calID)
	if err != nil {
		return err
	}
	if c.OwnerID != ownerID {
		return httpx.E("cal/share_not_found", "分享不存在或已撤销")
	}
	_, err = q.ExecContext(ctx, `UPDATE share_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		fmtTime(nowUTC()), id)
	return err
}

// ResolveShare 校验分享令牌并返回日历：撤销→share_not_found；过期→share_expired。
func ResolveShare(ctx context.Context, q DBTX, token string) (*Calendar, error) {
	var (
		id, calID int64
		exp, rev  sql.NullString
	)
	err := q.QueryRowContext(ctx, `SELECT id, calendar_id, expires_at, revoked_at FROM share_links WHERE token = ?`, token).
		Scan(&id, &calID, &exp, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.E("cal/share_not_found", "分享不存在或已撤销")
	}
	if err != nil {
		return nil, err
	}
	if rev.Valid {
		return nil, httpx.E("cal/share_not_found", "分享不存在或已撤销")
	}
	if exp.Valid {
		t, err := parseTime(exp.String)
		if err == nil && nowUTC().After(t) {
			return nil, httpx.E("cal/share_expired", "分享已过期")
		}
	}
	return GetCalendar(ctx, q, calID)
}

// SiteName 读取站点名（settings 表，供公开页展示）。
func SiteName(ctx context.Context, q DBTX) string {
	var v string
	if err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'site_name'`).Scan(&v); err != nil || v == "" {
		return "CalHub"
	}
	return v
}
