// Package shell 收敛 Unbox 桌面壳层的全部 Wails 相关 glue。
//
// 它是业务层（internal/player、internal/config）与 Wails v3 之间唯一的接缝：
// 业务层不 import Wails，壳层则通过 player.Player 接口依赖播放能力（Task 5 起）。
// 播放器实例由 cmd/unbox 在启动时用 PickPlayer 选出，再经 NewApp 注入本包，
// 最后透出给前端调用。
package shell

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"

	assets "github.com/unbox/unbox"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// testStreamURL 是 M1 冒烟用的一条公开 HLS 测试流（mux test-streams）。
// 选 HLS 而非 MP4：本机网络可达，且更贴近 IPTV 的实际播放形态。
const testStreamURL = "https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8"

// ShellService 是暴露给前端的壳层服务。
// player 为 Task 5 起注入的播放器实例；为 nil 时表示「播放器未就绪」。
// provider 为当前内容来源（初始为空，待前端 ImportSubscription 后重建）；
// store 为收藏/最近/分组的持久化存储。
type ShellService struct {
	player   player.Player
	provider provider.Provider
	store    *store.Store
	mu       sync.RWMutex // 守护 provider 读写
}

// Platform 返回当前运行平台（linux / darwin / windows）。
func (s *ShellService) Platform() string {
	return runtime.GOOS
}

// PlayerReady 报告播放器是否就绪。
func (s *ShellService) PlayerReady() bool {
	return s.player != nil
}

// LoadTestStream 加载一条公开测试流，打通「启动 → 加载 → mpv 出画面」的最小闭环。
// 播放器未就绪时返回明确错误（Wails 会在前端侧以异常形式抛出）。
func (s *ShellService) LoadTestStream() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Load(context.Background(), player.Stream{
		URL:  testStreamURL,
		Kind: player.StreamHLS,
	})
}

// forceLinuxX11Backend 在 Linux 下把 GDK_BACKEND 设为 x11，必须在 GTK 初始化前调用。
// GTK4 默认优先 Wayland 后端；WSLg 等同时暴露 Wayland + XWayland 的环境下，
// Wayland 后端会让 WebKit 窗口拿不到渲染上下文（不显示），且 mpvproc 的
// --wid 嵌入需要 X11 的 XID。X11（XWayland）后端才同时满足两者。
func forceLinuxX11Backend() {
	if runtime.GOOS == "linux" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}
}

// NewApp 创建 Unbox 桌面应用，应用级 Wails 选项细节全部收敛于此。
// p 为启动时选出的播放器实例（可为 nil，表示未就绪）；pv 为内容来源
// （可为 nil，表示尚未导入订阅）；st 为持久化存储（可为 nil，表示收藏不可用）。
func NewApp(p player.Player, pv provider.Provider, st *store.Store) *application.App {
	forceLinuxX11Backend()
	return application.New(application.Options{
		Name:        "unbox",
		Description: "Unbox — IPTV 播放器",
		Services: []application.Service{
			application.NewService(NewShellService(pv, p, st)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets.Frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
}

// OpenWindow 在 app 上创建并打开主窗口，并返回窗口（供后续拿原生句柄嵌入播放）。
func OpenWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Unbox",
		Width:            1000,
		Height:           618,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})
}
