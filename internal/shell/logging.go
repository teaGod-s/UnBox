package shell

import (
	"io"
	"log"
	"os"
	"runtime/debug"
	"sync"
)

// logBuffer 是有界环形日志缓冲，供设置页「查看日志」使用。
type logBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newLogBuffer(max int) *logBuffer { return &logBuffer{max: max} }

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// appLogs 是全局日志缓冲（最近 64KB）。
var appLogs = newLogBuffer(64 << 10)

// internalVersion 返回内部构建版本（debug.ReadBuildInfo 的 Main.Version）。
// 本地/CI 直接 go build 时通常为 "(devel)"；用 go install pkg@version 或
// 启用 -buildvcs 打 tag 构建时会得到真实版本号。
func internalVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

// InitLogging 把标准 log 重定向到 stderr + 内存环形缓冲，并在每行前加上内部版本。
// 须在 main 启动最早处调用，保证后续所有 log 输出都被捕获。
func InitLogging() {
	log.SetOutput(io.MultiWriter(os.Stderr, appLogs))
	log.SetPrefix("[" + internalVersion() + "] ")
}

// GetLogs 返回最近捕获的日志（供设置页展示）。
func (s *ShellService) GetLogs() string {
	return appLogs.String()
}
