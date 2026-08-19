//go:build !production

// Package assets 在非 production 构建中提供一个空的前端资源容器。
//
// 开发模式下 Wails 通过 FRONTEND_DEVSERVER_URL 从 Vite dev server 加载页面，
// 不需要嵌入资源；普通 go build / go test 也因此不依赖 frontend/dist 已存在，
// 避免「先跑 npm build 才能 go build ./...」的构建顺序耦合。
package assets

import "embed"

// Frontend 在非 production 构建中为空；页面由 Vite dev server 提供。
var Frontend embed.FS
