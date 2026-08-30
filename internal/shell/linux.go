package shell

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// lookPathUnshare / runUnshareProbe 是探测非特权用户命名空间的可注入替身，
// 供测试覆盖「userns 被禁 / unshare 缺失」两个分支（与 pick.go 的 lookPath 同风格）。
var (
	lookPathUnshare = exec.LookPath
	runUnshareProbe = func() error {
		out, err := exec.Command("unshare", "-U", "true").CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
)

// checkUserNamespaces 探测非特权用户命名空间（unprivileged userns）是否可用。
//
// WebKitGTK 的 web 进程沙箱依赖 bwrap（bubblewrap），而 bwrap 需要 unprivileged
// userns 才能建立沙箱。Ubuntu 24.04+ 默认 kernel.apparmor_restrict_unprivileged_userns=1，
// 会拦掉 bwrap 的 userns 创建，最终表现为「bwrap: setting up uid map: Permission denied」
// → webview 在 cgo 里 SIGTRAP 裸崩溃（详见 HANDOFF.md「已知限制」）。
// 故启动前先探测，失败时给出可操作指引，而不是让 WebKit 启动到一半才崩。
//
// 返回 nil 表示可用（或无法判断，放行）；返回 error 表示环境不满足、应停止启动。
func checkUserNamespaces() error {
	if _, err := lookPathUnshare("unshare"); err != nil {
		// unshare 缺失（极罕见），无法探测，放行以免误拦。
		return nil
	}
	if err := runUnshareProbe(); err != nil {
		return fmt.Errorf(`检测到当前 Linux 禁用了非特权用户命名空间（unprivileged user namespaces），
UnBox 的 WebKit 沙箱无法启动，会在打开窗口时崩溃。

请先执行以下任一命令后重试（无需重启系统）：

  Ubuntu 24.04+：
    sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
  （持久化：echo 'kernel.apparmor_restrict_unprivileged_userns=0' | sudo tee /etc/sysctl.d/99-unbox.conf）

  旧内核（Ubuntu 22.04 等）：
    sudo sysctl -w kernel.unprivileged_userns_clone=1

改完直接重跑 unbox 即可。底层错误: %v`, err)
	}
	return nil
}

// CheckLinuxPrerequisites 在 Linux 上启动前检查 WebKit webview 的环境前置条件。
// 非 Linux 平台直接放行。返回 error 表示环境不满足、应停止启动并提示用户。
func CheckLinuxPrerequisites() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return checkUserNamespaces()
}
