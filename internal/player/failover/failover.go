// Package failover 在底层 Player 之上实现「失败自动切换」：监听 EOF/Error
// 事件，按候选列表（主源 + 备份源，可经 probe 测速排序）切换下一条流。
// 逻辑位于 Player 接口之上，供 Web 失败后的 mpv 播放共用。
package failover

import (
	"context"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/probe"
)

// loopExitTimeout 是 Close 等待事件循环退出的上限。循环可能正卡在内层 Load
// 上，关闭不能被下游的阻塞拖住。
const loopExitTimeout = 2 * time.Second

// Player 包装底层 Player，实现自动切换。控制方法透传 inner；单一后台事件
// 循环消费 inner.Events() 的 EOF/Error，按当前会话的候选列表切换到下一条。
type Player struct {
	inner    player.Player
	prober   *probe.Prober
	done     chan struct{}
	loopDone chan struct{}
	events   chan player.Event

	mu         sync.Mutex
	candidates []string      // 当前会话候选（主源在前）
	stream     player.Stream // 当前会话原始流（含 Headers/Kind）
	index      int           // 当前已加载候选下标
	closeOnce  sync.Once
	closeErr   error
}

// New 返回自动切换包装器，并启动单一事件循环。prober 为 nil 时按原始顺序切换。
func New(inner player.Player, prober *probe.Prober) player.Player {
	p := &Player{
		inner:    inner,
		prober:   prober,
		done:     make(chan struct{}),
		loopDone: make(chan struct{}),
		events:   make(chan player.Event, 64),
	}
	go p.loop()
	return p
}

// loop 是 inner.Events() 的唯一消费者：先处理故障切换，再把同一事件转发给
// 上层，避免故障切换与 Shell 事件桥接抢同一通道。
//
// 注意：即使已经切到备用流，触发切换的终端事件仍会照原样转发，因此上层看到
// EOF/Error 只代表「发生了终止事件」，不代表这一路彻底失败；点播侧判断线路
// 是否可用要靠自己的尝试集合，不能只看这一个事件。
func (p *Player) loop() {
	defer close(p.loopDone)
	defer close(p.events)
	for {
		select {
		case <-p.done:
			return
		case ev, ok := <-p.inner.Events():
			if !ok {
				return
			}
			if ev.Kind == player.EventEOF || ev.Kind == player.EventError {
				p.switchToNext()
			}
			if !p.forward(ev) {
				return
			}
		}
	}
}

// forward 把事件转发给上层。终端事件（EOF/Error）阻塞送达——故障切换和点播
// 自动切集都依赖它们；其余事件在上层跟不上时丢弃：位置事件丢几条无碍，
// 但绝不能让前端消费速度反压到播放器读循环（mpvproc 的终端事件是阻塞发送的，
// 读循环一旦被卡住，命令应答也会停摆）。
func (p *Player) forward(ev player.Event) bool {
	if ev.Kind == player.EventEOF || ev.Kind == player.EventError {
		select {
		case p.events <- ev:
			return true
		case <-p.done:
			return false
		}
	}
	select {
	case p.events <- ev:
		return true
	case <-p.done:
		return false
	default:
		return true // 上层跟不上，丢弃这条非终端事件
	}
}

func (p *Player) switchToNext() {
	p.mu.Lock()
	if p.index+1 >= len(p.candidates) {
		p.mu.Unlock()
		return
	}
	p.index++
	next := p.candidates[p.index]
	s := p.stream
	p.mu.Unlock()
	_ = p.inner.Load(context.Background(), streamWith(s, next))
}

func (p *Player) Load(ctx context.Context, s player.Stream) error {
	candidates := append([]string{s.URL}, s.Backups...)
	if len(candidates) > 1 && p.prober != nil {
		candidates = p.prober.Rank(ctx, candidates, s.Headers)
	}
	p.mu.Lock()
	p.candidates = candidates
	p.stream = s
	p.index = 0
	p.mu.Unlock()
	return p.inner.Load(ctx, streamWith(s, candidates[0]))
}

func streamWith(s player.Stream, url string) player.Stream {
	s.URL = url
	return s
}

func (p *Player) Play() error            { return p.inner.Play() }
func (p *Player) Pause() error           { return p.inner.Pause() }
func (p *Player) Seek(sec float64) error { return p.inner.Seek(sec) }
func (p *Player) SetVolume(v int) error  { return p.inner.SetVolume(v) }
func (p *Player) SelectTrack(k player.TrackKind, id int) error {
	return p.inner.SelectTrack(k, id)
}
func (p *Player) State() player.State         { return p.inner.State() }
func (p *Player) Events() <-chan player.Event { return p.events }
func (p *Player) Close() error {
	p.closeOnce.Do(func() {
		close(p.done)
		p.closeErr = p.inner.Close()
		select {
		case <-p.loopDone:
		case <-time.After(loopExitTimeout):
		}
	})
	return p.closeErr
}
