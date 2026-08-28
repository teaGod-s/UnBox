// Command unbox 是 Unbox 的桌面主程序入口。
//
// 它只负责把「选出播放器 → 注入依赖 → 创建 app → 开窗 → Run」串起来；
// 应用与窗口的 Wails 选项细节收敛在 internal/shell。
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/unbox/unbox/internal/player/failover"
	"github.com/unbox/unbox/internal/probe"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/shell"
	"github.com/unbox/unbox/internal/store"
)

func main() {
	shell.InitLogging()
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
	shell.OpenWindow(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
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
