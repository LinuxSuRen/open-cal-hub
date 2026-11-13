// Package remind 提供事件提醒 worker：周期扫描未来 24h 内带提醒的实例，
// 以 dedup_key（event_row_id:start:minutes）幂等登记 reminders 行，
// 到期发送邮件并标记 sent_at。StartWorker 由服务入口随生命周期启动。
package remind

import (
	"context"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

const (
	scanInterval = 30 * time.Second
	lookahead    = 24 * time.Hour
	// 登记时允许的最大迟到：worker 停机恢复后补登记最近到期的提醒
	registerGrace = 10 * time.Minute
)

// StartWorker 启动后台扫描循环，ctx 取消时退出。
func StartWorker(a *app.App, ctx context.Context) {
	if a == nil || a.DB == nil {
		return
	}
	go func() {
		tk := time.NewTicker(scanInterval)
		defer tk.Stop()
		Scan(a, ctx) // 启动即扫一次
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				Scan(a, ctx)
			}
		}
	}()
}

// Scan 执行一次登记与派发；单次失败只记日志。
func Scan(a *app.App, ctx context.Context) {
	now := time.Now().UTC()
	registered, err := register(a, ctx, now)
	if err != nil {
		a.Log.Error("remind_register_failed", "error", err)
	}
	fired, err := fire(a, ctx, now)
	if err != nil {
		a.Log.Error("remind_fire_failed", "error", err)
	}
	if registered > 0 || fired > 0 {
		a.Log.Info("remind_scan", "registered", registered, "fired", fired)
	}
}

// register 扫描窗口内的实例并幂等登记 reminders 行，返回登记数。
func register(a *app.App, ctx context.Context, now time.Time) (int, error) {
	horizon := now.Add(lookahead)

	// 日历属主映射（提醒收件人 = 日历属主）
	owners := map[int64]int64{}
	crows, err := a.DB.QueryContext(ctx, `SELECT id, owner_id FROM calendars WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	for crows.Next() {
		var calID, owner int64
		if err := crows.Scan(&calID, &owner); err != nil {
			crows.Close()
			return 0, err
		}
		owners[calID] = owner
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return 0, err
	}

	rows, err := a.DB.QueryContext(ctx, `SELECT `+cal.EventColumns()+` FROM events
		WHERE reminders <> '[]' AND status <> 'cancelled'
		  AND ((recurrence_id IS NULL AND (rrule <> '' OR (dtstart <= ? AND dtend >= ?)))
		       OR recurrence_id IS NOT NULL)`,
		cal.UTCString(horizon), cal.UTCString(now.Add(-registerGrace)))
	if err != nil {
		return 0, err
	}
	evts, err := cal.ScanEventRows(rows)
	rows.Close()
	if err != nil {
		return 0, err
	}
	byCalendar := map[int64][]*cal.EventRow{}
	for _, row := range evts {
		byCalendar[row.CalendarID] = append(byCalendar[row.CalendarID], row)
	}

	n := 0
	for calID, evts := range byCalendar {
		owner, ok := owners[calID]
		if !ok {
			continue
		}
		instances := cal.ExpandRange(evts, now.Add(-time.Minute), horizon, false)
		for _, inst := range instances {
			start, err := time.Parse(time.RFC3339, inst.Start)
			if err != nil {
				continue
			}
			for _, m := range inst.Reminders {
				fireAt := start.Add(-time.Duration(m) * time.Minute)
				if fireAt.Before(now.Add(-registerGrace)) {
					continue // 迟到过多的不补发
				}
				dedup := dedupKey(inst.ID, inst.Start, m)
				res, err := a.DB.ExecContext(ctx,
					`INSERT OR IGNORE INTO reminders (user_id, event_row_id, fire_at, dedup_key, created_at) VALUES (?,?,?,?,?)`,
					owner, inst.ID, cal.UTCString(fireAt), dedup, cal.NowUTCString())
				if err != nil {
					return n, err
				}
				if affected, _ := res.RowsAffected(); affected > 0 {
					n++
				}
			}
		}
	}
	return n, nil
}

func dedupKey(rowID int64, start string, minutes int) string {
	return itoa(rowID) + ":" + start + ":" + itoa(int64(minutes))
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// dueItem 是一条待派发的提醒。
type dueItem struct {
	id                  int64
	dedup               string
	summary, tz         string
	email, locale, name string
}

// fire 派发到期提醒并标记 sent_at。
func fire(a *app.App, ctx context.Context, now time.Time) (int, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT r.id, r.dedup_key, e.summary, e.tzid, u.email, u.locale, u.display_name
		FROM reminders r
		JOIN events e ON e.id = r.event_row_id
		JOIN users u ON u.id = r.user_id
		WHERE r.sent_at IS NULL AND r.fire_at <= ?`, cal.UTCString(now))
	if err != nil {
		return 0, err
	}
	var list []dueItem
	for rows.Next() {
		var d dueItem
		if err := rows.Scan(&d.id, &d.dedup, &d.summary, &d.tz, &d.email, &d.locale, &d.name); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	n := 0
	for _, d := range list {
		start := instanceStartOf(d.dedup)
		subject, text := renderReminder(a.SiteName(ctx), d, start)
		if err := a.Mail.Send(ctx, mail.Message{
			To: d.email, Subject: subject, Text: text, Kind: "event_reminder",
		}); err != nil {
			a.Log.Error("remind_mail_failed", "id", d.id, "error", err)
			continue
		}
		if _, err := a.DB.ExecContext(ctx, `UPDATE reminders SET sent_at = ? WHERE id = ?`,
			cal.NowUTCString(), d.id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// instanceStartOf 从 dedup_key（rowID:start:minutes）解析实例开始时刻。
func instanceStartOf(dedup string) time.Time {
	parts := strings.Split(dedup, ":")
	for _, p := range parts {
		if t, err := time.Parse(time.RFC3339, p); err == nil {
			return t
		}
	}
	return time.Time{}
}

func renderReminder(site string, d dueItem, start time.Time) (string, string) {
	loc, err := time.LoadLocation(d.tz)
	if err != nil {
		loc = time.UTC
	}
	when := start.In(loc).Format("2006-01-02 15:04 (MST)")
	if strings.HasPrefix(d.locale, "en") {
		return "[" + site + "] Reminder: " + d.summary,
			"Hi " + d.name + ",\n\n" +
				"This is a reminder for your upcoming event:\n\n" +
				"  " + d.summary + "\n  " + when + "\n\n-- " + site
	}
	return "【" + site + "】提醒：" + d.summary,
		d.name + "，你好！\n\n" +
			"你有一个即将开始的事件：\n\n" +
			"  " + d.summary + "\n  " + when + "\n\n—— " + site
}
