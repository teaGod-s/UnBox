// Package shell 收敛 Unbox 桌面壳层的全部 Wails 相关 glue。
//
// 它是业务层（internal/player、internal/config、internal/playback）与 Wails v3 之间唯一的接缝：
// 业务层不 import Wails，壳层则通过 player.Player 接口依赖播放能力（Task 5 起）。
// 播放器实例由 cmd/unbox 在启动时用 PickPlayer 选出，再经 NewApp 注入本包，
// 最后透出给前端调用。
package shell

import (
	"context"
	"errors"
	"log"
	"os"
	"runtime"
	"sync"

	assets "github.com/unbox/unbox"
	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/playback"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvplugin"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// testStreamURL 是 M1 冒烟用的一条公开 HLS 测试流（mux test-streams）。
// 选 HLS 而非 MP4：本机网络可达，且更贴近 IPTV 的实际播放形态。
const testStreamURL = "https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8"

// ShellService 是暴露给前端的壳层服务。
// player 为 Task 5 起注入的播放器实例；为 nil 时表示「播放器未就绪」。
// live 为已加载的直播 provider（经 LoadLive 后）；liveSources 为导入时收集、
// 但尚未拉取 m3u 的直播源定义（按需加载）；vods 为点播站点；store 为持久化存储。
type ShellService struct {
	player        player.Player
	store         *store.Store
	live          provider.Provider            // 已加载的直播 provider（LoadLive 后）
	liveSources   []config.Live                // 待按需拉取的直播源定义
	liveCount     int                          // 已加载的直播频道数
	vods          map[string]provider.Provider // 点播站点 key → provider
	vodNames      map[string]string            // 点播站点 key → 显示名
	mu            sync.RWMutex                 // 守护 live/vods 读写
	playback      *playback.Controller
	playbackMu    sync.Mutex // 串行化会实际触发 Load+Play 的播放准备
	playbackSeq   uint64     // 最近一次前端播放请求 token
	playbackToken uint64     // 当前前端播放会话 token，0 表示无会话
	playbackFloor uint64     // 已失效 token 的上限
	mpvPlugin     *mpvplugin.Manager
	vodCFGs       []*config.Config   // 当前点播源解析后的配置（持久化快照）
	liveCFGs      []*config.Config   // 当前直播源解析后的配置（持久化快照）
	liveChannels  []config.Channel   // 当前直播源（播放列表）组装后的频道
	vodRef        string             // 当前点播源地址（回显用）
	liveRef       string             // 当前直播源地址（回显用）
	vodSiteLines  map[string]string  // 点播站点 key → 线路名
	searchCancel  context.CancelFunc // 当前全站搜索的取消函数
	searchSeq     uint64             // 全站搜索请求序号，事件携带此序号隔离过期结果
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
// 代价：WSLg 的 XWayland 路径有「光标不可见」的已知 bug（见 HANDOFF.md「已知限制」），
// 目前无 app 侧解法（切 Wayland 修光标会同时破坏窗口显示与 mpv 嵌入）。
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
	svc := NewShellService(pv, p, st)
	// 恢复上次导入的订阅快照（无网络，纯内存重建）；失败不阻断启动。
	if _, err := svc.RestoreSubscription(); err != nil {
		log.Printf("恢复订阅失败（按未导入处理）: %v", err)
	}
	return application.New(application.Options{
		Name:        "UnBox",
		Description: "UnBox — IPTV 播放器",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets.Frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
}

// OpenWindow 在 app 上创建并打开主窗口。
func OpenWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "UnBox",
		Width:            1000,
		Height:           618,
		MinWidth:         720,
		MinHeight:        480,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})
}
