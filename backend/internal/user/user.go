// Package user：用户模型、存储与账号 HTTP 流程（注册/登录/令牌/资料/应用密码）。
package user

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// User 用户行（DB 全量字段）。
type User struct {
	ID             int64
	Email          string
	Username       string
	PasswordHash   string
	DisplayName    string
	Bio            string
	Timezone       string
	Locale         string
	Role           string
	Status         string
	EmailVerifiedAt sql.NullString
	LastLoginAt    sql.NullString
	CreatedAt      string
	UpdatedAt      string
}

// UserJSON 契约 §2 的 User 对象形态。
type UserJSON struct {
	ID            int64   `json:"id"`
	Email         string  `json:"email"`
	Username      string  `json:"username"`
	DisplayName   string  `json:"display_name"`
	Bio           string  `json:"bio"`
	Timezone      string  `json:"timezone"`
	Locale        string  `json:"locale"`
	Role          string  `json:"role"`
	Status        string  `json:"status"`
	EmailVerified bool    `json:"email_verified"`
	CreatedAt     string  `json:"created_at"`
	LastLoginAt   *string `json:"last_login_at"`
}

func (u *User) JSON() UserJSON {
	var last *string
	if u.LastLoginAt.Valid && u.LastLoginAt.String != "" {
		s := u.LastLoginAt.String
		last = &s
	}
	return UserJSON{
		ID:            u.ID,
		Email:         u.Email,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Bio:           u.Bio,
		Timezone:      u.Timezone,
		Locale:        u.Locale,
		Role:          u.Role,
		Status:        u.Status,
		EmailVerified: u.EmailVerifiedAt.Valid && u.EmailVerifiedAt.String != "",
		CreatedAt:     u.CreatedAt,
		LastLoginAt:   last,
	}
}

const userCols = `id, email, username, password_hash, display_name, bio, timezone, locale, role, status, email_verified_at, last_login_at, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Bio,
		&u.Timezone, &u.Locale, &u.Role, &u.Status, &u.EmailVerifiedAt, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// NowUTC 当前 UTC RFC3339 时间（admin 等包复用同一时间约定）。
func NowUTC() string { return nowUTC() }

// CreateUser 插入用户并回填 ID。
func CreateUser(ctx context.Context, db *sql.DB, u *User) error {
	now := nowUTC()
	u.CreatedAt, u.UpdatedAt = now, now
	res, err := db.ExecContext(ctx,
		`INSERT INTO users (email, username, password_hash, display_name, bio, timezone, locale, role, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		u.Email, u.Username, u.PasswordHash, u.DisplayName, u.Bio, u.Timezone, u.Locale, u.Role, u.Status, now, now)
	if err != nil {
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func UserByID(ctx context.Context, db *sql.DB, id int64) (*User, error) {
	return scanUser(db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=?`, id))
}

// UserByLogin 按邮箱或用户名查（登录、找回密码、公开预约页共用）。
func UserByLogin(ctx context.Context, db *sql.DB, login string) (*User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	return scanUser(db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE (email=? OR username=?) AND status<>'deleted'`, login, login))
}

func UserByEmail(ctx context.Context, db *sql.DB, email string) (*User, error) {
	return scanUser(db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(email))))
}

func UserByUsername(ctx context.Context, db *sql.DB, username string) (*User, error) {
	return scanUser(db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username=?`, username))
}

func CountUsers(ctx context.Context, db *sql.DB) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE status<>'deleted'`).Scan(&n)
	return n, err
}

func TouchLogin(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET last_login_at=?, updated_at=? WHERE id=?`, nowUTC(), nowUTC(), id)
	return err
}

func MarkEmailVerified(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET email_verified_at=?, updated_at=? WHERE id=? AND email_verified_at IS NULL`, nowUTC(), nowUTC(), id)
	return err
}

func SetPasswordHash(ctx context.Context, db *sql.DB, id int64, hash string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, nowUTC(), id)
	return err
}

// UpdateProfile 仅更新提供的字段（nil 表示不更新）。
func UpdateProfile(ctx context.Context, db *sql.DB, id int64, displayName, timezone, locale, bio *string) error {
	u, err := UserByID(ctx, db, id)
	if err != nil {
		return err
	}
	if displayName != nil {
		u.DisplayName = *displayName
	}
	if timezone != nil {
		u.Timezone = *timezone
	}
	if locale != nil {
		u.Locale = *locale
	}
	if bio != nil {
		u.Bio = *bio
	}
	_, err = db.ExecContext(ctx,
		`UPDATE users SET display_name=?, timezone=?, locale=?, bio=?, updated_at=? WHERE id=?`,
		u.DisplayName, u.Timezone, u.Locale, u.Bio, nowUTC(), id)
	return err
}
