package shell

import (
	"context"
	"errors"
	"testing"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvplugin"
)

// stubPlayer 满足 player.Player，仅用于断言 PickPlayer 把 manager 的返回值透传出来。
type stubPlayer struct{}

func (stubPlayer) Load(context.Context, player.Stream) error { return nil }
func (stubPlayer) Play() error                               { return nil }
func (stubPlayer) Pause() error                              { return nil }
func (stubPlayer) Seek(float64) error                        { return nil }
func (stubPlayer) SetVolume(int) error                       { return nil }
func (stubPlayer) SelectTrack(player.TrackKind, int) error   { return nil }
func (stubPlayer) State() player.State                       { return player.State{} }
func (stubPlayer) Events() <-chan player.Event               { return nil }
func (stubPlayer) Close() error                              { return nil }

// fakeMPVManager 记录 PickPlayer 是否把选路与预检真正交给了 mpvplugin。
type fakeMPVManager struct {
	status    mpvplugin.Status
	player    player.Player
	playerErr error
	newCalls  int
}

func (f *fakeMPVManager) Status() mpvplugin.Status { return f.status }

func (f *fakeMPVManager) NewPlayer() (player.Player, error) {
	f.newCalls++
	return f.player, f.playerErr
}

// useManager 用桩替换真实 manager，返回恢复函数。
func useManager(f *fakeMPVManager) func() {
	orig := newMPVManager
	newMPVManager = func() mpvManager { return f }
	return func() { newMPVManager = orig }
}

// 关键回归：PickPlayer 原先自己 lookPath("mpv") 抢在 manager 之前，
// 于是「内嵌 mpv 优先」失效，且新增的 mpv --version 预检被整个绕过。
func TestPickPlayerDelegatesToManager(t *testing.T) {
	f := &fakeMPVManager{
		status: mpvplugin.Status{Available: true, Path: `C:\app\mpv\mpv.exe`},
		player: stubPlayer{},
	}
	defer useManager(f)()

	p, err := PickPlayer()
	if err != nil {
		t.Fatalf("PickPlayer() err = %v, want nil", err)
	}
	if p == nil {
		t.Fatal("PickPlayer() = nil player, want non-nil")
	}
	if f.newCalls != 1 {
		t.Fatalf("NewPlayer 调用 %d 次, want 1（选路与预检必须走 manager）", f.newCalls)
	}
}

func TestPickPlayerMissingMpv(t *testing.T) {
	f := &fakeMPVManager{status: mpvplugin.Status{Available: false}}
	defer useManager(f)()

	p, err := PickPlayer()
	if err == nil {
		t.Fatal("PickPlayer() err = nil, want non-nil（mpv 缺失应报错）")
	}
	if p != nil {
		t.Fatalf("PickPlayer() = %v, want nil player", p)
	}
	if f.newCalls != 0 {
		t.Fatalf("mpv 不可用时不该创建播放器，实际调用 %d 次", f.newCalls)
	}
}

// 预检失败（mpv 跑不起来或版本过旧）必须原样冒出来，而不是被吞掉后
// 让用户在播放时才撞上「连接 IPC 失败」。
func TestPickPlayerPropagatesManagerError(t *testing.T) {
	precheckErr := errors.New("mpv 无法运行（可能缺少依赖 DLL 或被安全软件拦截）")
	f := &fakeMPVManager{
		status:    mpvplugin.Status{Available: true, Path: "mpv.exe"},
		playerErr: precheckErr,
	}
	defer useManager(f)()

	p, err := PickPlayer()
	if !errors.Is(err, precheckErr) {
		t.Fatalf("err = %v, want 包装预检错误", err)
	}
	if p != nil {
		t.Fatalf("PickPlayer() = %v, want nil player", p)
	}
}
