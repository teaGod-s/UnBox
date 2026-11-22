package mpvproc

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// 命令应答超时是最常见的 mpv 失败形态，报错必须带上 mpv 自己的 stderr，
// 否则只剩一句干巴巴的「命令应答超时」，无从定位是网络还是解码问题。
func TestSendTimeoutCarriesMpvStderr(t *testing.T) {
	p := &mpvProc{
		responses: make(chan response, 16),
		conn:      unresponsiveConn{},
		session:   1,
	}
	p.stderr = &stderrTail{max: 1024}
	_, _ = p.stderr.Write([]byte("Failed to resolve 'example.com'"))

	oldTimeout := cmdResponseTimeout
	cmdResponseTimeout = 30 * time.Millisecond
	defer func() { cmdResponseTimeout = oldTimeout }()

	err := p.send("loadfile", "http://x/a.m3u8", "replace")
	if err == nil || !strings.Contains(err.Error(), "Failed to resolve") {
		t.Fatalf("应答超时应带上 mpv stderr，got %v", err)
	}
}

// unresponsiveConn 是「写得进、永远不应答」的假连接，用来触发命令应答超时。
type unresponsiveConn struct{}

func (unresponsiveConn) Write(p []byte) (int, error) { return len(p), nil }
func (unresponsiveConn) Read(p []byte) (int, error)  { return 0, io.ErrClosedPipe }
func (unresponsiveConn) Close() error                { return nil }

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

// TestConcurrentReload 并发交错 Load/Play/Pause/Seek/Close，令旧会话 send 与
// 新会话 Load 重叠，验证：会话代际丢弃跨会话串味的迟到应答、指针同一性判断
// 不误杀后继会话，且全程无 race/panic/死锁。末尾再做一次 Load+Play+Pause 确认
// 实例仍可用。
func TestConcurrentReload(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("本机无 mpv，跳过集成测试")
	}
	p, err := New(mpv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const url = "https://example.com/none.m3u8"

	// 初始会话。
	if err := p.Load(ctx, player.Stream{URL: url}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				_ = p.Load(ctx, player.Stream{URL: url})
				_ = p.Play()
				_ = p.Pause()
				_ = p.Seek(1)
				_ = p.Close()
			}
		}()
	}
	wg.Wait()

	// 结束后仍可用：串味被代际丢弃，无死锁。
	if err := p.Load(ctx, player.Stream{URL: url}); err != nil {
		t.Fatalf("末尾 Load: %v", err)
	}
	if err := p.Play(); err != nil {
		t.Fatalf("末尾 Play: %v", err)
	}
	if err := p.Pause(); err != nil {
		t.Fatalf("末尾 Pause: %v", err)
	}
}

func TestSendEventTerminalBlocksUntilRead(t *testing.T) {
	ch := make(chan player.Event) // 无缓冲：无人读取时必然阻塞
	done := make(chan struct{})
	go func() {
		sendEvent(ch, player.Event{Kind: player.EventEOF})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("终端事件在无人读取时被丢弃了（不应发生）")
	case <-time.After(20 * time.Millisecond):
		// 预期：阻塞在发送上，done 未关闭
	}
	<-ch // 读取后放行
	<-done
}

func TestSendEventPositionDropsWhenFull(t *testing.T) {
	ch := make(chan player.Event, 1)
	ch <- player.Event{Kind: player.EventPosition}          // 占满
	sendEvent(ch, player.Event{Kind: player.EventPosition}) // 应丢弃而非阻塞
}

// waitMediaReady 测试：媒体未就绪时阻塞，就绪信号到达后放行。
func TestWaitMediaReadyWaitsForSignal(t *testing.T) {
	p := &mpvProc{}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		p.waitMediaReady() // 返回 nil
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("媒体未就绪时 waitMediaReady 不应立即返回")
	case <-time.After(20 * time.Millisecond):
	}

	p.lifecycleMu.Lock()
	close(p.ready)
	p.lifecycleMu.Unlock()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("就绪信号到达后 waitMediaReady 未放行")
	}
}

// waitMediaReady 测试：已就绪时立即返回，不做多余等待。
func TestWaitMediaReadyReturnsWhenAlreadyReady(t *testing.T) {
	p := &mpvProc{}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	close(p.ready)
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		p.waitMediaReady() // 返回 nil
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("已就绪时 waitMediaReady 应立即返回")
	}
}

// waitMediaReady 测试：死源等不到就绪也不报失败时，到上限照常放行
// （交给 send 如实报错）。
func TestWaitMediaReadyTimesOut(t *testing.T) {
	old := seekReadyTimeout
	seekReadyTimeout = 20 * time.Millisecond
	defer func() { seekReadyTimeout = old }()

	p := &mpvProc{}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		p.waitMediaReady() // 超时放行，返回 nil
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("等不到就绪时应超时放行")
	}
}

// waitMediaReady 测试：媒体加载失败（end-file error → loadFailed 关闭）时
// 返回明确错误，seek 不再发注定被拒的命令。
func TestWaitMediaReadyReturnsErrorOnLoadFailed(t *testing.T) {
	p := &mpvProc{}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	evt := make(chan error, 1)
	go func() {
		evt <- p.waitMediaReady()
	}()

	// 未失败未就绪：应还在等。
	select {
	case <-evt:
		t.Fatal("加载尚未失败时 waitMediaReady 不应返回")
	case <-time.After(20 * time.Millisecond):
	}

	p.lifecycleMu.Lock()
	close(p.loadFailed)
	p.lifecycleMu.Unlock()

	select {
	case err := <-evt:
		if err == nil || !strings.Contains(err.Error(), "加载失败") {
			t.Fatalf("加载失败后应返回加载错误，got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("加载失败信号后 waitMediaReady 未返回")
	}
}

// waitMediaReady 测试：ready 与 loadFailed 都关闭时，就绪优先——一旦媒体加载
// 成功过，后续 seek 永远放行，不被播放中途的 end-file(error) 误伤。
func TestWaitMediaReadyPrefersReadyOverLateFailure(t *testing.T) {
	p := &mpvProc{}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	close(p.ready)
	p.loadFailed = make(chan struct{})
	close(p.loadFailed)
	p.lifecycleMu.Unlock()

	if err := p.waitMediaReady(); err != nil {
		t.Fatalf("就绪后应放行且不报失败，got %v", err)
	}
}

// Seek 测试：媒体未就绪时 seek 不发命令，就绪后才发（用 readLoop 注入 file-loaded）。
func TestSeekWaitsForMediaReadyViaReadLoop(t *testing.T) {
	p := &mpvProc{
		conn:      &recordingConn{writes: make(chan string, 16)},
		responses: make(chan response, 16),
		events:    make(chan player.Event, 64),
		session:   1,
	}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	ready := p.ready
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	// 先备好成功应答，再放行 seek：避免 send 等 15s 应答超时拖慢测试。
	p.responses <- response{session: 1, data: []byte(`{"error":"success"}`)}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = p.Seek(30) // 应等 file-loaded 后发送
	}()

	// 就绪信号未到：seek 命令不应出现在连接写入里。
	select {
	case w := <-p.conn.(*recordingConn).writes:
		t.Fatalf("媒体未就绪时不应写入 seek 命令，got %q", w)
	case <-time.After(30 * time.Millisecond):
	}

	// 注入 file-loaded 事件等价于关闭 ready（readLoop 的行为）。
	p.lifecycleMu.Lock()
	close(ready)
	p.lifecycleMu.Unlock()

	// 就绪后 seek 命令才应写出发送。
	select {
	case w := <-p.conn.(*recordingConn).writes:
		if !strings.Contains(w, `"seek"`) {
			t.Fatalf("就绪后应写入 seek 命令，got %q", w)
		}
	case <-time.After(time.Second):
		t.Fatal("就绪后 seek 未写出")
	}

	wg.Wait()
}

// Seek 测试：媒体加载失败时 seek 直接返回加载错误，不写命令。
func TestSeekReturnsErrorOnLoadFailed(t *testing.T) {
	p := &mpvProc{
		conn:      &recordingConn{writes: make(chan string, 16)},
		responses: make(chan response, 16),
		events:    make(chan player.Event, 64),
		session:   1,
	}
	p.lifecycleMu.Lock()
	p.ready = make(chan struct{})
	p.loadFailed = make(chan struct{})
	p.lifecycleMu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Seek(30)
	}()

	// 加载失败信号到达 → seek 应返回加载错误且从未写命令。
	p.lifecycleMu.Lock()
	close(p.loadFailed)
	p.lifecycleMu.Unlock()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "加载失败") {
			t.Fatalf("加载失败后 Seek 应返回加载错误，got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("加载失败后 Seek 未返回")
	}

	select {
	case w := <-p.conn.(*recordingConn).writes:
		t.Fatalf("加载失败后不应写入 seek 命令，got %q", w)
	default:
	}
}

// recordingConn 记录写入的命令行；读取立即 EOF，供 seek 走到 send 后不挂死。
type recordingConn struct {
	writes chan string // 非缓冲消费：每条 Write 记入后由测试读取
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.writes <- string(p)
	return len(p), nil
}
func (recordingConn) Read(p []byte) (int, error) { return 0, io.ErrClosedPipe }
func (recordingConn) Close() error               { return nil }
