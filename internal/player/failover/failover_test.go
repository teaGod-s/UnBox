package failover

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// fakePlayer 是一个可编程的 inner Player，用于驱动事件。
type fakePlayer struct {
	mu      sync.Mutex
	loaded  []string
	events  chan player.Event
	playErr error
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{events: make(chan player.Event, 16)}
}

func (f *fakePlayer) Load(ctx context.Context, s player.Stream) error {
	f.mu.Lock()
	f.loaded = append(f.loaded, s.URL)
	f.mu.Unlock()
	return nil
}
func (f *fakePlayer) emit(k player.EventKind) { f.events <- player.Event{Kind: k} }
func (f *fakePlayer) Play() error             { return nil }
func (f *fakePlayer) Pause() error            { return nil }
func (f *fakePlayer) Seek(float64) error      { return nil }
func (f *fakePlayer) SetVolume(int) error     { return nil }
func (f *fakePlayer) SelectTrack(player.TrackKind, int) error {
	return nil
}
func (f *fakePlayer) State() player.State         { return player.State{} }
func (f *fakePlayer) Events() <-chan player.Event { return f.events }
func (f *fakePlayer) Close() error                { return nil }
func (f *fakePlayer) loadedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.loaded...)
}

func TestFailoverSwitchesOnError(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil) // 无 prober：按原始顺序切换
	s := player.Stream{URL: "http://primary", Backups: []string{"http://b1", "http://b2"}}
	if err := fp.Load(context.Background(), s); err != nil {
		t.Fatalf("Load: %v", err)
	}
	inner.emit(player.EventError)
	inner.emit(player.EventError) // 再错一次，切到 b2
	waitFor(t, func() bool { return len(inner.loadedURLs()) == 3 })
	got := inner.loadedURLs()
	if got[0] != "http://primary" || got[1] != "http://b1" || got[2] != "http://b2" {
		t.Fatalf("loaded = %v，期望按序切换", got)
	}
}

func TestFailoverStopsWhenExhausted(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	fp.Load(context.Background(), player.Stream{URL: "http://only"})
	for i := 0; i < 5; i++ {
		inner.emit(player.EventError)
	}
	// 只有 1 个候选，第一次错误后即无下一个，不再 Load
	waitFor(t, func() bool { return len(inner.loadedURLs()) >= 1 })
	if len(inner.loadedURLs()) != 1 {
		t.Fatalf("loaded = %v，期望仅 1 次（无备份可切）", inner.loadedURLs())
	}
}

func TestFailoverNewLoadResetsSession(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	fp.Load(context.Background(), player.Stream{URL: "http/a", Backups: []string{"http/a2"}})
	fp.Load(context.Background(), player.Stream{URL: "http/b"}) // 新会话应取消旧监听
	inner.emit(player.EventError)
	waitFor(t, func() bool { return len(inner.loadedURLs()) == 2 }) // b 已 load
	// 旧会话的 a2 不应被加载；只可能加载 b 一次（无备份）
	if len(inner.loadedURLs()) != 2 || inner.loadedURLs()[1] != "http/b" {
		t.Fatalf("loaded = %v，期望 [a b]，无串味", inner.loadedURLs())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("超时等待条件满足")
}
