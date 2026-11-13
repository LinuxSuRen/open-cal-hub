// 管理端邮件出箱（分页 + 重试）与邀请码管理。
package admin

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

// ---- /admin/emails ----

func (h *handler) listEmails(w http.ResponseWriter, r *http.Request) {
	page, pageSize := httpx.PageParams(r)
	var total int64
	if err := h.a.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM mail_outbox`).Scan(&total); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(), `
		SELECT id, to_addr, subject, body_text, body_html, kind, status, error, created_at, sent_at
		FROM mail_outbox ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var to, subject, body, html, kind, statusV, createdAt string
		var errStr string
		var sentAt sql.NullString
		if err := rows.Scan(&id, &to, &subject, &body, &html, &kind, &statusV, &errStr, &createdAt, &sentAt); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		var sentOut any
		if sentAt.Valid && sentAt.String != "" {
			sentOut = sentAt.String
		}
		out = append(out, map[string]any{
			"id": id, "to": to, "subject": subject, "body_text": body, "body_html": html,
			"kind": kind, "status": statusV, "error": errStr, "created_at": createdAt, "sent_at": sentOut,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteList(w, page, pageSize, int(total), out)
}

func (h *handler) retryEmail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"id": "邮件 ID 无效"}))
		return
	}
	var to, subject, body, html, kind string
	err = h.a.DB.QueryRowContext(r.Context(),
		`SELECT to_addr, subject, body_text, body_html, kind FROM mail_outbox WHERE id=?`, id).
		Scan(&to, &subject, &body, &html, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	_, err = h.a.DB.ExecContext(r.Context(), `UPDATE mail_outbox SET status='pending', error='' WHERE id=?`, id)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	sendErr := h.a.Mail.Send(r.Context(), mail.Message{To: to, Subject: subject, Text: body, HTML: html, Kind: kind})
	now := time.Now().UTC().Format(time.RFC3339)
	if sendErr != nil {
		_, _ = h.a.DB.ExecContext(r.Context(), `UPDATE mail_outbox SET status='failed', error=? WHERE id=?`, sendErr.Error(), id)
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "邮件发送失败，请稍后重试").WithCause(sendErr))
		return
	}
	_, _ = h.a.DB.ExecContext(r.Context(), `UPDATE mail_outbox SET status='sent', sent_at=?, error='' WHERE id=?`, now, id)
	h.adminAudit(r, "email.retry", "mail", strconv.FormatInt(id, 10), map[string]any{"kind": kind})
	httpx.WriteData(w, http.StatusOK, nil)
}

// ---- /admin/invite-codes ----

func (h *handler) listInviteCodes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.a.DB.QueryContext(r.Context(), `
		SELECT code, created_at, expires_at, used_at FROM invite_codes ORDER BY created_at DESC, code LIMIT 1000`)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var code, createdAt string
		var expiresAt, usedAt sql.NullString
		if err := rows.Scan(&code, &createdAt, &expiresAt, &usedAt); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		var exp, used any
		if expiresAt.Valid && expiresAt.String != "" {
			exp = expiresAt.String
		}
		if usedAt.Valid && usedAt.String != "" {
			used = usedAt.String
		}
		out = append(out, map[string]any{
			"code": code, "created_at": createdAt, "expires_at": exp, "used_at": used,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	httpx.WriteData(w, http.StatusOK, out)
}

func (h *handler) createInviteCodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count           *int `json:"count"`
		ExpiresInHours  *int `json:"expires_in_hours"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	count := 1
	if req.Count != nil {
		count = *req.Count
	}
	if count < 1 || count > 100 {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"count": "数量需为 1-100"}))
		return
	}
	hours := 168
	hasExpiry := true
	if req.ExpiresInHours != nil {
		hours = *req.ExpiresInHours
		if hours < 0 || hours > 365*24 {
			httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"expires_in_hours": "有效期需为 0-8760 小时（0 表示不过期）"}))
			return
		}
		if hours == 0 {
			hasExpiry = false
		}
	}
	var createdBy any
	if actor, ok := auth.From(r.Context()); ok && actor != nil {
		createdBy = actor.ID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var expires any
	if hasExpiry {
		expires = time.Now().UTC().Add(time.Duration(hours) * time.Hour).Format(time.RFC3339)
	}
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := inviteCode()
		if _, err := h.a.DB.ExecContext(r.Context(),
			`INSERT INTO invite_codes (code, created_by, expires_at, created_at) VALUES (?,?,?,?)`,
			code, createdBy, expires, now); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		codes = append(codes, code)
	}
	h.adminAudit(r, "invite.create", "invite_code", "", map[string]any{"count": count, "expires_in_hours": hours})
	httpx.WriteData(w, http.StatusCreated, map[string]any{"codes": codes})
}

func (h *handler) deleteInviteCode(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		httpx.WriteError(w, h.a.Log, httpx.Validation(map[string]string{"code": "邀请码无效"}))
		return
	}
	res, err := h.a.DB.ExecContext(r.Context(), `DELETE FROM invite_codes WHERE code=?`, code)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	h.adminAudit(r, "invite.delete", "invite_code", code, nil)
	httpx.WriteData(w, http.StatusOK, nil)
}

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// inviteCode 形如 7XKQ-4MHB-9WZT（无歧义大写字符）。
func inviteCode() string {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteAlphabet))))
		if err != nil {
			n = big.NewInt(0)
		}
		b.WriteByte(inviteAlphabet[n.Int64()])
	}
	return b.String()
}
