// OAuth 登录（issue #2，GitHub + Google 优先）：
// Authorization Code + state（HMAC 签名 + Cookie 双提交，15 分钟）。
// 身份模型：oauth_identities(provider, provider_uid) → user；同一外部身份
// 重复登录幂等；未绑定时按已验证邮箱关联本地账号或自动注册（invite 模式
// 下仅登录不自动注册）。
package user

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
)

const (
	oauthStateCookie = "calhub_oauth_state"
	oauthStateTTL    = 15 * time.Minute
)

// oauthProviders 返回已启用的 provider（client_id/secret 齐备）配置。
func oauthProviders(c *config.Config) map[string]config.OAuthProvider {
	out := map[string]config.OAuthProvider{}
	if p := c.Auth.OAuth.GitHub; p.ClientID != "" && p.ClientSecret != "" {
		out["github"] = p
	}
	if p := c.Auth.OAuth.Google; p.ClientID != "" && p.ClientSecret != "" {
		out["google"] = p
	}
	return out
}

// oauthScope 各 provider 的授权范围。
func oauthScope(provider string) string {
	if provider == "github" {
		return "read:user user:email"
	}
	return "openid email profile"
}

// ---- state（防 CSRF）：payload.signature，签名密钥复用 JWT 密钥 ----

type oauthStatePayload struct {
	Nonce   string `json:"n"`
	Expires int64  `json:"e"`
	Redirect string `json:"r"`
}

func signOAuthState(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func encodeOAuthState(secret, redirect string) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	p := oauthStatePayload{
		Nonce:   hex.EncodeToString(b),
		Expires: time.Now().Add(oauthStateTTL).Unix(),
		Redirect: redirect,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + signOAuthState(secret, payload), nil
}

// verifyOAuthState 校验签名与有效期，返回 payload。
func verifyOAuthState(secret, state string) (*oauthStatePayload, error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed state")
	}
	if !hmac.Equal([]byte(signOAuthState(secret, parts[0])), []byte(parts[1])) {
		return nil, errors.New("bad state signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("bad state payload")
	}
	var p oauthStatePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if time.Now().Unix() > p.Expires {
		return nil, errors.New("state expired")
	}
	return &p, nil
}

// safeRedirect 仅允许站内相对路径（防 open redirect）。
func safeRedirect(r string) string {
	if r == "" || !strings.HasPrefix(r, "/") || strings.HasPrefix(r, "//") || strings.Contains(r, "\r") || strings.Contains(r, "\n") {
		return "/"
	}
	return r
}

// ---- HTTP 端点 ----

// oauthAuthorize GET /api/v1/auth/oauth/{provider}/authorize?redirect_to=
func (h *handler) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	p, ok := oauthProviders(h.a.Cfg)[provider]
	if !ok {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	redirect := safeRedirect(r.URL.Query().Get("redirect_to"))
	state, err := encodeOAuthState(h.a.Cfg.Auth.JWTSecret, redirect)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/api/v1/auth/oauth",
		MaxAge: int(oauthStateTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{}
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", h.redirectURI(provider))
	q.Set("state", state)
	q.Set("scope", oauthScope(provider))
	if provider == "google" {
		q.Set("response_type", "code")
		q.Set("access_type", "online")
	}
	http.Redirect(w, r, p.AuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

func (h *handler) redirectURI(provider string) string {
	return h.a.Cfg.Auth.OAuth.RedirectBase + "/api/v1/auth/oauth/" + provider + "/callback"
}

// oauthCallback GET /api/v1/auth/oauth/{provider}/callback?code&state
func (h *handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	p, ok := oauthProviders(h.a.Cfg)[provider]
	if !ok {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	// state：query 与 Cookie 双提交一致
	qState := r.URL.Query().Get("state")
	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || cookie.Value == "" || cookie.Value != qState {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "登录状态校验失败，请重试"))
		return
	}
	payload, err := verifyOAuthState(h.a.Cfg.Auth.JWTSecret, qState)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "登录状态校验失败，请重试").WithCause(err))
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.WriteError(w, h.a.Log, httpx.E("auth/invalid_token", "授权码缺失"))
		return
	}

	profile, err := fetchOAuthProfile(r.Context(), provider, p, h.redirectURI(provider), code)
	if err != nil {
		h.securityAudit(r, 0, "auth.oauth_failed", "oauth", provider, map[string]any{"reason": "profile", "error": err.Error()})
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "第三方登录失败，请重试").WithCause(err))
		return
	}

	u, created, err := h.resolveOAuthUser(r, provider, profile)
	if err != nil {
		httpx.WriteError(w, h.a.Log, err)
		return
	}
	if created {
		h.securityAudit(r, u.ID, "user.oauth_register", "user", strconv.FormatInt(u.ID, 10), map[string]any{
			"provider": provider, "email": profile.Email,
		})
	}
	h.securityAudit(r, u.ID, "auth.oauth_login", "oauth", provider, map[string]any{"provider_uid": profile.UID})
	h.issueSession(w, r, u)
	// 前端可通过响应中的 user/access_token 完成后续跳转；redirect 仅提示回跳目标
	_ = payload.Redirect
}

// oauthProfile 是归一化后的第三方身份。
type oauthProfile struct {
	UID         string
	Email       string
	EmailVerified bool
	Name        string
}

// fetchOAuthProfile 完成 code→token→userinfo。
func fetchOAuthProfile(ctx context.Context, provider string, p config.OAuthProvider, redirectURI, code string) (*oauthProfile, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	// 1) code 换 access_token
	tokReq := url.Values{}
	tokReq.Set("client_id", p.ClientID)
	tokReq.Set("client_secret", p.ClientSecret)
	tokReq.Set("code", code)
	tokReq.Set("redirect_uri", redirectURI)
	if provider == "google" {
		tokReq.Set("grant_type", "authorization_code")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(tokReq.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange: %d %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return nil, fmt.Errorf("token exchange: no access_token (%s)", string(body))
	}

	// 2) userinfo
	get := func(u string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("userinfo %s: %d %s", u, resp.StatusCode, string(b))
		}
		return b, nil
	}

	switch provider {
	case "github":
		userBody, err := get(p.UserInfoURL)
		if err != nil {
			return nil, err
		}
		var gu struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Name  string `json:"name"`
		}
		if err := json.Unmarshal(userBody, &gu); err != nil {
			return nil, err
		}
		// 邮箱：显式拉 /user/emails 取 primary+verified（可能是私密地址）
		emailsBody, err := get(p.EmailsURL)
		if err != nil {
			return nil, err
		}
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := json.Unmarshal(emailsBody, &emails); err != nil {
			return nil, err
		}
		prof := &oauthProfile{UID: strconv.FormatInt(gu.ID, 10), Name: orDefault(gu.Name, gu.Login)}
		for _, e := range emails {
			if e.Primary && e.Verified {
				prof.Email, prof.EmailVerified = strings.ToLower(e.Email), true
				break
			}
		}
		if prof.Email == "" {
			return nil, errors.New("github: no primary verified email")
		}
		return prof, nil
	case "google":
		userBody, err := get(p.UserInfoURL)
		if err != nil {
			return nil, err
		}
		var gu struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
		}
		if err := json.Unmarshal(userBody, &gu); err != nil {
			return nil, err
		}
		if gu.Sub == "" || gu.Email == "" || !gu.EmailVerified {
			return nil, errors.New("google: email not verified")
		}
		return &oauthProfile{
			UID: gu.Sub, Email: strings.ToLower(gu.Email), EmailVerified: true, Name: gu.Name,
		}, nil
	}
	return nil, fmt.Errorf("unsupported provider %q", provider)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// resolveOAuthUser 身份解析：已绑定→登录；未绑定→按邮箱关联本地账号（补验证）
// 或自动注册（invite 模式拒绝）；返回用户与是否新建。
func (h *handler) resolveOAuthUser(r *http.Request, provider string, profile *oauthProfile) (*User, bool, error) {
	ctx := r.Context()
	// 1) 已绑定
	var userID int64
	err := h.a.DB.QueryRowContext(ctx,
		`SELECT user_id FROM oauth_identities WHERE provider=? AND provider_uid=?`, provider, profile.UID).
		Scan(&userID)
	if err == nil {
		u, uerr := UserByID(ctx, h.a.DB, userID)
		if uerr != nil {
			return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(uerr)
		}
		if u.Status != "active" {
			return nil, false, httpx.E("auth/account_disabled", "账号已被停用")
		}
		return u, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}

	// 2) 按邮箱关联本地账号：邮箱已被提供方验证，视为完成验证
	if u, err := UserByEmail(ctx, h.a.DB, profile.Email); err == nil {
		if u.Status == "deleted" {
			return nil, false, httpx.E("auth/invalid_credentials", "账号不存在")
		}
		if u.Status != "active" {
			return nil, false, httpx.E("auth/account_disabled", "账号已被停用")
		}
		if !u.EmailVerifiedAt.Valid || u.EmailVerifiedAt.String == "" {
			if _, err := h.a.DB.ExecContext(ctx, `UPDATE users SET email_verified_at=?, updated_at=? WHERE id=?`,
				nowUTC(), nowUTC(), u.ID); err != nil {
				return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
			}
		}
		if err := h.bindOAuthIdentity(ctx, u.ID, provider, profile); err != nil {
			return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
		}
		return u, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}

	// 3) 自动注册：invite 模式拒绝
	if mode, _ := h.a.EffectiveRegistrationMode(ctx); mode == "invite" {
		return nil, false, httpx.E("auth/invite_required", "当前仅支持邀请注册，请先用邮箱注册并绑定")
	}
	username, err := GenerateUsername(ctx, h.a.DB, profile.Email)
	if err != nil {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}
	role := "user"
	if h.a.Cfg.Auth.FirstUserAdmin {
		if n, err := CountUsers(ctx, h.a.DB); err == nil && n == 0 {
			role = "admin"
		}
	}
	display := orDefault(profile.Name, username)
	if len(display) > 100 {
		display = display[:100]
	}
	// 随机密码哈希：OAuth 账号不可用密码登录（可走忘记密码设置新密码）
	randomHash, _ := auth.HashPassword(newRandomToken(24))
	u := &User{
		Email: profile.Email, Username: username, PasswordHash: randomHash,
		DisplayName: display, Timezone: "UTC", Locale: "zh-CN",
		Role: role, Status: "active",
	}
	if err := CreateUser(ctx, h.a.DB, u); err != nil {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}
	if _, err := h.a.DB.ExecContext(ctx, `UPDATE users SET email_verified_at=? WHERE id=?`, nowUTC(), u.ID); err != nil {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}
	u.EmailVerifiedAt = sql.NullString{String: nowUTC(), Valid: true}
	if h.a.OnUserRegistered != nil {
		if err := h.a.OnUserRegistered(ctx, u.ID, u.Timezone); err != nil {
			h.a.Log.Error("oauth_on_user_registered_failed", "user_id", u.ID, "error", err)
		}
	}
	if err := h.bindOAuthIdentity(ctx, u.ID, provider, profile); err != nil {
		return nil, false, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err)
	}
	return u, true, nil
}

func (h *handler) bindOAuthIdentity(ctx context.Context, userID int64, provider string, profile *oauthProfile) error {
	_, err := h.a.DB.ExecContext(ctx,
		`INSERT OR IGNORE INTO oauth_identities (user_id, provider, provider_uid, email_at_link, created_at) VALUES (?,?,?,?,?)`,
		userID, provider, profile.UID, profile.Email, nowUTC())
	return err
}

func newRandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- 绑定管理（登录态） ----

// listOAuthIdentities GET /api/v1/me/oauth
func (h *handler) listOAuthIdentities(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.From(r.Context())
	if !ok || u == nil {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	rows, err := h.a.DB.QueryContext(r.Context(),
		`SELECT provider, provider_uid, email_at_link, created_at FROM oauth_identities WHERE user_id=? ORDER BY id`, u.ID)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var provider, uid, emailAtLink, createdAt string
		if err := rows.Scan(&provider, &uid, &emailAtLink, &createdAt); err != nil {
			httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
			return
		}
		out = append(out, map[string]any{
			"provider": provider, "provider_uid": uid, "email_at_link": emailAtLink, "created_at": createdAt,
		})
	}
	httpx.WriteData(w, http.StatusOK, out)
}

// unbindOAuth DELETE /api/v1/me/oauth/{provider}
func (h *handler) unbindOAuth(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.From(r.Context())
	if !ok || u == nil {
		httpx.WriteError(w, h.a.Log, httpx.Unauthorized())
		return
	}
	provider := r.PathValue("provider")
	res, err := h.a.DB.ExecContext(r.Context(),
		`DELETE FROM oauth_identities WHERE user_id=? AND provider=?`, u.ID, provider)
	if err != nil {
		httpx.WriteError(w, h.a.Log, httpx.E("common/internal", "服务暂时不可用，请稍后重试").WithCause(err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, h.a.Log, httpx.NotFound())
		return
	}
	httpx.WriteData(w, http.StatusOK, nil)
}
