// Package web 内嵌前端构建产物（frontend 执行 pnpm build 后由脚本拷贝到 web/dist）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist 返回前端静态文件系统。
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
