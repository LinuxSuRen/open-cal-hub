// 管理员新增用户测试（复用 admin_test 包的 newEnv）。
package admin_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
)

func TestAdminCreateUser(t *testing.T) {
	e := newEnv(t)
	adminTok, _ := e.newUser("rick") // 首用户=admin
	// 与生产 routes.go 一致地接注册钩子（默认日历），验证新增用户触发
	e.a.OnUserRegistered = func(ctx context.Context, userID int64, tz string) error {
		_, err := cal.EnsureDefaultCalendar(ctx, e.a.DB, userID, tz)
		return err
	}

	post := func(body map[string]any) result {
		return e.do("POST", "/api/v1/admin/users", body, adminTok)
	}

	t.Run("显式密码创建并可直接登录", func(t *testing.T) {
		r := post(map[string]any{
			"email": "member@example.com", "username": "member",
			"password": "passw0rd1", "display_name": "成员甲",
		})
		if r.status != http.StatusCreated {
			t.Fatalf("create: %d %s", r.status, r.raw)
		}
		d := dataMap(r)
		if d["reset_email_sent"] != false {
			t.Fatalf("显式密码不应发重置邮件: %s", r.raw)
		}
		u := d["user"].(map[string]any)
		if u["username"] != "member" || u["email_verified"] != true || u["role"] != "user" {
			t.Fatalf("user: %s", r.raw)
		}
		// 新用户可直接登录（已验证）
		if lr := e.do("POST", "/api/v1/auth/login", map[string]string{
			"email": "member@example.com", "password": "passw0rd1",
		}, ""); lr.status != 200 {
			t.Fatalf("login: %d %s", lr.status, lr.raw)
		}
		// 注册钩子生效：默认日历存在
		var cals int
		_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM calendars c JOIN users u ON u.id=c.owner_id WHERE u.username='member'`).Scan(&cals)
		if cals != 1 {
			t.Fatalf("默认日历: %d", cals)
		}
	})

	t.Run("缺省密码：随机密码 + 重置邮件", func(t *testing.T) {
		r := post(map[string]any{"email": "invitee@example.com"})
		if r.status != http.StatusCreated {
			t.Fatalf("create: %d %s", r.status, r.raw)
		}
		if dataMap(r)["reset_email_sent"] != true {
			t.Fatalf("应发送重置邮件: %s", r.raw)
		}
		var n int
		_ = e.a.DB.QueryRow(`SELECT COUNT(*) FROM mail_outbox WHERE kind='reset_password' AND to_addr='invitee@example.com'`).Scan(&n)
		if n != 1 {
			t.Fatalf("重置邮件: %d", n)
		}
	})

	t.Run("role 指定与非法值", func(t *testing.T) {
		if r := post(map[string]any{"email": "op@example.com", "password": "passw0rd1", "role": "admin"}); r.status != 201 {
			t.Fatalf("role=admin: %d %s", r.status, r.raw)
		}
		if r := post(map[string]any{"email": "bad@example.com", "password": "passw0rd1", "role": "root"}); r.status != 400 {
			t.Fatalf("非法 role: %d", r.status)
		}
		var role string
		_ = e.a.DB.QueryRow(`SELECT role FROM users WHERE email='op@example.com'`).Scan(&role)
		if role != "admin" {
			t.Fatalf("op 角色: %s", role)
		}
	})

	t.Run("冲突与校验", func(t *testing.T) {
		if r := post(map[string]any{"email": "member@example.com", "password": "passw0rd1"}); r.status != 409 || errCode(r) != "auth/email_taken" {
			t.Fatalf("email taken: %d %s", r.status, r.raw)
		}
		if r := post(map[string]any{"email": "member2@example.com", "username": "member", "password": "passw0rd1"}); r.status != 409 || errCode(r) != "auth/username_taken" {
			t.Fatalf("username taken: %d %s", r.status, r.raw)
		}
		if r := post(map[string]any{"email": "bad", "password": "passw0rd1"}); r.status != 400 {
			t.Fatalf("bad email: %d", r.status)
		}
		if r := post(map[string]any{"email": "weak@example.com", "password": "weak"}); r.status != 400 {
			t.Fatalf("weak password: %d", r.status)
		}
		if r := post(map[string]any{"email": "tz@example.com", "password": "passw0rd1", "timezone": "Mars/Olympus"}); r.status != 400 {
			t.Fatalf("bad tz: %d", r.status)
		}
	})

	t.Run("审计写入", func(t *testing.T) {
		var action, detail string
		err := e.a.DB.QueryRow(`SELECT action, detail FROM audit_logs WHERE action='user.create' ORDER BY id DESC LIMIT 1`).
			Scan(&action, &detail)
		if err != nil {
			t.Fatalf("audit: %v", err)
		}
		if !containsStr(detail, `"password_delivery"`) {
			t.Fatalf("审计 detail: %s", detail)
		}
	})
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
