// calhub 服务入口：加载配置、打开存储、启动 HTTP 服务。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/linuxsuren/open-cal-hub/backend/internal/app"
	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/db"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
	"github.com/linuxsuren/open-cal-hub/backend/internal/server"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（YAML，可选）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load_config_failed", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DB.Path), 0o755); err != nil {
		slog.Error("mkdir_data_failed", "error", err)
		os.Exit(1)
	}
	secret, err := ensureSecret(cfg)
	if err != nil {
		slog.Error("prepare_secret_failed", "error", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	if cfg.Log.Format == "json" {
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	}

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		log.Error("open_db_failed", "path", cfg.DB.Path, "error", err)
		os.Exit(1)
	}
	defer database.Close()

	var sender mail.Sender
	if cfg.Mail.Mode == "smtp" {
		sender = &mail.SMTP{DB: database, Log: log, Host: cfg.Mail.Host, Port: cfg.Mail.Port,
			Username: cfg.Mail.Username, Password: cfg.Mail.Password, From: cfg.Mail.From, TLS: cfg.Mail.TLS}
	} else {
		sender = &mail.Outbox{DB: database}
	}

	a := &app.App{Cfg: cfg, DB: database, Log: log, Mail: sender, Signer: auth.NewSigner(secret, cfg.Auth.AccessTokenTTL)}
	registerRoutes(a) // 各功能包在此挂载（auth/user/admin/cal/booking/dav）

	srv := &http.Server{Addr: cfg.Server.Address, Handler: server.New(a), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("server_listening", "address", cfg.Server.Address, "base_url", cfg.Server.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("serve_failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Info("server_stopped")
}

// ensureSecret 优先使用配置的 JWT 密钥；否则自动生成并持久化到 data_dir/secret.key（0600）。
func ensureSecret(cfg *config.Config) (string, error) {
	if cfg.Auth.JWTSecret != "" {
		return cfg.Auth.JWTSecret, nil
	}
	path := filepath.Join(cfg.Server.DataDir, "secret.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return string(b), nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}
