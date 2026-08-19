//go:build production

// Package assets 在 production 构建中嵌入编译后的前端资源（frontend/dist），
// 使 unbox 二进制可独立分发，无需在运行时依赖外部目录。
//
// 本包只包含 Go 标准库的 embed 指令，不 import Wails；它放在模块根目录是因为
// go:embed 的路径相对于所在 .go 文件，只有根目录下的文件才能以
// "frontend/dist" 的相对路径命中仓库根的前端产物。
package assets

import "embed"

// Frontend 是编译进二进制的全部前端静态资源。
//
//go:embed all:frontend/dist
var Frontend embed.FS
