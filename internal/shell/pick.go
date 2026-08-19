package shell

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvlib"
	"github.com/unbox/unbox/internal/player/mpvproc"
)

// lookPath 是 exec.LookPath 的可注入替身，供测试覆盖「mpv 缺失」分支。
var lookPath = exec.LookPath

// PickPlayer 返回当前平台应使用的播放器实例；不 import Wails，可被 go test 直接测。
//
// macOS 走 libmpv 后端（mpvlib），其余平台通过 mpvproc 驱动外部 mpv 子进程。
// mpv 缺失时返回明确错误而非 panic，调用方（cmd/unbox）据此刻画「播放器未就绪」。
func PickPlayer() (player.Player, error) {
	if runtime.GOOS == "darwin" {
		return mpvlib.New()
	}
	exe, err := lookPath("mpv")
	if err != nil {
		return nil, fmt.Errorf("未找到 mpv 可执行文件: %w", err)
	}
	return mpvproc.New(exe)
}
