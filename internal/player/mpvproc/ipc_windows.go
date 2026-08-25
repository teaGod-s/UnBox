//go:build windows

package mpvproc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// pipeSeq 保证同进程内命名管道名唯一。
var pipeSeq atomic.Uint64

// newIPCPath 返回一个唯一的命名管道基础名（不带 \\.\pipe\ 前缀）。mpv 的
// --input-ipc-server 在 Windows 上创建命名管道，并自行加 \\.\pipe\ 前缀。
func newIPCPath() (string, error) {
	return fmt.Sprintf("unbox-mpv-%d", pipeSeq.Add(1)), nil
}

// dialIPC 以重试方式连接 mpv 的命名管道：mpv 启动后创建管道有微小延迟。
// 客户端端用 os.OpenFile 打开 \\.\pipe\<name>（等价 CreateFile 的 client 端）。
func dialIPC(path string) (io.ReadWriteCloser, error) {
	pipe := `\\.\pipe\` + path
	deadline := time.Now().Add(ipcConnectTimeout)
	for {
		f, err := os.OpenFile(pipe, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cleanupIPC 命名管道随 mpv 退出自动销毁，无需显式删除。
func cleanupIPC(path string) {}

// setupProcAttr 隐藏 mpv 子进程的控制台窗口（mpv.exe 是控制台程序，否则从
// GUI 应用启动会弹出一个黑色终端）。CREATE_NO_WINDOW 禁止创建控制台。
func setupProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
