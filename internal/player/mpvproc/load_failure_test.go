package mpvproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// fakeMPVEnv 让测试二进制把自己当成「坏掉的 mpv」：打印一行 stderr 后立刻退出，
// 复现缺 DLL / 被杀软拦截时的真实表现——进程建得起来，但 IPC 管道永远不出现。
const fakeMPVEnv = "UNBOX_FAKE_MPV_STDERR"

func TestMain(m *testing.M) {
	if msg := os.Getenv(fakeMPVEnv); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(3)
	}
	os.Exit(m.Run())
}

// 这是本次修复的核心回归：mpv 启动即崩时，错误必须说明「启动后立即退出」，
// 并把 mpv 自己打印的原因原样带出来。此前 stderr 被丢进 io.Discard，
// 用户和我们能看到的只有一句「连接 mpv IPC 失败: 找不到管道」。
func TestLoadSurfacesMPVStderrWhenProcessDies(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("取不到测试二进制路径: %v", err)
	}
	t.Setenv(fakeMPVEnv, "Failed to load libmpv-2.dll")

	p, err := New(exe)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	err = p.Load(ctx, player.Stream{URL: "https://example.com/a.m3u8"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("mpv 起来即崩，Load 应报错")
	}
	if !errors.Is(err, errMPVExitedEarly) {
		t.Fatalf("错误应说明进程提前退出，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "libmpv-2.dll") {
		t.Fatalf("错误应带上 mpv 自己的输出，实际 %q", err.Error())
	}
	// 假 mpv 以 os.Exit(3) 结束；退出码必须透传出来——它是静默秒退时唯一的判据。
	if !strings.Contains(err.Error(), "退出码 3") {
		t.Fatalf("错误应带上退出码，实际 %q", err.Error())
	}
	// 进程在毫秒级退出，不该再空等满 ipcConnectTimeout。
	if elapsed > ipcConnectTimeout {
		t.Fatalf("耗时 %v，应在进程退出后立即返回而不是等满超时", elapsed)
	}
}
