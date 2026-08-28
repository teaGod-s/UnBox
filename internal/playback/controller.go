package playback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/player"
)

var ErrMPVUnavailable = errors.New("mpv 插件未安装")

type streamResolver interface {
	Resolve(context.Context, player.Stream) (player.Stream, error)
}
type streamProxy interface {
	Register(context.Context, player.Stream) (string, error)
	Close() error
}

// Controller 负责一次播放计划的解析、路由和 Web→mpv 降级。
type Controller struct {
	resolver streamResolver
	proxy    streamProxy
	mu       sync.Mutex
	mpv      player.Player
	sessions map[string]player.Stream
	resolve  func(context.Context, player.Stream) (player.Stream, string, error)
	probe    func(context.Context, player.Stream) (string, error)
}

func NewController(resolver streamResolver, proxy streamProxy, mpv player.Player) *Controller {
	return &Controller{resolver: resolver, proxy: proxy, mpv: mpv, sessions: make(map[string]player.Stream)}
}

func (c *Controller) SetMPV(next player.Player) error {
	c.mu.Lock()
	old := c.mpv
	c.mpv = next
	c.mu.Unlock()
	if old != nil && old != next {
		return old.Close()
	}
	return nil
}

func (c *Controller) MPVReady() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.mpv != nil }

func (c *Controller) Prepare(ctx context.Context, input player.Stream) (Plan, error) {
	resolved := input
	if c.resolver != nil {
		var err error
		resolved, err = c.resolver.Resolve(ctx, input)
		if err != nil {
			return Plan{}, err
		}
	}
	if c.resolve != nil {
		var err error
		resolved, _, err = c.resolve(ctx, resolved)
		if err != nil {
			return Plan{}, err
		}
	}
	if resolved.Kind == player.StreamRTMP || resolved.Kind == player.StreamLocal {
		return c.prepareMPV(ctx, resolved)
	}
	if resolved.Kind == player.StreamHLS {
		probe := c.probeHLSCodec
		if c.probe != nil {
			probe = c.probe
		}
		codec, err := probe(ctx, resolved)
		if err == nil && isHEVC(codec) {
			return c.prepareMPV(ctx, resolved)
		}
	}
	if c.proxy == nil {
		return Plan{}, errors.New("Web 播放代理未就绪")
	}
	proxyURL, err := c.proxy.Register(ctx, resolved)
	if err != nil {
		return Plan{}, err
	}
	id := sessionID()
	c.mu.Lock()
	c.sessions[id] = cloneStream(resolved)
	c.mu.Unlock()
	return Plan{ID: id, Backend: BackendWeb, URL: proxyURL, Kind: resolved.Kind.String(), CanFallback: c.MPVReady()}, nil
}

func (c *Controller) prepareMPV(_ context.Context, stream player.Stream) (Plan, error) {
	c.mu.Lock()
	ready := c.mpv != nil
	c.mu.Unlock()
	if !ready {
		return Plan{}, ErrMPVUnavailable
	}
	id := sessionID()
	c.mu.Lock()
	c.sessions[id] = cloneStream(stream)
	c.mu.Unlock()
	return Plan{ID: id, Backend: BackendMPV, Kind: stream.Kind.String()}, nil
}

func (c *Controller) Fallback(ctx context.Context, id string) (Plan, error) {
	c.mu.Lock()
	stream, ok := c.sessions[id]
	if ok {
		delete(c.sessions, id)
	}
	mpv := c.mpv
	c.mu.Unlock()
	if !ok {
		return Plan{}, errors.New("播放会话不存在或已降级")
	}
	if mpv == nil {
		return Plan{}, ErrMPVUnavailable
	}
	if err := mpv.Load(ctx, stream); err != nil {
		return Plan{}, fmt.Errorf("mpv 加载失败: %w", err)
	}
	if err := mpv.Play(); err != nil {
		return Plan{}, fmt.Errorf("mpv 播放失败: %w", err)
	}
	return Plan{ID: id, Backend: BackendMPV, Kind: stream.Kind.String()}, nil
}

func (c *Controller) Close() error {
	c.mu.Lock()
	c.sessions = make(map[string]player.Stream)
	mpv := c.mpv
	c.mpv = nil
	c.mu.Unlock()
	var first error
	if c.proxy != nil {
		first = c.proxy.Close()
	}
	if mpv != nil {
		if err := mpv.Close(); first == nil {
			first = err
		}
	}
	return first
}

func (c *Controller) probeHLSCodec(ctx context.Context, stream player.Stream) (string, error) {
	u := stream.URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	applyHeaders(req.Header, stream.Headers)
	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HLS 探测失败: HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 64<<10)
	n, _ := resp.Body.Read(buf)
	text := string(buf[:n])
	if !strings.Contains(text, "#EXT-X-STREAM-INF") {
		return "", nil
	}
	idx := strings.Index(text, "CODECS=")
	if idx < 0 {
		return "", nil
	}
	return text[idx:], nil
}

func isHEVC(codec string) bool { return regexp.MustCompile(`(?i)(hvc1|hev1|hevc)`).MatchString(codec) }
func sessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", len(b))))
	}
	return hex.EncodeToString(b)
}
