package mpvproc

import (
	"errors"
	"fmt"
	"os/exec"
)

// windowsExitReasons 把 Windows 上「零输出秒退」的常见退出码翻成可读原因。
//
// 这些故障都发生在加载器或进程启动阶段，mpv 自己来不及向 stderr 写任何东西，
// 于是「缺 DLL」和「进程崩溃」在日志里长得一模一样——退出码是唯一能区分它们
// 的证据，必须带进报错里。
var windowsExitReasons = map[int]string{
	3221225595: "架构不匹配，通常是 32/64 位用错", // 0xC000007B STATUS_INVALID_IMAGE_FORMAT
	3221225477: "进程崩溃（访问冲突）",          // 0xC0000005 STATUS_ACCESS_VIOLATION
	3221225781: "找不到依赖的 DLL",          // 0xC0000135 STATUS_DLL_NOT_FOUND
	3221225785: "DLL 缺少入口点",           // 0xC0000139 STATUS_ENTRYPOINT_NOT_FOUND
	3221225794: "DLL 初始化失败",           // 0xC0000142 STATUS_DLL_INIT_FAILED
}

// describeExit 生成退出码的可读说明。已知码附上十六进制形式，
// 便于用户拿去搜索或对照系统错误表。
func describeExit(code int) string {
	if reason, ok := windowsExitReasons[code]; ok {
		return fmt.Sprintf("退出码 %d (0x%X)：%s", code, code, reason)
	}
	// 未知码只说数字，不编造原因。
	return fmt.Sprintf("退出码 %d", code)
}

// mpvExitError 把「进程提前退出」转成可读错误。取不到退出码（例如被信号终止）
// 时退回不带码的通用说明，仍保持 errors.Is 可判定。
func mpvExitError(waitErr error) error {
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return errMPVExitedEarly
	}
	return fmt.Errorf("%w（%s）", errMPVExitedEarly, describeExit(exitErr.ExitCode()))
}
