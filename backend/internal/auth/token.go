// 一次性令牌（邮箱验证/密码重置）：随机 32B、只存 SHA-256 哈希、有期限、一次性。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// ErrTokenInvalid 令牌不存在或已使用；ErrTokenExpired 令牌已过期。
var (
	ErrTokenInvalid = errors.New("token invalid or used")
	ErrTokenExpired = errors.New("token expired")
)

// TokenTTL 各类令牌有效期。
const (
	VerifyEmailTTL   = 24 * time.Hour
	ResetPasswordTTL = time.Hour
)

// NewToken 生成 32 字节随机令牌（url-safe base64 明文）与它的 SHA-256 十六进制哈希。
func NewToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, HashToken(plain), nil
}

// HashToken 计算 SHA-256 十六进制哈希（入库形态）。
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// SaveToken 落库一个待用令牌。payload 可存附加信息（如发起方 IP）。
func SaveToken(ctx context.Context, db *sql.DB, userID int64, kind, hash, payload string, ttl time.Duration) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO auth_tokens (user_id, kind, token_hash, payload, expires_at, created_at) VALUES (?,?,?,?,?,?)`,
		userID, kind, hash, payload, time.Now().UTC().Add(ttl).Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	return err
}

// ConsumeToken 原子消费令牌（一次性）：返回 user_id；错误区分无效与过期。
// 采用「先读后条件更新」并把过期判断放在 UPDATE 条件里，避免竞态双花。
func ConsumeToken(ctx context.Context, db *sql.DB, kind, plain string) (int64, error) {
	hash := HashToken(plain)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx,
		`UPDATE auth_tokens SET used_at=? WHERE kind=? AND token_hash=? AND used_at IS NULL AND expires_at>?`,
		now, kind, hash, now)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 判断到底是无效还是过期，以返回准确的错误码。
		var expires string
		var used sql.NullString
		err := db.QueryRowContext(ctx, `SELECT expires_at, used_at FROM auth_tokens WHERE kind=? AND token_hash=?`, kind, hash).Scan(&expires, &used)
		if err != nil {
			return 0, ErrTokenInvalid
		}
		if used.Valid {
			return 0, ErrTokenInvalid
		}
		return 0, ErrTokenExpired
	}
	var userID int64
	if err := db.QueryRowContext(ctx, `SELECT user_id FROM auth_tokens WHERE kind=? AND token_hash=?`, kind, hash).Scan(&userID); err != nil {
		return 0, err
	}
	return userID, nil
}

// InvalidateUserTokens 作废某用户某类的全部未用令牌（重发前清理，防止堆积）。
func InvalidateUserTokens(ctx context.Context, db *sql.DB, userID int64, kind string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE auth_tokens SET used_at=? WHERE user_id=? AND kind=? AND used_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), userID, kind)
	return err
}
