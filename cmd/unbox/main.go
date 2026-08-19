// Command unbox 是 Unbox 的桌面主程序入口。
//
// 它只负责把「解析配置 → 创建 app → 开窗 → Run」四步串起来；
// 应用与窗口的 Wails 选项细节收敛在 internal/shell。
package main

import (
	"log"

	"github.com/unbox/unbox/internal/shell"
)

func main() {
	// 1. 解析配置：M1 阶段无持久化配置，此处占位留空（Plan 3 接入真实源列表）。
	// 2. 创建 app。
	app := shell.NewApp()
	// 3. 开窗。
	shell.OpenWindow(app)
	// 4. 进入事件循环（阻塞直至退出）。
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
