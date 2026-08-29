package shell

import (
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
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

// internalVersion 返回内部构建版本标识。
// 优先用 debug.ReadBuildInfo 的 Main.Version（go install pkg@version 时为真实版本）；
// 本地/CI 直接 go build 时 Main.Version 恒为 "(devel)"，此时改用 VCS 信息
// （vcs.time + vcs.revision，需 -buildvcs=true）拼成伪版本
// 形如 v0.0.0-20260812151221-8f53fd3ee25c，便于区分不同构建。
func internalVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		var rev, ts string
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.time":
				ts = s.Value
			}
		}
		if rev != "" && ts != "" {
			// ts 形如 2026-08-12T15:12:21Z → 压缩为 20260812151221（取前 14 位）
			compact := strings.NewReplacer("-", "", ":", "", "T", "", "Z", "").Replace(ts)
			if len(compact) > 14 {
				compact = compact[:14]
			}
			if len(rev) > 12 {
				rev = rev[:12]
			}
			return "v0.0.0-" + compact + "-" + rev
		}
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
