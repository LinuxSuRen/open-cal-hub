// Package version 单一版本源；发布流程打 tag 时通过 -ldflags -X 注入。
package version

// Version 为当前构建的语义化版本。用 var 而非 const，
// 以便发布流水线注入：-ldflags "-X github.com/linuxsuren/open-cal-hub/backend/internal/version.Version=<tag>"
var Version = "1.0.0"
