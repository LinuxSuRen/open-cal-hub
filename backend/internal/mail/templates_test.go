package mail

import (
	"strings"
	"testing"
	"time"
)

func TestRenderVerifyEmail(t *testing.T) {
	subject, text := RenderVerifyEmail("CalHub", "http://x.example", "tok123")
	if !strings.Contains(subject, "验证") {
		t.Fatalf("subject: %s", subject)
	}
	if !strings.Contains(text, "http://x.example/verify?token=tok123") {
		t.Fatalf("link missing: %s", text)
	}
}

func TestRenderResetEmail(t *testing.T) {
	_, text := RenderResetPassword("CalHub", "http://x.example", "rtok")
	if !strings.Contains(text, "http://x.example/reset?token=rtok") {
		t.Fatalf("link missing: %s", text)
	}
}

func TestRenderBookingTemplates(t *testing.T) {
	start := time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC) // 北京时间 09:00
	e := BookingEvent{
		EventTypeName: "30 分钟会谈", HostName: "Rick", GuestName: "Neo",
		GuestEmail: "neo@example.com", Start: start, End: start.Add(30 * time.Minute),
		Timezone: "Asia/Shanghai", Note: "聊聊架构", ManageURL: "/b/abc",
	}
	s1, t1 := RenderBookingConfirmedGuest("CalHub", "http://x.example", e)
	if !strings.Contains(t1, "30 分钟会谈") || !strings.Contains(t1, "Neo") || !strings.Contains(t1, "09:00") {
		t.Fatalf("guest mail wrong: %s", t1)
	}
	if !strings.Contains(t1, "http://x.example/b/abc") {
		t.Fatalf("manage url missing: %s", t1)
	}
	_ = s1
	s2, t2 := RenderBookingConfirmedHost("CalHub", "http://x.example", e)
	if !strings.Contains(t2, "neo@example.com") {
		t.Fatalf("host mail should include guest email: %s", t2)
	}
	_ = s2
	_, t3 := RenderBookingCancelled("CalHub", "http://x.example", "主人", e)
	if !strings.Contains(t3, "取消") {
		t.Fatalf("cancel mail: %s", t3)
	}
	_, t4 := RenderBookingRescheduled("CalHub", "http://x.example", e, start, start.Add(30*time.Minute))
	if !strings.Contains(t4, "新时间") {
		t.Fatalf("reschedule mail: %s", t4)
	}
}
