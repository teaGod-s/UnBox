// Command unbox 是 Unbox 的桌面主程序入口。
//
// 它只负责把「选出播放器 → 注入依赖 → 创建 app → 开窗 → Run」串起来；
// 应用与窗口的 Wails 选项细节收敛在 internal/shell。
package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/failover"
	"github.com/unbox/unbox/internal/probe"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/shell"
	"github.com/unbox/unbox/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	p, err := shell.PickPlayer()
	if err != nil {
		log.Printf("播放器初始化失败（继续以未就绪状态启动）: %v", err)
	}
	pl := p // player.Player，可能 nil
	if p != nil {
		pl = failover.New(p, probe.NewProber())
	}
	st, serr := store.Open(appDataPath())
	if serr != nil {
		log.Printf("数据库初始化失败（收藏/最近不可用）: %v", serr)
	}
	// 初始 Provider 为空：等待前端 ImportSubscription 后重建。
	var pv provider.Provider
	app := shell.NewApp(pl, pv, st)
	win := shell.OpenWindow(app)

	// 窗口显示后拿原生句柄注入 mpvproc，实现嵌入；拿不到则回退独立窗口。
	if embed, ok := p.(player.Embedder); ok {
		go embedWindow(embed, win)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// embedWindow 轮询等待窗口 realize 后拿到 XID/HWND 并注入；超时则回退。
func embedWindow(embed player.Embedder, win *application.WebviewWindow) {
	for i := 0; i < 100; i++ {
		if id := shell.NativeWindowID(win); id != 0 {
			embed.SetEmbedWindow(id)
			log.Printf("已启用窗口嵌入 (id=%d)", id)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("未能在超时内获取原生窗口句柄，回退为独立窗口播放")
}

// appDataPath 返回数据库存放路径（用户配置目录下的 unbox/unbox.db）。
func appDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	p := filepath.Join(dir, "unbox")
	_ = os.MkdirAll(p, 0o755)
	return filepath.Join(p, "unbox.db")
}
