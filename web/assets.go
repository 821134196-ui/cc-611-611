// Package webfs 把前端构建产物 web/dist 嵌入 Go 二进制，
// 使发布时只需一个可执行文件。
package webfs

import "embed"

//go:embed all:dist
var Dist embed.FS
