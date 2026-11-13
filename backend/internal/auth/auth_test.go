package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// insertUser 建一个最小用户行（满足外键）。
func insertUser(t *testing.T, d *sql.DB, id int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(`INSERT INTO users (id, email, username, password_hash, display_name, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		id, "u"+fmtI64(id)+"@t.local", "u"+fmtI64(id), "x", "u", now, now)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func fmtI64(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("passw0rd!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !CheckPassword("passw0rd!", h) {
		t.Fatal("correct password should match")
	}
	if CheckPassword("wrongpass1", h) {
		t.Fatal("wrong password should not match")
	}
	if CheckPassword("passw0rd!", "$argon2id$garbage") {
		t.Fatal("malformed hash must not authenticate")
	}
}

func TestTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	plain, hash, err := NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if len(plain) < 40 || HashToken(plain) != hash {
		t.Fatal("token/hash mismatch")
	}
	d := openDB(t)
	insertUser(t, d, 7)
	if err := SaveToken(ctx, d, 7, "verify_email", hash, "", time.Hour); err != nil {
		t.Fatalf("save: %v", err)
	}
	uid, err := ConsumeToken(ctx, d, "verify_email", plain)
	if err != nil || uid != 7 {
		t.Fatalf("consume: uid=%d err=%v", uid, err)
	}
	// 一次性：再用失效。
	if _, err := ConsumeToken(ctx, d, "verify_email", plain); err != ErrTokenInvalid {
		t.Fatalf("reuse should be ErrTokenInvalid, got %v", err)
	}
	// 未知令牌 → invalid（不是 expired）。
	if _, err := ConsumeToken(ctx, d, "verify_email", "no-such-token"); err != ErrTokenInvalid {
		t.Fatalf("unknown should be ErrTokenInvalid, got %v", err)
	}
}

func TestTokenExpiry(t *testing.T) {
	ctx := context.Background()
	plain, hash, _ := NewToken()
	d := openDB(t)
	insertUser(t, d, 1)
	_, err := d.Exec(`INSERT INTO auth_tokens (user_id, kind, token_hash, payload, expires_at, created_at) VALUES (1,'verify_email',?,'',?,?)`,
		hash, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := ConsumeToken(ctx, d, "verify_email", plain); err != ErrTokenExpired {
		t.Fatalf("expired should be ErrTokenExpired, got %v", err)
	}
}

func TestSessionRotateAndReuse(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	insertUser(t, d, 5)
	s := &Sessions{DB: d}
	tok1, sid, err := s.Create(ctx, 5, "ua-test", "1.2.3.4", time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tok2, uid, err := s.Rotate(ctx, tok1, time.Hour)
	if err != nil || uid != 5 {
		t.Fatalf("rotate: uid=%d err=%v", uid, err)
	}
	// 旧令牌重放 → ErrTokenReuse，且整个 family 被吊销。
	if _, _, err := s.Rotate(ctx, tok1, time.Hour); err != ErrTokenReuse {
		t.Fatalf("old token replay should be ErrTokenReuse, got %v", err)
	}
	if _, _, err := s.Rotate(ctx, tok2, time.Hour); err != ErrSessionNotFound {
		t.Fatalf("family revoked: current token should now be not found, got %v", err)
	}
	_ = sid
}

func TestSessionExpiryAndList(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	insertUser(t, d, 6)
	s := &Sessions{DB: d}
	tok, sid, err := s.Create(ctx, 6, "ua", "9.9.9.9", time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 过期会话：直接把 expires_at 改到过去模拟。
	_, _ = d.Exec(`UPDATE sessions SET expires_at=? WHERE id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339), sid)
	if _, _, err := s.Rotate(ctx, tok, time.Hour); err != ErrSessionExpired {
		t.Fatalf("expired session should be ErrSessionExpired, got %v", err)
	}
	// 活跃会话列表不包含过期会话。
	tok2, sid2, _ := s.Create(ctx, 6, "ua2", "8.8.8.8", time.Hour)
	list, err := s.List(ctx, 6)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, sess := range list {
		if sess.ID == sid2 {
			found = true
		}
		if sess.ID == sid {
			t.Fatal("expired session must not be listed")
		}
	}
	if !found {
		t.Fatal("active session missing from list")
	}
	// RevokeOthers 保留当前。
	if err := s.RevokeOthers(ctx, 6, sid2); err != nil {
		t.Fatalf("revoke others: %v", err)
	}
	list, _ = s.List(ctx, 6)
	if len(list) != 1 || list[0].ID != sid2 {
		t.Fatalf("expected only current session, got %d", len(list))
	}
	_ = tok2
}
