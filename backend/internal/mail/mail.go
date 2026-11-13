// Package mail 抽象邮件发送：outbox（开发/联调，只落库）与 smtp 两种实现。
package mail

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log/slog"
	"net/smtp"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	Kind    string // verify_email / reset_password / booking_confirmed / ...
}

// Sender 邮件发送接口。
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Outbox 只写 mail_outbox 表，供开发与 e2e 检验。
type Outbox struct{ DB *sql.DB }

func (o *Outbox) Send(ctx context.Context, msg Message) error {
	_, err := o.DB.ExecContext(ctx, `INSERT INTO mail_outbox (to_addr, subject, body_text, body_html, kind, status) VALUES (?,?,?,?,?, 'sent')`,
		msg.To, msg.Subject, msg.Text, msg.HTML, msg.Kind)
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
		from := s.From
		to := []string{msg.To}
		body := "From: " + from + "\r\nTo: " + msg.To + "\r\nSubject: " + msg.Subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + msg.Text
		err = smtp.SendMail(addr, auth, from, to, []byte(body))
	}
	if s.DB != nil {
		status, dbErr := "sent", ""
		if err != nil {
			status, dbErr = "failed", err.Error()
		}
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO mail_outbox (to_addr, subject, body_text, body_html, kind, status, error, sent_at) VALUES (?,?,?,?,?,?,?,?)`,
			msg.To, msg.Subject, msg.Text, msg.HTML, msg.Kind, status, dbErr, time.Now().UTC().Format(time.RFC3339))
	}
	if err != nil {
		s.Log.Error("smtp_send_failed", "to", msg.To, "kind", msg.Kind, "error", err)
	}
	return err
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
	body := "From: " + s.From + "\r\nTo: " + msg.To + "\r\nSubject: " + msg.Subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + msg.Text
	if _, err := w.Write([]byte(body)); err != nil {
		return err
	}
	return w.Close()
}
