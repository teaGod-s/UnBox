package shell

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvplugin"
)

// mpvManager 是 PickPlayer 用到的能力面，便于测试替换成桩。
type mpvManager interface {
	Status() mpvplugin.Status
	NewPlayer() (player.Player, error)
}

// newMPVManager 是 Manager 构造的可注入替身，供测试覆盖各分支。
var newMPVManager = func() mpvManager {
	root, _ := os.UserConfigDir()
	return mpvplugin.New(runtime.GOOS, root)
}

// PickPlayer 返回当前平台应使用的播放器实例；不 import Wails，可被 go test 直接测。
//
// 选路完全交给 mpvplugin：应用内嵌 → 插件目录 → 系统 PATH。内嵌优先是有意的——
// NSIS 安装包自带可用的 mpv，若让 PATH 上残留的旧 mpv 抢先，用户会在毫无察觉的
// 情况下用到一个坏掉的播放器。此前这里自己先 lookPath("mpv")，既反转了这个顺序，
// 又绕过了 NewPlayer 里的 mpv --version 预检。
//
// mpv 缺失或不可用时返回明确错误而非 panic，调用方（cmd/unbox）据此刻画
// 「播放器未就绪」。
func PickPlayer() (player.Player, error) {
	manager := newMPVManager()
	status := manager.Status()
	if !status.Available {
		return nil, fmt.Errorf("未找到 mpv 可执行文件")
	}

	// 出问题时第一件要知道的事是「用到了哪个 mpv」，进日志缓冲供「查看日志」回看。
	log.Printf("mpv 播放器: %s", status.Path)

	return manager.NewPlayer()
}
