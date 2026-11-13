// Package app 装配共享依赖，供各功能包注册路由使用。
package app

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/linuxsuren/open-cal-hub/backend/internal/auth"
	"github.com/linuxsuren/open-cal-hub/backend/internal/config"
	"github.com/linuxsuren/open-cal-hub/backend/internal/mail"
)

type App struct {
	Cfg    *config.Config
	DB     *sql.DB
	Log    *slog.Logger
	Mail   mail.Sender
	Signer *auth.Signer
	// LoadUser 由 user 包在注册时注入（避免循环依赖）。
	LoadUser auth.UserLoader
}

// User 实现按 ID 加载活跃用户（user 包注册真实实现前的基础占位，返回 nil 表示不存在）。
func (a *App) User(ctx context.Context, id int64) (*auth.User, error) {
	if a.LoadUser != nil {
		return a.LoadUser(ctx, id)
	}
	return nil, sql.ErrNoRows
}
