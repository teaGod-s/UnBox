package playback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
)

var ErrMPVUnavailable = errors.New("mpv 插件未安装")

// preloadWarmupTimeout 限制 mpv-only 流后台网络预热的时长。
const preloadWarmupTimeout = 20 * time.Second

type streamResolver interface {
	Resolve(context.Context, player.Stream) (player.Stream, error)
}
type streamProxy interface {
	Register(context.Context, player.Stream) (string, error)
	Release(string) error
	Close() error
}

// preloadSession 记录一次预载任务，供 Release 清理代理会话或取消后台预热。
type preloadSession struct {
	proxyURL string
	cancel   context.CancelFunc
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
	preloads map[string]preloadSession
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
		preloads: make(map[string]preloadSession),
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

	if c.needsMPV(ctx, resolved) {
		return c.loadMPV(ctx, resolved, 0)
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

// needsMPV 判断一条已解析的流是否只能交给 mpv 播放。
func (c *Controller) needsMPV(ctx context.Context, resolved player.Stream) bool {
	// RTMP / 本地文件：Web 永远播不了，只能 mpv。
	if resolved.Kind == player.StreamRTMP || resolved.Kind == player.StreamLocal {
		return true
	}
	// WebView 无 MSE 时，HLS/FLV/TS 依赖 hls.js/mpegts.js 均不可用，只有
	// MP4 能走原生 <video>；其余一律 mpv。
	if !c.webMSEEnabled() && resolved.Kind != player.StreamMP4 {
		return true
	}
	// HLS 编码探测：HEVC 浏览器解不了，走 mpv。探测失败按非 HEVC 处理（fail-open 到 Web）。
	if resolved.Kind == player.StreamHLS {
		probe := c.probeHLSCodec
		if c.probe != nil {
			probe = c.probe
		}
		codec, err := probe(ctx, resolved)
		if err == nil && isHEVC(codec) {
			return true
		}
	}
	return false
}

// Preload 为下一集准备资源：Web 可播流注册独立代理会话，其余流只做轻量
// 网络预热。整个过程不接触共享播放器，也不改动当前播放会话。
// 返回的 Plan.ID 交给 Release 清理。
func (c *Controller) Preload(ctx context.Context, input player.Stream) (Plan, error) {
	resolved := input
	if c.resolver != nil {
		var err error
		resolved, err = c.resolver.Resolve(ctx, input)
		if err != nil {
			return Plan{}, err
		}
	}
	id := sessionID()
	if c.needsMPV(ctx, resolved) {
		cancel := c.warmup(resolved)
		c.mu.Lock()
		c.preloads[id] = preloadSession{cancel: cancel}
		c.mu.Unlock()
		return Plan{ID: id, Backend: BackendMPV, Kind: resolved.Kind.String()}, nil
	}
	if c.proxy == nil {
		return Plan{}, errors.New("Web 播放代理未就绪")
	}
	proxyURL, err := c.proxy.Register(ctx, resolved)
	if err != nil {
		return Plan{}, err
	}
	c.mu.Lock()
	c.preloads[id] = preloadSession{proxyURL: proxyURL}
	c.mu.Unlock()
	return Plan{ID: id, Backend: BackendWeb, URL: proxyURL, Kind: resolved.Kind.String()}, nil
}

// Release 清理一次预载：代理会话立即失效，后台预热被取消。未知 id 视为已清理。
func (c *Controller) Release(id string) error {
	c.mu.Lock()
	session, ok := c.preloads[id]
	delete(c.preloads, id)
	c.mu.Unlock()
	if !ok {
		return nil
	}
	if session.cancel != nil {
		session.cancel()
	}
	if session.proxyURL != "" && c.proxy != nil {
		return c.proxy.Release(session.proxyURL)
	}
	return nil
}

// warmup 在后台对 mpv-only 流做轻量网络预热：只发一个 Range 请求建连并读少量
// 字节，绝不调用共享播放器的 Load/Play。失败不冒泡，只记日志。
// 返回的 cancel 由 Release 调用。
func (c *Controller) warmup(stream player.Stream) context.CancelFunc {
	// 本地文件没有网络预热可言，直接返回空操作的取消函数。
	if stream.Kind == player.StreamLocal || !isHTTPURL(stream.URL) {
		return func() {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), preloadWarmupTimeout)
	go func() {
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
		if err != nil {
			return
		}
		req.Header.Set("Range", "bytes=0-1")
		applyHeaders(req.Header, stream.Headers)
		resp, err := c.client.Do(req)
		if err != nil {
			// 取消/超时是预载的正常结局，不当错误记录。
			if ctx.Err() == nil {
				log.Printf("预载预热失败 url=%s: %v", stream.URL, err)
			}
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	}()
	return cancel
}

// isHTTPURL 判断地址是否可做 HTTP 预热。
func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// loadMPV 把 stream 真正加载进 mpv 并开始播放。直接路由到 mpv 的流（RTMP/
// HEVC/本地）与 Web 失败的降级最终都收敛到这里，避免「只登记不加载」的死分支。
// start 为 Web 播放器降级前已看到的位置（秒）；0 表示从头/直播边缘起播。
func (c *Controller) loadMPV(ctx context.Context, stream player.Stream, start float64) (Plan, error) {
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
	if start > 0 {
		// 定位失败不阻断降级：能播上（哪怕从开头）也比 Web 播放失败强，
		// 不能因为一次 seek 失败就把用户踢回选集页。
		_ = mpv.Seek(start)
	}
	return Plan{ID: sessionID(), Backend: BackendMPV, Kind: stream.Kind.String()}, nil
}

// Fallback 用 mpv 重新播放 Web 失败的同一个流，并把 position 作为续播起点。
func (c *Controller) Fallback(ctx context.Context, id string, position float64) (Plan, error) {
	c.mu.Lock()
	stream, ok := c.sessions[id]
	if ok {
		delete(c.sessions, id)
	}
	c.mu.Unlock()
	if !ok {
		return Plan{}, errors.New("播放会话不存在或已降级")
	}
	return c.loadMPV(ctx, stream, position)
}

func (c *Controller) Close() error {
	c.mu.Lock()
	c.sessions = make(map[string]player.Stream)
	preloads := c.preloads
	c.preloads = make(map[string]preloadSession)
	mpv := c.mpv
	c.mpv = nil
	c.mu.Unlock()
	for _, preload := range preloads {
		if preload.cancel != nil {
			preload.cancel()
		}
	}
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
