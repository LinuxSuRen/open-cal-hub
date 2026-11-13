// Package auth 提供 JWT 签发校验、上下文用户与 HTTP 认证中间件。
// 密码、会话、令牌等完整流程由本包其余文件补充。
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Role     string `json:"role"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Signer 签发与校验 access token。
type Signer struct {
	secret []byte
	ttl    time.Duration
}

func NewSigner(secret string, ttl time.Duration) *Signer {
	return &Signer{secret: []byte(secret), ttl: ttl}
}

func (s *Signer) TTL() time.Duration { return s.ttl }

func (s *Signer) Sign(userID int64, role, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Role:     role,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   itoa(userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
			Issuer:    "calhub",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Signer) Verify(token string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return &claims, nil
}

// User 请求上下文中的当前用户。
type User struct {
	ID       int64
	Username string
	Email    string
	Role     string
}

type ctxKey int

const userKey ctxKey = 1

func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func From(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userKey).(*User)
	return u, ok
}

// UserLoader 由 user 包提供：按 ID 加载用户（含角色与状态校验）。
type UserLoader func(ctx context.Context, id int64) (*User, error)

// Middleware 校验 Bearer token 并注入上下文用户。
// （CalDAV 的 Basic 认证由 dav 包单独实现，REST 层只接受 Bearer。）
func Middleware(signer *Signer, load UserLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := signer.Verify(bearerToken(r))
			if err != nil {
				unauthorized(w)
				return
			}
			u, err := load(r.Context(), claims.UserID)
			if err != nil || u == nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	httpx.WriteError(w, nil, httpx.Unauthorized())
}

// RequireAdmin 要求 admin 角色。
func RequireAdmin(signer *Signer, load UserLoader) func(http.Handler) http.Handler {
	mw := Middleware(signer, load)
	return func(next http.Handler) http.Handler {
		return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, ok := From(r.Context()); ok && u.Role == "admin" {
				next.ServeHTTP(w, r)
				return
			}
			httpx.WriteError(w, nil, httpx.Forbidden())
		}))
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
