// Package failover 在底层 Player 之上实现「失败自动切换」：监听 EOF/Error
// 事件，按候选列表（主源 + 备份源，可经 probe 测速排序）切换下一条流。
// 逻辑位于 Player 接口之上，供 Web 失败后的 mpv 播放共用。
package failover

import (
	"context"
	"sync"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/probe"
)

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
			select {
			case p.events <- ev:
			case <-p.done:
				return
			}
		}
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
	var err error
	p.closeOnce.Do(func() {
		close(p.done)
		err = p.inner.Close()
		<-p.loopDone
	})
	return err
}
