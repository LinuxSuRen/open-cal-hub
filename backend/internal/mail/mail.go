// Package mail 抽象邮件发送：outbox（开发/联调，只落库）与 smtp 两种实现。
package mail

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	Kind    string // verify_email / reset_password / booking_confirmed / ...
	// Attachments 纯文本类附件（如 text/calendar 的 ICS）；outbox 模式追加到正文留档。
	Attachments []Attachment
}

// Attachment 是纯文本附件（避免引入二进制编码复杂度；日历邀请场景足够）。
type Attachment struct {
	Filename    string
	ContentType string
	Text        string
}

// Sender 邮件发送接口。
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Outbox 只写 mail_outbox 表，供开发与 e2e 检验。
type Outbox struct{ DB *sql.DB }

func (o *Outbox) Send(ctx context.Context, msg Message) error {
	now := time.Now().UTC().Format(time.RFC3339)
	text := msg.Text
	for _, att := range msg.Attachments {
		text += "\n\n--- attachment: " + att.Filename + " (" + att.ContentType + ") ---\n" + att.Text
	}
	_, err := o.DB.ExecContext(ctx, `INSERT INTO mail_outbox (to_addr, subject, body_text, body_html, kind, status, created_at, sent_at) VALUES (?,?,?,?,?, 'sent', ?, ?)`,
		msg.To, msg.Subject, text, msg.HTML, msg.Kind, now, now)
	return err
}

// SMTP 通过配置的中继发送；发送后仍写 outbox 一条 sent 记录留档。
type SMTP struct {
	DB                            *sql.DB
	Log                           *slog.Logger
	Host                          string
	Port                          int
	Username, Password, From, TLS string
}

func (s *SMTP) Send(ctx context.Context, msg Message) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	var err error
	switch s.TLS {
	case "tls":
		err = s.sendTLS(addr, msg)
	default: // starttls / none
		var auth smtp.Auth
		if s.Username != "" {
			auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
		}
		err = smtp.SendMail(addr, auth, s.From, []string{msg.To}, buildMIME(s.From, msg))
	}
	if s.DB != nil {
		status, dbErr := "sent", ""
		if err != nil {
			status, dbErr = "failed", err.Error()
		}
		text := msg.Text
		for _, att := range msg.Attachments {
			text += "\n\n--- attachment: " + att.Filename + " (" + att.ContentType + ") ---\n" + att.Text
		}
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO mail_outbox (to_addr, subject, body_text, body_html, kind, status, error, sent_at) VALUES (?,?,?,?,?,?,?,?)`,
			msg.To, msg.Subject, text, msg.HTML, msg.Kind, status, dbErr, time.Now().UTC().Format(time.RFC3339))
	}
	if err != nil {
		s.Log.Error("smtp_send_failed", "to", msg.To, "kind", msg.Kind, "error", err)
	}
	return err
}

// buildMIME 构造完整邮件：无附件时纯文本；有附件时 multipart/mixed。
func buildMIME(from string, msg Message) []byte {
	headers := "From: " + from + "\r\nTo: " + msg.To + "\r\nSubject: " + msg.Subject +
		"\r\nMIME-Version: 1.0\r\n"
	if len(msg.Attachments) == 0 {
		return []byte(headers + "Content-Type: text/plain; charset=utf-8\r\n\r\n" + msg.Text)
	}
	boundary := "calhub-" + fmt.Sprintf("%x", time.Now().UnixNano())
	var b strings.Builder
	b.WriteString(headers)
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(msg.Text)
	for _, att := range msg.Attachments {
		b.WriteString("\r\n--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + att.ContentType + "\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"" + att.Filename + "\"\r\n\r\n")
		b.WriteString(att.Text)
	}
	b.WriteString("\r\n--" + boundary + "--")
	return []byte(b.String())
}

func (s *SMTP) sendTLS(addr string, msg Message) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.From); err != nil {
		return err
	}
	if err := c.Rcpt(msg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(buildMIME(s.From, msg)); err != nil {
		return err
	}
	return w.Close()
}
