// Command unbox 是 Unbox 的桌面主程序入口。
//
// 它只负责把「选出播放器 → 创建 app → 开窗 → Run」四步串起来；
// 应用与窗口的 Wails 选项细节收敛在 internal/shell。
package main

import (
	"log"

	"github.com/unbox/unbox/internal/shell"
)

func main() {
	// 1. 选出播放器：mpv 缺失时记录错误并以 nil 继续（M1 无 mpv 时壳仍能开窗，
	//    前端显示「播放器未就绪」）。
	p, err := shell.PickPlayer()
	if err != nil {
		log.Printf("播放器初始化失败（继续以未就绪状态启动）: %v", err)
	}
	// 2. 创建 app。
	app := shell.NewApp(p)
	// 3. 开窗。
	shell.OpenWindow(app)
	// 4. 进入事件循环（阻塞直至退出）。
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
