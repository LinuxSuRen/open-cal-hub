// 初始管理员引导测试：空库创建、幂等跳过、半配置报错、弱密码拒绝。
package user

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
)

func newBootstrapApp(t *testing.T) *app.App {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Path = filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &app.App{Cfg: cfg, DB: database, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("空库创建 admin", func(t *testing.T) {
		a := newBootstrapApp(t)
		created, err := BootstrapAdminFromConfig(ctx, a, "ops@example.com", "s3curePass1")
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		u, err := UserByEmail(ctx, a.DB, "ops@example.com")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if u.Role != "admin" || u.Status != "active" {
			t.Fatalf("role/status: %+v", u)
		}
		if !u.EmailVerifiedAt.Valid || u.EmailVerifiedAt.String == "" {
			t.Fatal("引导账号应视为已验证")
		}
		// 可登录（密码哈希有效）
		if !auth.CheckPassword("s3curePass1", u.PasswordHash) {
			t.Fatal("密码哈希应可校验")
		}
	})

	t.Run("已有用户幂等跳过", func(t *testing.T) {
		a := newBootstrapApp(t)
		if _, err := BootstrapAdminFromConfig(ctx, a, "ops@example.com", "s3curePass1"); err != nil {
			t.Fatalf("first: %v", err)
		}
		created, err := BootstrapAdminFromConfig(ctx, a, "other@example.com", "s3curePass2")
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if created {
			t.Fatal("非空库不应再创建")
		}
		if n, _ := CountUsers(ctx, a.DB); n != 1 {
			t.Fatalf("用户数应保持 1，实得 %d", n)
		}
	})

	t.Run("半配置报错", func(t *testing.T) {
		a := newBootstrapApp(t)
		if _, err := BootstrapAdminFromConfig(ctx, a, "", "s3curePass1"); err == nil {
			t.Fatal("只配密码应报错")
		}
		if _, err := BootstrapAdminFromConfig(ctx, a, "ops@example.com", ""); err == nil {
			t.Fatal("只配邮箱应报错")
		}
	})

	t.Run("弱密码与坏邮箱拒绝", func(t *testing.T) {
		a := newBootstrapApp(t)
		if _, err := BootstrapAdminFromConfig(ctx, a, "ops@example.com", "weak"); err == nil {
			t.Fatal("弱密码应拒绝")
		}
		if _, err := BootstrapAdminFromConfig(ctx, a, "not-an-email", "s3curePass1"); err == nil {
			t.Fatal("坏邮箱应拒绝")
		}
		if n, _ := CountUsers(ctx, a.DB); n != 0 {
			t.Fatalf("拒绝时不应建用户，实得 %d", n)
		}
	})
}
