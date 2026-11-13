//go:build tools

// Package tools 固定构建期依赖：这些模块由其他成员的包（cal/booking/dav）使用，
// 在它们的代码接线前，通过此文件防止 go mod tidy 剪掉版本锁定。
// （alexedwards/argon2id 已由 internal/auth 实际 import，此处一并声明便于集中管理。）
package tools

import (
	_ "github.com/alexedwards/argon2id"
	_ "github.com/emersion/go-ical"
	_ "github.com/emersion/go-webdav"
	_ "github.com/teambition/rrule-go"
)
