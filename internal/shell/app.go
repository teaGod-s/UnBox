// Package shell 收敛 Unbox 桌面壳层的全部 Wails 相关 glue。
//
// 它是业务层（internal/player、internal/config）与 Wails v3 之间唯一的接缝：
// 业务层不 import Wails，壳层也不 import 业务层。Task 5 起通过 player.Player
// 接口把播放器注入本包，再透出给前端。
package shell

import (
	"runtime"

	assets "github.com/unbox/unbox"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ShellService 是暴露给前端的壳层服务。
// Task 5 起在此注入 Player 接口，把播放能力透出给 UI。
type ShellService struct{}

// Platform 返回当前运行平台（linux / darwin / windows）。
func (s *ShellService) Platform() string {
	return runtime.GOOS
}

// PlayerReady 报告播放器是否就绪。
// M1 阶段播放器接线在 Task 5 完成，此占位恒返回 false。
func (s *ShellService) PlayerReady() bool {
	return false
}

// NewApp 创建 Unbox 桌面应用，应用级 Wails 选项细节全部收敛于此。
func NewApp() *application.App {
	return application.New(application.Options{
		Name:        "unbox",
		Description: "Unbox — IPTV 播放器",
		Services: []application.Service{
			application.NewService(&ShellService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets.Frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
}

// OpenWindow 在 app 上创建并打开主窗口，窗口级 Wails 选项细节收敛于此。
func OpenWindow(app *application.App) {
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Unbox",
		Width:            1000,
		Height:           618,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})
}
