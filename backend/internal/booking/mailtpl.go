// 预约邮件：复用 internal/mail 的中文模板，补英文变体（按收件人 locale/
// 时区推断），并附带 METHOD:REQUEST / METHOD:CANCEL 的 ICS。
package booking

import (
	"context"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"

	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

// hostLocale 读取主人 locale（邮件语言选择用）。
func (h *handler) hostLocale(ctx context.Context, hostID int64) string {
	var locale string
	_ = h.a.DB.QueryRowContext(ctx, `SELECT locale FROM users WHERE id = ?`, hostID).Scan(&locale)
	return locale
}

// preferEnglish 按 locale 或时区推断英文模板。
func preferEnglish(locale, tz string) bool {
	if strings.HasPrefix(locale, "en") {
		return true
	}
	return strings.HasPrefix(tz, "America/") || strings.HasPrefix(tz, "Europe/") ||
		strings.HasPrefix(tz, "Australia/") || strings.HasPrefix(tz, "Pacific/")
}

// buildICS 生成单事件 VCALENDAR（METHOD:REQUEST 或 CANCEL）。
func buildICS(method string, b *Booking, et *EventType, host *Host, setTZ string) string {
	loc := calTZOrUTC(setTZ)
	ev := ics.NewEvent()
	ev.Props.SetText(ics.PropUID, b.ICSUID)
	ev.Props.SetDateTime(ics.PropDateTimeStamp, time.Now().UTC())
	ev.Props.SetText(ics.PropSummary, et.Name+" · "+b.GuestName)
	ev.Props.SetDateTime(ics.PropDateTimeStart, b.Start.In(loc))
	ev.Props.SetDateTime(ics.PropDateTimeEnd, b.End.In(loc))
	ev.Props.SetText(ics.PropStatus, map[bool]string{true: "CANCELLED", false: "CONFIRMED"}[method == "CANCEL"])
	if host.Email != "" {
		ev.Props.SetText("ORGANIZER", "mailto:"+host.Email)
	}
	if b.GuestEmail != "" {
		ev.Props.SetText("ATTENDEE", "mailto:"+b.GuestEmail)
	}
	vcal := ics.NewCalendar()
	vcal.Props.SetText(ics.PropProductID, cal.ProductID)
	vcal.Props.SetText(ics.PropVersion, "2.0")
	vcal.Props.SetText(ics.PropMethod, method)
	vcal.Children = append(vcal.Children, ev.Component)
	var sb strings.Builder
	_ = ics.NewEncoder(&sb).Encode(vcal)
	return sb.String()
}

func calTZOrUTC(name string) *time.Location {
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return time.UTC
}

// notifyConfirmed 预约成功：客人与主人各一封（附 REQUEST ICS）。
func (h *handler) notifyConfirmed(ctx context.Context, host *Host, et *EventType, b *Booking, manageToken string) {
	site := h.siteName(ctx)
	baseURL := h.baseURL()
	setTZ := h.setTZOf(ctx, et)
	ev := mail.BookingEvent{
		EventTypeName: et.Name,
		HostName:      host.DisplayName,
		GuestName:     b.GuestName, GuestEmail: b.GuestEmail,
		Start: b.Start, End: b.End, Timezone: b.GuestTZ, Note: b.GuestNote,
		ManageURL: "/b/" + manageToken,
	}
	icsData := buildICS("REQUEST", b, et, host, setTZ)

	subject, text := mail.RenderBookingConfirmedGuest(site, baseURL, ev)
	if preferEnglish("", b.GuestTZ) {
		subject, text = renderBookingConfirmedGuestEn(site, baseURL, ev)
	}
	h.sendMail(ctx, b.GuestEmail, subject, text, "booking_confirmed", icsData)

	hostSubject, hostText := mail.RenderBookingConfirmedHost(site, baseURL, ev)
	h.sendMail(ctx, host.Email, hostSubject, hostText, "booking_confirmed", icsData)
}

// notifyCancelled 取消：双方各一封（附 CANCEL ICS）。
func (h *handler) notifyCancelled(ctx context.Context, b *Booking) {
	et, host, ok := h.bookingContextBG(ctx, b)
	if !ok {
		return
	}
	site := h.siteName(ctx)
	baseURL := h.baseURL()
	setTZ := h.setTZOf(ctx, et)
	ev := mail.BookingEvent{
		EventTypeName: et.Name,
		HostName:      host.DisplayName,
		GuestName:     b.GuestName, GuestEmail: b.GuestEmail,
		Start: b.Start, End: b.End, Timezone: b.GuestTZ, Note: b.GuestNote,
		ManageURL: "",
	}
	byWho := "主人"
	if b.CancelledBy == "guest" {
		byWho = "客人"
	}
	icsData := buildICS("CANCEL", b, et, host, setTZ)

	subject, text := mail.RenderBookingCancelled(site, baseURL, byWho, ev)
	if preferEnglish(h.hostLocale(ctx, host.ID), b.GuestTZ) {
		subject, text = renderBookingCancelledEn(site, baseURL, byWhoEn(b.CancelledBy), ev)
	}
	h.sendMail(ctx, b.GuestEmail, subject, text, "booking_cancelled", icsData)
	h.sendMail(ctx, host.Email, subject, text, "booking_cancelled", icsData)
}

// notifyRescheduled 改期：双方各一封（附 REQUEST ICS）。
func (h *handler) notifyRescheduled(ctx context.Context, host *Host, et *EventType, b *Booking, oldStart, oldEnd time.Time) {
	site := h.siteName(ctx)
	baseURL := h.baseURL()
	setTZ := h.setTZOf(ctx, et)
	ev := mail.BookingEvent{
		EventTypeName: et.Name,
		HostName:      host.DisplayName,
		GuestName:     b.GuestName, GuestEmail: b.GuestEmail,
		Start: b.Start, End: b.End, Timezone: b.GuestTZ, Note: b.GuestNote,
	}
	icsData := buildICS("REQUEST", b, et, host, setTZ)

	subject, text := mail.RenderBookingRescheduled(site, baseURL, ev, oldStart, oldEnd)
	if preferEnglish(h.hostLocale(ctx, host.ID), b.GuestTZ) {
		subject, text = renderBookingRescheduledEn(site, baseURL, ev, oldStart, oldEnd)
	}
	h.sendMail(ctx, b.GuestEmail, subject, text, "booking_rescheduled", icsData)
	h.sendMail(ctx, host.Email, subject, text, "booking_rescheduled", icsData)
}

// bookingContextBG 是 bookingContext 的后台版本（无 *http.Request）。
func (h *handler) bookingContextBG(ctx context.Context, b *Booking) (*EventType, *Host, bool) {
	et, err := EventTypeByID(ctx, h.a.DB, b.EventTypeID, b.HostID)
	if err != nil {
		et = &EventType{ID: b.EventTypeID, Name: "预约", DurationMinutes: int(b.End.Sub(b.Start).Minutes())}
	}
	var host Host
	err = h.a.DB.QueryRowContext(ctx,
		`SELECT id, username, display_name, bio, timezone, email FROM users WHERE id = ?`, b.HostID).
		Scan(&host.ID, &host.Username, &host.DisplayName, &host.Bio, &host.Timezone, &host.Email)
	if err != nil {
		return nil, nil, false
	}
	return et, &host, true
}

func (h *handler) siteName(ctx context.Context) string {
	v := h.a.SiteName(ctx)
	if v == "" {
		return "CalHub"
	}
	return v
}

func (h *handler) baseURL() string {
	if h.a.Cfg != nil && h.a.Cfg.Server.BaseURL != "" {
		return strings.TrimSuffix(h.a.Cfg.Server.BaseURL, "/")
	}
	return ""
}

// setTZOf 取预约类型关联可用时间集的时区（事件时区口径）。
func (h *handler) setTZOf(ctx context.Context, et *EventType) string {
	if et.AvailabilityID == nil {
		return "UTC"
	}
	set, err := LoadAvailabilityByID(ctx, h.a.DB, *et.AvailabilityID)
	if err != nil || set == nil || set.Timezone == "" {
		return "UTC"
	}
	return set.Timezone
}

// sendMail 统一出口：失败只记日志，不影响业务结果。
func (h *handler) sendMail(ctx context.Context, to, subject, text, kind, icsData string) {
	if to == "" || h.a.Mail == nil {
		return
	}
	msg := mail.Message{To: to, Subject: subject, Text: text, Kind: kind}
	if icsData != "" {
		msg.Attachments = []mail.Attachment{{
			Filename:    "invite.ics",
			ContentType: "text/calendar; charset=utf-8",
			Text:        icsData,
		}}
	}
	if err := h.a.Mail.Send(ctx, msg); err != nil {
		h.a.Log.Error("booking_mail_send_failed", "kind", kind, "to", to, "error", err)
	}
}

// ---- 英文模板 ----

func byWhoEn(by string) string {
	if by == "guest" {
		return "the guest"
	}
	return "the host"
}

func renderBookingConfirmedGuestEn(site, baseURL string, e mail.BookingEvent) (string, string) {
	subject := "[" + site + "] Booking confirmed: " + e.EventTypeName
	text := "Hi " + e.GuestName + ",\n\n" +
		"Your booking is confirmed:\n\n" +
		"  Topic: " + e.EventTypeName + "\n" +
		"  Host: " + e.HostName + "\n" +
		"  Time: " + enWhen(e) + "\n" +
		enNote(e) + enFooter(site, baseURL, e)
	return subject, text
}

func renderBookingCancelledEn(site, baseURL, byWho string, e mail.BookingEvent) (string, string) {
	subject := "[" + site + "] Booking cancelled: " + e.EventTypeName
	text := "Hi,\n\n" +
		"The following booking has been cancelled by " + byWho + ":\n\n" +
		"  Topic: " + e.EventTypeName + "\n" +
		"  Host: " + e.HostName + "\n" +
		"  Guest: " + e.GuestName + "\n" +
		"  Time: " + enWhen(e) + "\n" +
		enNote(e) + enFooter(site, baseURL, e)
	return subject, text
}

func renderBookingRescheduledEn(site, baseURL string, e mail.BookingEvent, oldStart, oldEnd time.Time) (string, string) {
	subject := "[" + site + "] Booking rescheduled: " + e.EventTypeName
	text := "Hi,\n\n" +
		"The following booking has been rescheduled:\n\n" +
		"  Topic: " + e.EventTypeName + "\n" +
		"  Host: " + e.HostName + "\n" +
		"  Guest: " + e.GuestName + "\n" +
		"  Old time: " + enFormat(oldStart, e.Timezone) + " to " + enFormat(oldEnd, e.Timezone) + "\n" +
		"  New time: " + enWhen(e) + "\n" +
		enNote(e) + enFooter(site, baseURL, e)
	return subject, text
}

func enWhen(e mail.BookingEvent) string {
	return enFormat(e.Start, e.Timezone) + " to " + enFormat(e.End, e.Timezone)
}

func enFormat(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04 (MST)")
}

func enNote(e mail.BookingEvent) string {
	if e.Note == "" {
		return ""
	}
	return "  Note: " + e.Note + "\n"
}

func enFooter(site, baseURL string, e mail.BookingEvent) string {
	var b strings.Builder
	if e.ManageURL != "" {
		b.WriteString("\nManage booking: " + baseURL + e.ManageURL + "\n")
	}
	b.WriteString("\n-- " + site)
	return b.String()
}
