// remind worker 测试：登记幂等、到期派发、停机恢复补登记。
package remind_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
	"github.com/linuxsuren/open-cal-hub/backend/internal/remind"
)

func newEnv(t *testing.T) *app.App {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Server.BaseURL = "http://calhub.test"
	cfg.Auth.JWTSecret = "unit-test-secret-0123456789"
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &app.App{
		Cfg: cfg, DB: database,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mail:   &mail.Outbox{DB: database},
		Signer: auth.NewSigner(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL),
	}
}

func addEventWithReminder(t *testing.T, a *app.App, calendarID int64, summary string, start time.Time, reminders []int) int64 {
	t.Helper()
	row, err := cal.CreateEventFromInput(context.Background(), a.DB, calendarID, "", cal.EventInput{
		Summary: summary, Start: start, End: start.Add(time.Hour), TZ: "Asia/Shanghai",
		Reminders: reminders,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return row.ID
}

func TestRemindWorker(t *testing.T) {
	a := newEnv(t)
	now := time.Now().UTC()

	// 主人 + 日历
	res, err := a.DB.Exec(`INSERT INTO users (email, username, password_hash, display_name, timezone, locale, created_at, updated_at)
		VALUES ('r@example.com','rick','x','Rick','Asia/Shanghai','zh-CN',?,?)`, cal.NowUTCString(), cal.NowUTCString())
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	uid, _ := res.LastInsertId()
	c, err := cal.CreateCalendar(context.Background(), a.DB, uid, "主日历", "", "", "Asia/Shanghai")
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}

	// 事件 A：40 分钟后开始，提前 30 分钟 → fire_at 10 分钟后（只登记不派发）
	addEventWithReminder(t, a, c.ID, "未来提醒", now.Add(40*time.Minute), []int{30})
	// 事件 B：25 分钟后开始，提前 30 分钟 → fire_at 5 分钟前（登记且立即到期）
	addEventWithReminder(t, a, c.ID, "到期提醒", now.Add(25*time.Minute), []int{30})
	// 事件 C：太迟（fire_at 30+ 分钟前）→ 超过补登记宽限，跳过
	addEventWithReminder(t, a, c.ID, "迟到提醒", now.Add(20*time.Minute), []int{60})
	// 事件 D：无提醒 → 不登记
	addEventWithReminder(t, a, c.ID, "无提醒", now.Add(40*time.Minute), nil)

	ctx := context.Background()
	remind.Scan(a, ctx)

	var total, sent int
	if err := a.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN sent_at IS NOT NULL THEN 1 ELSE 0 END),0) FROM reminders`).Scan(&total, &sent); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 { // A + B；C 超宽限跳过、D 无提醒
		t.Fatalf("登记数期望 2，实得 %d", total)
	}
	if sent != 1 {
		t.Fatalf("派发数期望 1，实得 %d", sent)
	}

	// 幂等：重扫不重复登记、不重复发信
	remind.Scan(a, ctx)
	var total2, mails int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM reminders`).Scan(&total2)
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM mail_outbox WHERE kind='event_reminder'`).Scan(&mails)
	if total2 != 2 || mails != 1 {
		t.Fatalf("重扫后 reminders=%d mails=%d（期望 2/1）", total2, mails)
	}

	// 邮件内容包含事件摘要
	var body string
	_ = a.DB.QueryRow(`SELECT body_text FROM mail_outbox WHERE kind='event_reminder' LIMIT 1`).Scan(&body)
	if body == "" || !contains(body, "到期提醒") {
		t.Fatalf("提醒邮件内容: %.200s", body)
	}

	// StartWorker 随 ctx 取消退出（冒烟）
	wctx, cancel := context.WithCancel(ctx)
	remind.StartWorker(a, wctx)
	cancel() // 立即取消，goroutine 应退出
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
