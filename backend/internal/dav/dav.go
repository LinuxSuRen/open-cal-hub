package dav

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/alexedwards/argon2id"
	"github.com/emersion/go-webdav/caldav"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/cal"
)

// VerifyLogin 校验登录密码（username 或 email）。
// 由 backend-core/user 在接线时替换为其正式实现；默认实现直接查 users 表做 argon2id 校验。
var VerifyLogin = defaultVerifyLogin

// VerifyAppPassword 校验应用密码（username 或 email + 应用密码）。
var VerifyAppPassword = defaultVerifyAppPassword

// Register 挂载 /dav/ CalDAV 服务。
func Register(mux *http.ServeMux, a *app.App) {
	backend := &Backend{DB: a.DB}
	// 上游 caldav.Handler 按去掉 Prefix 后的路径段数推断资源类型
	// （1=principal、2=home set、3=日历、4=对象）。本服务路径布局为
	// /dav/users/{username}/calendars/...，因此 Prefix 需包含 users 段，
	// 否则所有层级都偏移一位：PROPFIND 集合 404、MKCOL 403。
	handler := &caldav.Handler{Backend: backend, Prefix: Prefix + "/users"}
	mux.Handle("/dav/", &server{a: a, backend: backend, inner: handler})
	mux.Handle("/dav", &server{a: a, backend: backend, inner: handler})
}

type server struct {
	a       *app.App
	backend *Backend
	inner   http.Handler
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, err := s.authenticate(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="CalHub CalDAV", charset="UTF-8"`)
		http.Error(w, "CalDAV: unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := WithPrincipal(r.Context(), p)
	// 上游库无法从 Backend 错误映射 404/412（internal.HTTPError 不可导入），
	// 对象级 GET/HEAD/DELETE/OPTIONS 在此预检，保证状态码语义正确。
	if !s.preflight(w, r.WithContext(ctx)) {
		return
	}
	s.inner.ServeHTTP(w, r.WithContext(ctx))
}

// authenticate 支持 Basic（登录密码或应用密码）与 Bearer（测试便利）。
func (s *server) authenticate(r *http.Request) (*Principal, error) {
	ctx := r.Context()
	if user, pass, ok := r.BasicAuth(); ok {
		if p, err := VerifyLogin(ctx, s.a.DB, user, pass); err == nil && p != nil {
			return p, nil
		}
		if p, err := VerifyAppPassword(ctx, s.a.DB, user, pass); err == nil && p != nil {
			return p, nil
		}
		return nil, errUnauthorized
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") && s.a.Signer != nil {
		claims, err := s.a.Signer.Verify(strings.TrimSpace(h[7:]))
		if err != nil {
			return nil, errUnauthorized
		}
		u, err := s.a.User(ctx, claims.UserID)
		if err != nil || u == nil {
			return nil, errUnauthorized
		}
		return &Principal{UserID: u.ID, Username: u.Username}, nil
	}
	return nil, errUnauthorized
}

var errUnauthorized = &davError{code: http.StatusUnauthorized, msg: "unauthorized"}

// preflight 对对象路径做存在性预检；返回 false 表示已直接写出响应。
func (s *server) preflight(w http.ResponseWriter, r *http.Request) bool {
	ref, ok := parseObjectPath(r.URL.Path)
	if !ok {
		return true
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodDelete, http.MethodOptions, "PROPFIND", "REPORT":
	default:
		return true // PUT 等写方法允许不存在
	}
	if _, cerr := s.backend.ownCalendar(r.Context(), ref); cerr != nil {
		writePlainError(w, cerr)
		return false
	}
	rows, err := cal.EventsByUID(r.Context(), s.a.DB, ref.calendarID, ref.uid)
	if err != nil || len(rows) == 0 {
		if r.Method == http.MethodOptions {
			w.Header().Set("DAV", "1, calendar-access")
			w.Header().Set("Allow", "OPTIONS, PUT")
			w.WriteHeader(http.StatusOK)
			return false
		}
		http.Error(w, "CalDAV: not found", http.StatusNotFound)
		return false
	}
	return true
}

func writePlainError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	var de *davError
	if e, ok := err.(*davError); ok {
		de = e
	}
	if de != nil {
		code = de.code
	}
	http.Error(w, "CalDAV: error", code)
}

// defaultVerifyLogin 直接查 users 表校验登录密码（argon2id）。
func defaultVerifyLogin(ctx context.Context, q cal.DBTX, usernameOrEmail, password string) (*Principal, error) {
	var (
		id       int64
		username string
		hash     string
		status   string
	)
	err := q.QueryRowContext(ctx, `SELECT id, username, password_hash, status FROM users
		WHERE (username = ? OR email = ?) LIMIT 1`, usernameOrEmail, usernameOrEmail).
		Scan(&id, &username, &hash, &status)
	if err == sql.ErrNoRows {
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, errUnauthorized
	}
	if ok, err := argon2id.ComparePasswordAndHash(password, hash); err != nil || !ok {
		return nil, errUnauthorized
	}
	return &Principal{UserID: id, Username: username}, nil
}

// defaultVerifyAppPassword 校验应用密码（argon2id 哈希逐条比对）。
func defaultVerifyAppPassword(ctx context.Context, q cal.DBTX, usernameOrEmail, password string) (*Principal, error) {
	var (
		id       int64
		username string
		status   string
	)
	err := q.QueryRowContext(ctx, `SELECT id, username, status FROM users
		WHERE (username = ? OR email = ?) LIMIT 1`, usernameOrEmail, usernameOrEmail).
		Scan(&id, &username, &status)
	if err == sql.ErrNoRows {
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, errUnauthorized
	}
	rows, err := q.QueryContext(ctx, `SELECT id, password_hash FROM app_passwords WHERE user_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		id   int64
		hash string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.hash); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range candidates {
		if ok, err := argon2id.ComparePasswordAndHash(password, c.hash); err == nil && ok {
			_, _ = q.ExecContext(ctx, `UPDATE app_passwords SET last_used_at = ? WHERE id = ?`,
				cal.NowUTCString(), c.id)
			return &Principal{UserID: id, Username: username}, nil
		}
	}
	return nil, errUnauthorized
}
