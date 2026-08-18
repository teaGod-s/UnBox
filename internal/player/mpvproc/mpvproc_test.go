package mpvproc

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

func TestLoadPlayClose(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("本机无 mpv，跳过集成测试")
	}
	p, err := New(mpv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 用本地生成的静音短视频？M1 不引入 ffmpeg，这里用一个不会立即 EOF 的
	// 短循环流不好造，退而求其次：验证 Load 能启动 mpv 并建立 IPC 即可，
	// 播放本身由 Task 5 的冒烟在真实环境人工确认。
	if err := p.Load(ctx, player.Stream{URL: "https://example.com/none.m3u8"}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st := p.State(); st.Playing != player.StatePlaying {
		t.Fatalf("Load 后 Playing = %v, want playing", st.Playing)
	}
	if err := p.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
}
