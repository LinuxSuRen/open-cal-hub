// Package config 加载 calhub 服务配置：默认值 < YAML 文件 < 环境变量。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  Server  `yaml:"server"`
	DB      DB      `yaml:"db"`
	Auth    Auth    `yaml:"auth"`
	Mail    Mail    `yaml:"mail"`
	Log     Log     `yaml:"log"`
	Metrics Metrics `yaml:"metrics"`
}

type Server struct {
	Address string `yaml:"address"` // 监听地址，默认 :8080
	BaseURL string `yaml:"base_url"`
	DataDir string `yaml:"data_dir"` // 数据目录（DB、生成的密钥等）
}

type DB struct {
	Path string `yaml:"path"` // SQLite 文件路径
}

type Auth struct {
	JWTSecret              string        `yaml:"jwt_secret"` // 留空则自动生成并持久化到 data_dir
	AccessTokenTTL         time.Duration `yaml:"access_ttl"`
	RefreshTokenTTL        time.Duration `yaml:"refresh_ttl"`
	FirstUserAdmin         bool          `yaml:"first_user_admin"`
	RequireEmailValidation bool          `yaml:"require_email_validation"`
	// 初始管理员引导（issue #13）：空库首启时创建 admin。
	// 两项需同时配置；创建后建议改密并移除配置。
	BootstrapAdminEmail    string `yaml:"bootstrap_admin_email"`
	BootstrapAdminPassword string `yaml:"bootstrap_admin_password"`
	// OAuth 登录（issue #2）：client_id/secret 为空则该 provider 不启用。
	OAuth OAuth `yaml:"oauth"`
}

// OAuth 提供方配置；URL 项默认公网端点，测试可用本地地址覆盖。
type OAuth struct {
	GitHub       OAuthProvider `yaml:"github"`
	Google       OAuthProvider `yaml:"google"`
	RedirectBase string        `yaml:"redirect_base"` // 缺省用 server.base_url
}

// OAuthProvider 单个提供方端点。
type OAuthProvider struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	AuthorizeURL string `yaml:"authorize_url"`
	TokenURL     string `yaml:"token_url"`
	UserInfoURL  string `yaml:"userinfo_url"`
	EmailsURL    string `yaml:"emails_url"` // GitHub 专用
}

type Mail struct {
	Mode     string `yaml:"mode"` // outbox | smtp
	From     string `yaml:"from"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	TLS      string `yaml:"tls"` // starttls | tls | none
}

type Log struct {
	Level  string `yaml:"level"` // debug | info | warn | error
	Format string `yaml:"format"`
}

type Metrics struct {
	Enabled bool `yaml:"enabled"`
}

func Default() *Config {
	return &Config{
		Server: Server{Address: ":8080", BaseURL: "http://localhost:8080", DataDir: "data"},
		DB:     DB{Path: ""},
		Auth: Auth{
			AccessTokenTTL:         15 * time.Minute,
			RefreshTokenTTL:        30 * 24 * time.Hour,
			FirstUserAdmin:         true,
			RequireEmailValidation: true,
		},
		Mail:    Mail{Mode: "outbox", From: "calhub@localhost", TLS: "starttls"},
		Log:     Log{Level: "info", Format: "text"},
		Metrics: Metrics{Enabled: true},
	}
}

// Load 依次应用默认值、YAML（若 path 非空）、环境变量（CALHUB_<SECTION>_<KEY>，如 CALHUB_SERVER_ADDRESS）。
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	applyEnv(cfg)
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.DB.Path == "" {
		c.DB.Path = filepath.Join(c.Server.DataDir, "calhub.db")
	}
	c.Server.BaseURL = strings.TrimRight(c.Server.BaseURL, "/")
	if c.Auth.AccessTokenTTL <= 0 {
		c.Auth.AccessTokenTTL = 15 * time.Minute
	}
	if c.Auth.RefreshTokenTTL <= 0 {
		c.Auth.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if c.Mail.Mode != "smtp" {
		c.Mail.Mode = "outbox"
	}
	// OAuth 默认端点（测试可用本地地址覆盖）
	if c.Auth.OAuth.GitHub.AuthorizeURL == "" {
		c.Auth.OAuth.GitHub.AuthorizeURL = "https://github.com/login/oauth/authorize"
	}
	if c.Auth.OAuth.GitHub.TokenURL == "" {
		c.Auth.OAuth.GitHub.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if c.Auth.OAuth.GitHub.UserInfoURL == "" {
		c.Auth.OAuth.GitHub.UserInfoURL = "https://api.github.com/user"
	}
	if c.Auth.OAuth.GitHub.EmailsURL == "" {
		c.Auth.OAuth.GitHub.EmailsURL = "https://api.github.com/user/emails"
	}
	if c.Auth.OAuth.Google.AuthorizeURL == "" {
		c.Auth.OAuth.Google.AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if c.Auth.OAuth.Google.TokenURL == "" {
		c.Auth.OAuth.Google.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if c.Auth.OAuth.Google.UserInfoURL == "" {
		c.Auth.OAuth.Google.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	if c.Auth.OAuth.RedirectBase == "" {
		c.Auth.OAuth.RedirectBase = c.Server.BaseURL
	}
	c.Auth.OAuth.RedirectBase = strings.TrimRight(c.Auth.OAuth.RedirectBase, "/")
}

func applyEnv(c *Config) {
	setStr := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	setDur := func(key string, dst *time.Duration) {
		if v, ok := os.LookupEnv(key); ok {
			if d, err := time.ParseDuration(v); err == nil {
				*dst = d
			}
		}
	}
	setBool := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			}
		}
	}
	setInt := func(key string, dst *int) {
		if v, ok := os.LookupEnv(key); ok {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	setStr("CALHUB_SERVER_ADDRESS", &c.Server.Address)
	setStr("CALHUB_SERVER_BASE_URL", &c.Server.BaseURL)
	setStr("CALHUB_SERVER_DATA_DIR", &c.Server.DataDir)
	setStr("CALHUB_DB_PATH", &c.DB.Path)
	setStr("CALHUB_AUTH_JWT_SECRET", &c.Auth.JWTSecret)
	setDur("CALHUB_AUTH_ACCESS_TTL", &c.Auth.AccessTokenTTL)
	setDur("CALHUB_AUTH_REFRESH_TTL", &c.Auth.RefreshTokenTTL)
	setBool("CALHUB_AUTH_FIRST_USER_ADMIN", &c.Auth.FirstUserAdmin)
	setBool("CALHUB_AUTH_REQUIRE_EMAIL_VALIDATION", &c.Auth.RequireEmailValidation)
	setStr("CALHUB_AUTH_BOOTSTRAP_ADMIN_EMAIL", &c.Auth.BootstrapAdminEmail)
	setStr("CALHUB_AUTH_BOOTSTRAP_ADMIN_PASSWORD", &c.Auth.BootstrapAdminPassword)
	for prov, dst := range map[string]*OAuthProvider{
		"GITHUB": &c.Auth.OAuth.GitHub,
		"GOOGLE": &c.Auth.OAuth.Google,
	} {
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_CLIENT_ID", &dst.ClientID)
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_CLIENT_SECRET", &dst.ClientSecret)
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_AUTHORIZE_URL", &dst.AuthorizeURL)
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_TOKEN_URL", &dst.TokenURL)
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_USERINFO_URL", &dst.UserInfoURL)
		setStr("CALHUB_AUTH_OAUTH_"+prov+"_EMAILS_URL", &dst.EmailsURL)
	}
	setStr("CALHUB_AUTH_OAUTH_REDIRECT_BASE", &c.Auth.OAuth.RedirectBase)
	setStr("CALHUB_MAIL_MODE", &c.Mail.Mode)
	setStr("CALHUB_MAIL_FROM", &c.Mail.From)
	setStr("CALHUB_MAIL_HOST", &c.Mail.Host)
	setInt("CALHUB_MAIL_PORT", &c.Mail.Port)
	setStr("CALHUB_MAIL_USERNAME", &c.Mail.Username)
	setStr("CALHUB_MAIL_PASSWORD", &c.Mail.Password)
	setStr("CALHUB_MAIL_TLS", &c.Mail.TLS)
	setStr("CALHUB_LOG_LEVEL", &c.Log.Level)
	setStr("CALHUB_LOG_FORMAT", &c.Log.Format)
}
