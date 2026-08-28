package shell

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvplugin"
	"github.com/unbox/unbox/internal/player/mpvproc"
)

// lookPath 是 exec.LookPath 的可注入替身，供测试覆盖「mpv 缺失」分支。
var lookPath = exec.LookPath
var pluginStatus = func() mpvplugin.Status {
	root, _ := os.UserConfigDir()
	return mpvplugin.New(runtime.GOOS, root).Status()
}

// PickPlayer 返回当前平台应使用的播放器实例；不 import Wails，可被 go test 直接测。
//
// 所有平台都通过 mpvproc 驱动外部 mpv 子进程。
// mpv 缺失时返回明确错误而非 panic，调用方（cmd/unbox）据此刻画「播放器未就绪」。
func PickPlayer() (player.Player, error) {
	exe, err := lookPath("mpv")
	if err == nil {
		return mpvproc.New(exe)
	}
	status := pluginStatus()
	if status.Available {
		return mpvproc.New(status.Path)
	}
	return nil, fmt.Errorf("未找到 mpv 可执行文件: %w", err)
}
