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

func TestFailoverFansOutEvents(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	defer fp.Close()

	if err := fp.Load(context.Background(), player.Stream{URL: "http://primary"}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	inner.events <- player.Event{Kind: player.EventPosition, Position: 12.5}
	inner.events <- player.Event{Kind: player.EventPlaying}
	inner.events <- player.Event{Kind: player.EventBuffering}
	inner.events <- player.Event{Kind: player.EventError, Err: context.Canceled}
	inner.events <- player.Event{Kind: player.EventEOF}

	got := make([]player.Event, 0, 5)
	for len(got) < 5 {
		select {
		case ev := <-fp.Events():
			got = append(got, ev)
		case <-time.After(time.Second):
			t.Fatalf("超时等待扇出事件，已收到 %d 个", len(got))
		}
	}
	want := []player.EventKind{
		player.EventPosition,
		player.EventPlaying,
		player.EventBuffering,
		player.EventError,
		player.EventEOF,
	}
	for i, ev := range got {
		if ev.Kind != want[i] {
			t.Fatalf("事件 %d = %v，want %v", i, ev.Kind, want[i])
		}
	}
	if got[0].Position != 12.5 {
		t.Fatalf("位置事件载荷丢失: %#v", got[0])
	}
	if got[3].Err != context.Canceled {
		t.Fatalf("错误事件载荷丢失: %#v", got[3])
	}
}

func TestFailoverDropsNonTerminalEventsWhenUnconsumed(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	defer fp.Close()

	// 不消费 fp.Events()：非终端事件应被丢弃，而不是反压回内层播放器。
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for i := 0; i < 500; i++ {
			inner.events <- player.Event{Kind: player.EventPosition, Position: float64(i)}
		}
	}()
	select {
	case <-producerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("非终端事件不应反压到内层播放器")
	}
}

func TestFailoverKeepsTerminalEventsWhenUnconsumed(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	defer fp.Close()

	// 灌入超过扇出缓冲的事件后再补一条终端事件：位置事件会被丢弃，
	// 终端事件必须一直等到消费者读到为止。
	go func() {
		for i := 0; i < 200; i++ {
			inner.events <- player.Event{Kind: player.EventPosition, Position: float64(i)}
		}
		inner.events <- player.Event{Kind: player.EventError, Err: context.Canceled}
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-fp.Events():
			if ev.Kind != player.EventError {
				continue
			}
			if ev.Err != context.Canceled {
				t.Fatalf("终端事件载荷丢失: %#v", ev)
			}
			return
		case <-deadline:
			t.Fatal("未能在超时前读到终端事件")
		}
	}
}

func TestFailoverClosesFanoutChannel(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	if err := fp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-fp.Events():
		if ok {
			t.Fatal("Close 后 Events 通道应关闭")
		}
	case <-time.After(time.Second):
		t.Fatal("Close 后 Events 通道未关闭")
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
