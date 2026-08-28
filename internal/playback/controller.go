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
//
// webMSE 标记 WebView 是否支持 MSE（hls.js/mpegts.js 依赖）。Linux 的
// WebKitGTK 无 MSE 也不原生支持 HLS，故 HLS/FLV/TS 只能走 mpv；原生
// <video> 仅可靠覆盖 MP4。其余平台（Windows WebView2 / macOS WKWebView）
// 具备 MSE 或原生 HLS，Web 能力默认为真。
type Controller struct {
	resolver streamResolver
	proxy    streamProxy
	client   *http.Client
	mu       sync.Mutex
	mpv      player.Player
	sessions map[string]player.Stream
	webMSE   bool
	probe    func(context.Context, player.Stream) (string, error)
}

func NewController(resolver streamResolver, proxy streamProxy, mpv player.Player) *Controller {
	return &Controller{
		resolver: resolver,
		proxy:    proxy,
		client:   &http.Client{Timeout: probeTimeout},
		mpv:      mpv,
		sessions: make(map[string]player.Stream),
		webMSE:   true,
	}
}

// SetWebMSE 标记 WebView 的 MSE 能力；须在开始 Prepare 之前调用一次。
func (c *Controller) SetWebMSE(v bool) {
	c.mu.Lock()
	c.webMSE = v
	c.mu.Unlock()
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

	// RTMP / 本地文件：Web 永远播不了，只能 mpv。
	if resolved.Kind == player.StreamRTMP || resolved.Kind == player.StreamLocal {
		return c.loadMPV(ctx, resolved)
	}

	// WebView 无 MSE 时，HLS/FLV/TS 依赖 hls.js/mpegts.js 均不可用，只有
	// MP4 能走原生 <video>；其余一律 mpv。
	if !c.webMSEEnabled() && resolved.Kind != player.StreamMP4 {
		return c.loadMPV(ctx, resolved)
	}

	// HLS 编码探测：HEVC 浏览器解不了，走 mpv。探测失败按非 HEVC 处理（fail-open 到 Web）。
	if resolved.Kind == player.StreamHLS {
		probe := c.probeHLSCodec
		if c.probe != nil {
			probe = c.probe
		}
		codec, err := probe(ctx, resolved)
		if err == nil && isHEVC(codec) {
			return c.loadMPV(ctx, resolved)
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

// loadMPV 把 stream 真正加载进 mpv 并开始播放。直接路由到 mpv 的流（RTMP/
// HEVC/本地）与 Web 失败的降级最终都收敛到这里，避免「只登记不加载」的死分支。
func (c *Controller) loadMPV(ctx context.Context, stream player.Stream) (Plan, error) {
	c.mu.Lock()
	mpv := c.mpv
	c.mu.Unlock()
	if mpv == nil {
		return Plan{}, ErrMPVUnavailable
	}
	if err := mpv.Load(ctx, stream); err != nil {
		return Plan{}, fmt.Errorf("mpv 加载失败: %w", err)
	}
	if err := mpv.Play(); err != nil {
		return Plan{}, fmt.Errorf("mpv 播放失败: %w", err)
	}
	return Plan{ID: sessionID(), Backend: BackendMPV, Kind: stream.Kind.String()}, nil
}

func (c *Controller) Fallback(ctx context.Context, id string) (Plan, error) {
	c.mu.Lock()
	stream, ok := c.sessions[id]
	if ok {
		delete(c.sessions, id)
	}
	c.mu.Unlock()
	if !ok {
		return Plan{}, errors.New("播放会话不存在或已降级")
	}
	return c.loadMPV(ctx, stream)
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

func (c *Controller) webMSEEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.webMSE
}

func (c *Controller) probeHLSCodec(ctx context.Context, stream player.Stream) (string, error) {
	u := stream.URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	applyHeaders(req.Header, stream.Headers)
	resp, err := c.client.Do(req)
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
