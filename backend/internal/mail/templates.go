// 中文邮件模板（纯文本为主，兼容各邮件客户端）。链接基于配置 base_url 拼接。
// 预约类模板由 booking/cal 包调用；认证流程由 user 包调用。
package mail

import (
	"strings"
	"time"
)

// RenderVerifyEmail 邮箱验证邮件（有效期 24 小时）。
func RenderVerifyEmail(siteName, baseURL, token string) (subject, text string) {
	link := baseURL + "/verify?token=" + token
	subject = "【" + siteName + "】请验证你的邮箱"
	text = "你好！\n\n" +
		"感谢注册 " + siteName + "。请点击下面的链接完成邮箱验证（24 小时内有效）：\n\n" +
		link + "\n\n" +
		"如果这不是你本人的操作，请忽略本邮件。\n\n" +
		"—— " + siteName
	return
}

// RenderResetPassword 密码重置邮件（有效期 1 小时）。
func RenderResetPassword(siteName, baseURL, token string) (subject, text string) {
	link := baseURL + "/reset?token=" + token
	subject = "【" + siteName + "】重置密码"
	text = "你好！\n\n" +
		"我们收到了重置密码的请求。请点击下面的链接设置新密码（1 小时内有效，且只能使用一次）：\n\n" +
		link + "\n\n" +
		"如果这不是你本人的操作，请忽略本邮件，你的密码不会发生变化。\n\n" +
		"—— " + siteName
	return
}

// RenderEmailChanged 邮箱变更通知。
func RenderEmailChanged(siteName, newEmail string) (subject, text string) {
	subject = "【" + siteName + "】你的邮箱已更新"
	text = "你好！\n\n" +
		"你的 " + siteName + " 账号绑定邮箱已变更为：" + newEmail + "。\n" +
		"如果这不是你本人的操作，请立即登录并修改密码，或联系管理员。\n\n" +
		"—— " + siteName
	return
}

// formatLocalTime 按客人时区展示时间；tz 无效时退回 UTC。
func formatLocalTime(startUTC time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return startUTC.In(loc).Format("2006-01-02 15:04 (MST)")
}

// BookingEvent 预约通知共用的信息。
type BookingEvent struct {
	EventTypeName string
	HostName      string
	GuestName     string
	GuestEmail    string
	Start         time.Time
	End           time.Time
	Timezone      string
	Note          string
	ManageURL     string // 相对路径，如 /b/abc123；可为空
}

func (e BookingEvent) when() string {
	s := formatLocalTime(e.Start, e.Timezone)
	en := formatLocalTime(e.End, e.Timezone)
	return s + " 至 " + en
}

func (e BookingEvent) footer(siteName, baseURL string) string {
	var b strings.Builder
	if e.ManageURL != "" {
		b.WriteString("\n预约管理链接：" + baseURL + e.ManageURL + "\n")
	}
	b.WriteString("\n—— " + siteName)
	return b.String()
}

// RenderBookingConfirmedGuest 预约确认（客人）。
func RenderBookingConfirmedGuest(siteName, baseURL string, e BookingEvent) (subject, text string) {
	subject = "【" + siteName + "】预约已确认：" + e.EventTypeName
	text = e.GuestName + "，你好！\n\n" +
		"你的预约已确认：\n\n" +
		"  主题：" + e.EventTypeName + "\n" +
		"  主人：" + e.HostName + "\n" +
		"  时间：" + e.when() + "\n" +
		footerNote(e.Note) +
		e.footer(siteName, baseURL)
	return
}

// RenderBookingConfirmedHost 预约通知（主人）。
func RenderBookingConfirmedHost(siteName, baseURL string, e BookingEvent) (subject, text string) {
	subject = "【" + siteName + "】新预约：" + e.EventTypeName + " - " + e.GuestName
	text = e.HostName + "，你好！\n\n" +
		"收到一个新的预约：\n\n" +
		"  主题：" + e.EventTypeName + "\n" +
		"  客人：" + e.GuestName + " <" + e.GuestEmail + ">\n" +
		"  时间：" + e.when() + "\n" +
		footerNote(e.Note) +
		e.footer(siteName, baseURL)
	return
}

// RenderBookingCancelled 预约取消通知（双方通用，byWho 为取消方说明）。
func RenderBookingCancelled(siteName, baseURL, byWho string, e BookingEvent) (subject, text string) {
	subject = "【" + siteName + "】预约已取消：" + e.EventTypeName
	text = "你好！\n\n" +
		"以下预约已被" + byWho + "取消：\n\n" +
		"  主题：" + e.EventTypeName + "\n" +
		"  主人：" + e.HostName + "\n" +
		"  客人：" + e.GuestName + "\n" +
		"  原时间：" + e.when() + "\n" +
		footerNote(e.Note) +
		e.footer(siteName, baseURL)
	return
}

// RenderBookingRescheduled 预约改期通知（newStart/newEnd 为新时间）。
func RenderBookingRescheduled(siteName, baseURL string, e BookingEvent, oldStart, oldEnd time.Time) (subject, text string) {
	subject = "【" + siteName + "】预约已改期：" + e.EventTypeName
	oldTZ := e.Timezone
	loc, err := time.LoadLocation(oldTZ)
	if err != nil {
		loc = time.UTC
	}
	text = "你好！\n\n" +
		"以下预约已改期：\n\n" +
		"  主题：" + e.EventTypeName + "\n" +
		"  主人：" + e.HostName + "\n" +
		"  客人：" + e.GuestName + "\n" +
		"  原时间：" + oldStart.In(loc).Format("2006-01-02 15:04 (MST)") + " 至 " + oldEnd.In(loc).Format("2006-01-02 15:04 (MST)") + "\n" +
		"  新时间：" + e.when() + "\n" +
		footerNote(e.Note) +
		e.footer(siteName, baseURL)
	return
}

func footerNote(note string) string {
	if note == "" {
		return ""
	}
	return "  备注：" + note + "\n"
}
