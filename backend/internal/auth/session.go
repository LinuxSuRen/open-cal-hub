// 刷新令牌会话：轮换 + family 重用检测。
//
// 模型：一次登录 = 一条 session 行（一个 family）。刷新令牌自包含会话 ID：
// "<sid>.<secret>"；库里只存 secret 的 SHA-256。刷新时轮换 secret（同行 UPDATE）；
// 若到来的 secret 与当前行不匹配但行存在（即旧代币重放），判定 family 被盗用，
// 吊销整个 family 的全部会话。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrSessionNotFound 会话不存在或已吊销；ErrSessionExpired 会话已过期；
// ErrTokenReuse 检测到刷新令牌重用（family 已被吊销）。
var (
	ErrSessionNotFound = errors.New("session not found or revoked")
	ErrSessionExpired  = errors.New("session expired")
	ErrTokenReuse      = errors.New("refresh token reuse detected")
)

// Session 会话行。
type Session struct {
	ID         int64
	UserID     int64
	FamilyID   string
	UserAgent  string
	IP         string
	CreatedAt  string
	LastUsedAt sql.NullString
	ExpiresAt  string
}

// Sessions 会话存储。
type Sessions struct{ DB *sql.DB }

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// Create 建立新会话并返回刷新令牌明文。
func (s *Sessions) Create(ctx context.Context, userID int64, ua, ip string, ttl time.Duration) (plainToken string, sessID int64, err error) {
	secret, hash, err := NewToken()
	if err != nil {
		return "", 0, err
	}
	family, _, err := NewToken()
	if err != nil {
		return "", 0, err
	}
	familyID := family[:16]
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (user_id, refresh_hash, family_id, user_agent, ip, created_at, last_used_at, expires_at)
		 VALUES (?,?,?,?,?,?,NULL,?)`,
		userID, hash, familyID, truncate(ua, 300), truncate(ip, 64), nowUTC(), time.Now().UTC().Add(ttl).Format(time.RFC3339))
	if err != nil {
		return "", 0, err
	}
	sessID, err = res.LastInsertId()
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%d.%s", sessID, secret), sessID, nil
}

// Rotate 校验并轮换刷新令牌，返回新令牌与 user_id。
// 行存在但 secret 不匹配 → 旧令牌重放：吊销整个 family 并返回 ErrTokenReuse。
func (s *Sessions) Rotate(ctx context.Context, plainToken string, ttl time.Duration) (newToken string, userID int64, err error) {
	sid, secret, ok := splitRefresh(plainToken)
	if !ok {
		return "", 0, ErrSessionNotFound
	}
	var (
		storedHash, expires string
		revoked             sql.NullString
	)
	err = s.DB.QueryRowContext(ctx, `SELECT refresh_hash, expires_at, revoked_at FROM sessions WHERE id=?`, sid).
		Scan(&storedHash, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrSessionNotFound
	}
	if err != nil {
		return "", 0, err
	}
	if revoked.Valid {
		// 已吊销的 family：当前代 secret 视为会话失效；旧代 secret 是又一次重放。
		if HashToken(secret) == storedHash {
			return "", 0, ErrSessionNotFound
		}
		s.revokeFamily(ctx, sid)
		return "", 0, ErrTokenReuse
	}
	if expires < nowUTC() {
		return "", 0, ErrSessionExpired
	}
	if HashToken(secret) != storedHash {
		// 旧代 secret 重放：吊销整个 family。
		s.revokeFamily(ctx, sid)
		return "", 0, ErrTokenReuse
	}
	newSecret, newHash, err := NewToken()
	if err != nil {
		return "", 0, err
	}
	newExpires := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET refresh_hash=?, last_used_at=?, expires_at=? WHERE id=? AND revoked_at IS NULL`,
		newHash, nowUTC(), newExpires, sid); err != nil {
		return "", 0, err
	}
	var uid int64
	if err := s.DB.QueryRowContext(ctx, `SELECT user_id FROM sessions WHERE id=?`, sid).Scan(&uid); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%d.%s", sid, newSecret), uid, nil
}

// splitRefresh 解析 "<sid>.<secret>"。
func splitRefresh(tok string) (int64, string, bool) {
	i := strings.IndexByte(tok, '.')
	if i <= 0 {
		return 0, "", false
	}
	sid, err := strconv.ParseInt(tok[:i], 10, 64)
	if err != nil || sid <= 0 {
		return 0, "", false
	}
	return sid, tok[i+1:], true
}

func (s *Sessions) revokeFamily(ctx context.Context, sid int64) {
	_, _ = s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at=? WHERE family_id=(SELECT family_id FROM sessions WHERE id=?) AND revoked_at IS NULL`,
		nowUTC(), sid)
}

// Revoke 吊销单个会话（登出）。
func (s *Sessions) Revoke(ctx context.Context, sid int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, nowUTC(), sid)
	return err
}

// RevokeByIDForUser 吊销指定用户的某个会话（仅当属于该用户）。返回是否命中。
func (s *Sessions) RevokeByIDForUser(ctx context.Context, userID, sid int64) (bool, error) {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`, nowUTC(), sid, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RevokeOthers 吊销某用户除指定会话外的全部会话（改密后保当前登录）。
func (s *Sessions) RevokeOthers(ctx context.Context, userID, keepSID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL AND id<>?`, nowUTC(), userID, keepSID)
	return err
}

// RevokeAll 吊销某用户全部会话（管理员停用/重置密码后）。
func (s *Sessions) RevokeAll(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, nowUTC(), userID)
	return err
}

// List 用户的活跃会话（未吊销未过期），新创建的在前。
func (s *Sessions) List(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_id, family_id, user_agent, ip, created_at, last_used_at, expires_at
		 FROM sessions WHERE user_id=? AND revoked_at IS NULL AND expires_at>? ORDER BY id DESC`,
		userID, nowUTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.FamilyID, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastUsedAt, &s.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionIDFromToken 从刷新令牌明文取会话 ID（不校验）。
func SessionIDFromToken(plainToken string) int64 {
	sid, _, ok := splitRefresh(plainToken)
	if !ok {
		return 0
	}
	return sid
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
