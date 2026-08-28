package shell

import (
	"errors"
	"testing"

	"github.com/unbox/unbox/internal/player/mpvplugin"
)

func TestPickPlayerHappyPath(t *testing.T) {
	// 本机已装 mpv，lookPath("mpv") 应命中；PickPlayer 返回非 nil 的播放器且无错误。
	// 保守起见先探测：若本机确实没有 mpv，跳过 happy path 断言，避免误报。
	if _, err := lookPath("mpv"); err != nil {
		t.Skipf("本机未安装 mpv，跳过 happy path: %v", err)
	}

	p, err := PickPlayer()
	if err != nil {
		t.Fatalf("PickPlayer() err = %v, want nil", err)
	}
	if p == nil {
		t.Fatal("PickPlayer() = nil player, want non-nil")
	}
	_ = p.Close()
}

func TestPickPlayerMissingMpv(t *testing.T) {
	// 用桩替换 lookPath，覆盖「mpv 缺失」分支：应返回明确错误而非 panic。
	orig := lookPath
	lookPath = func(string) (string, error) {
		return "", errors.New("exec: mpv not found")
	}
	defer func() { lookPath = orig }()
	origStatus := pluginStatus
	pluginStatus = func() mpvplugin.Status { return mpvplugin.Status{} }
	defer func() { pluginStatus = origStatus }()

	p, err := PickPlayer()
	if err == nil {
		t.Fatal("PickPlayer() err = nil, want non-nil（mpv 缺失应报错）")
	}
	if p != nil {
		t.Fatalf("PickPlayer() = %v, want nil player", p)
	}
}
