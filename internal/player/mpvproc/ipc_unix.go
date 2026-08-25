//go:build !windows

package mpvproc

import (
	"io"
	"net"
	"os"
	"os/exec"
	"time"
)

// newIPCPath 生成一个唯一的 Unix socket 路径（供 --input-ipc-server 使用）。
func newIPCPath() (string, error) {
	sock, err := os.CreateTemp("", "unbox-mpv-*.sock")
	if err != nil {
		return "", err
	}
	path := sock.Name()
	_ = sock.Close()
	_ = os.Remove(path)
	return path, nil
}

// dialIPC 以重试方式连接 mpv IPC socket：mpv 启动后创建 socket 有微小延迟。
func dialIPC(path string) (io.ReadWriteCloser, error) {
	deadline := time.Now().Add(ipcConnectTimeout)
	for {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cleanupIPC 删除 Unix socket 文件。
func cleanupIPC(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// setupProcAttr 非 Windows 平台无需隐藏控制台，空实现。
func setupProcAttr(cmd *exec.Cmd) {}
