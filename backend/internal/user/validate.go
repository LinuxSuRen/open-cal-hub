// 注册/账号输入校验：邮箱、密码强度、用户名规则与保留字。
package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"math/big"
	"regexp"
	"strings"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

var (
	emailRe     = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
	usernameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,30}$`)
	sanitizeRe  = regexp.MustCompile(`[^a-z0-9]+`)
)

// reservedUsernames 契约 §2 保留字：路由/系统语义用户名禁止注册。
var reservedUsernames = map[string]bool{
	"admin": true, "api": true, "u": true, "s": true, "b": true, "dav": true,
	"login": true, "logout": true, "register": true, "me": true, "pub": true,
	"static": true, "assets": true, "healthz": true, "readyz": true, "metrics": true,
	"support": true, "help": true,
}

// NormalizeEmail 规范化邮箱（小写、去空白）。
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// ValidateEmail 校验邮箱格式，返回用户可见错误（nil=通过）。
func ValidateEmail(email string) *httpx.Error {
	if len(email) < 5 || len(email) > 254 || !emailRe.MatchString(email) {
		return httpx.Validation(map[string]string{"email": "邮箱格式不正确"})
	}
	return nil
}

// ValidatePasswordStrength 密码强度：≥8 位且同时含字母与数字。
func ValidatePasswordStrength(pw string) *httpx.Error {
	if len(pw) < 8 || len(pw) > 128 {
		return httpx.E("auth/weak_password", "密码强度不足，至少 8 位且需包含字母与数字")
	}
	var hasLetter, hasDigit bool
	for _, c := range pw {
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return httpx.E("auth/weak_password", "密码强度不足，至少 8 位且需包含字母与数字")
	}
	return nil
}

// ValidateUsername 用户名规则与保留字。
func ValidateUsername(name string) *httpx.Error {
	if !usernameRe.MatchString(name) {
		return httpx.Validation(map[string]string{"username": "用户名需 2-31 位，小写字母开头，可含数字、下划线、中划线"})
	}
	if reservedUsernames[name] {
		return httpx.E("auth/reserved_username", "该用户名不可用")
	}
	return nil
}

// GenerateUsername 从邮箱前缀派生用户名；非法/冲突时加随机后缀。
// 只保证不与现存用户名冲突；调用方仍需处理并发插入的唯一约束错误。
func GenerateUsername(ctx context.Context, db *sql.DB, email string) (string, error) {
	prefix := email
	if i := strings.IndexByte(email, '@'); i > 0 {
		prefix = email[:i]
	}
	prefix = sanitizeRe.ReplaceAllString(strings.ToLower(prefix), "-")
	prefix = strings.Trim(prefix, "-")
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	if !usernameRe.MatchString(prefix) || reservedUsernames[prefix] {
		prefix = "user"
	}
	candidate := prefix
	for attempt := 0; attempt < 8; attempt++ {
		taken, err := usernameTaken(ctx, db, candidate)
		if err != nil {
			return "", err
		}
		if !taken && !reservedUsernames[candidate] {
			return candidate, nil
		}
		candidate = prefix + "-" + randSuffix(4)
		if len(candidate) > 31 {
			candidate = prefix[:len(prefix)-(len(candidate)-31)] + "-" + randSuffix(4)
		}
	}
	return "u" + randSuffix(9), nil
}

func usernameTaken(ctx context.Context, db *sql.DB, name string) (bool, error) {
	_, err := UserByUsername(ctx, db, name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const nameAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randSuffix(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(nameAlphabet))))
		if err != nil {
			idx = big.NewInt(0)
		}
		b.WriteByte(nameAlphabet[idx.Int64()])
	}
	return b.String()
}
