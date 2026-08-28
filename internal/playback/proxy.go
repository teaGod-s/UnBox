package playback

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
)

const (
	maxManifestBytes = int64(4 << 20)
	// 上游流可能持续整个播放时长，故不设 http.Client 整体 Timeout（那会
	// 在长流中途掐断）。改为只给建连/握手/响应头设超时，防死源挂起。
	streamDialTimeout  = 10 * time.Second
	streamTLSHandshake = 10 * time.Second
	streamRespHeader   = 30 * time.Second
)

var hlsURIAttribute = regexp.MustCompile(`URI="([^"]+)"`)

type proxyEntry struct {
	stream    player.Stream
	secret    []byte
	expiresAt time.Time
}

// Proxy 把带防盗链 headers 的远端流安全地转发到本机 WebView。
type Proxy struct {
	client *http.Client
	ttl    time.Duration
	now    func() time.Time

	mu       sync.RWMutex
	streams  map[string]proxyEntry
	listener net.Listener
	server   *http.Server
	closed   bool
}

// NewProxy 创建按需启动的本地代理。
func NewProxy(client *http.Client, ttl time.Duration) *Proxy {
	if client == nil {
		client = &http.Client{Transport: newStreamTransport()}
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Proxy{
		client:  client,
		ttl:     ttl,
		now:     time.Now,
		streams: make(map[string]proxyEntry),
	}
}

// newStreamTransport 返回只对建连/握手/响应头设超时的 Transport，避免用
// 整体 Timeout 把长播放流中途掐断。
func newStreamTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: streamDialTimeout}).DialContext,
		TLSHandshakeTimeout:   streamTLSHandshake,
		ResponseHeaderTimeout: streamRespHeader,
	}
}

// Register 登记一条流并返回仅持有签名 URL 才能访问的本地地址。
func (p *Proxy) Register(_ context.Context, stream player.Stream) (string, error) {
	target, err := url.Parse(stream.URL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return "", errors.New("代理只支持有效的 HTTP/HTTPS 播放地址")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("生成代理令牌失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return "", errors.New("播放代理已关闭")
	}
	if err := p.startLocked(); err != nil {
		return "", err
	}
	now := p.now()
	p.cleanupLocked(now)
	p.streams[token] = proxyEntry{
		stream:    cloneStream(stream),
		secret:    append([]byte(nil), tokenBytes...),
		expiresAt: now.Add(p.ttl),
	}
	return p.signedURLLocked(token, tokenBytes, target), nil
}

func (p *Proxy) startLocked() error {
	if p.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("启动播放代理失败: %w", err)
	}
	p.listener = listener
	p.server = &http.Server{Handler: http.HandlerFunc(p.serveHTTP), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		_ = p.server.Serve(listener)
	}()
	return nil
}

func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w.Header())
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "不支持的代理请求方法", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/proxy/")
	if token == "" || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}
	entry, ok := p.lookup(token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	target, ok := verifyTarget(r.URL.Query(), entry.secret)
	if !ok {
		http.Error(w, "代理地址签名无效", http.StatusForbidden)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if err != nil {
		http.Error(w, "创建上游请求失败", http.StatusBadGateway)
		return
	}
	applyHeaders(req.Header, entry.stream.Headers)
	copyRequestHeaders(req.Header, r.Header)
	client := p.clientFor(entry.stream.Headers)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "请求上游媒体失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	manifest := isHLSManifest(resp.Request.URL, resp.Header.Get("Content-Type"))
	copyResponseHeaders(w.Header(), resp.Header, manifest)
	setCORSHeaders(w.Header())
	if !manifest || r.Method == http.MethodHead || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
	if err != nil {
		http.Error(w, "读取 HLS 清单失败", http.StatusBadGateway)
		return
	}
	if int64(len(body)) > maxManifestBytes {
		http.Error(w, "HLS 清单过大", http.StatusBadGateway)
		return
	}
	rewritten := p.rewriteManifest(token, entry.secret, resp.Request.URL, string(body))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.WriteString(w, rewritten)
}

func (p *Proxy) lookup(token string) (proxyEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.streams[token]
	if !ok {
		return proxyEntry{}, false
	}
	if !p.now().Before(entry.expiresAt) {
		delete(p.streams, token)
		return proxyEntry{}, false
	}
	entry.secret = append([]byte(nil), entry.secret...)
	entry.stream = cloneStream(entry.stream)
	return entry, true
}

func (p *Proxy) rewriteManifest(token string, secret []byte, base *url.URL, body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			if target, ok := resolveMediaURL(base, trimmed); ok {
				lines[i] = strings.Replace(line, trimmed, p.signedURL(token, secret, target), 1)
			}
			continue
		}
		lines[i] = hlsURIAttribute.ReplaceAllStringFunc(line, func(match string) string {
			sub := hlsURIAttribute.FindStringSubmatch(match)
			if len(sub) != 2 {
				return match
			}
			target, ok := resolveMediaURL(base, sub[1])
			if !ok {
				return match
			}
			return `URI="` + p.signedURL(token, secret, target) + `"`
		})
	}
	return strings.Join(lines, "\n")
}

func resolveMediaURL(base *url.URL, reference string) (*url.URL, bool) {
	rel, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return nil, false
	}
	target := base.ResolveReference(rel)
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, false
	}
	return target, true
}

func (p *Proxy) signedURL(token string, secret []byte, target *url.URL) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.signedURLLocked(token, secret, target)
}

func (p *Proxy) signedURLLocked(token string, secret []byte, target *url.URL) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(target.String()))
	values := url.Values{"url": {encoded}, "sig": {signTarget(secret, target.String())}}
	return "http://" + p.listener.Addr().String() + "/proxy/" + token + "?" + values.Encode()
}

func signTarget(secret []byte, target string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(target))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyTarget(query url.Values, secret []byte) (*url.URL, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(query.Get("url"))
	if err != nil {
		return nil, false
	}
	target, err := url.Parse(string(raw))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, false
	}
	want, err := hex.DecodeString(signTarget(secret, target.String()))
	if err != nil {
		return nil, false
	}
	got, err := hex.DecodeString(query.Get("sig"))
	if err != nil || !hmac.Equal(got, want) {
		return nil, false
	}
	return target, true
}

func (p *Proxy) clientFor(headers map[string]string) *http.Client {
	client := *p.client
	original := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		applyHeaders(req.Header, headers)
		if original != nil {
			return original(req, via)
		}
		if len(via) >= 10 {
			return errors.New("重定向次数过多")
		}
		return nil
	}
	return &client
}

func isHLSManifest(target *url.URL, contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "application/vnd.apple.mpegurl" || mediaType == "application/x-mpegurl" || mediaType == "audio/mpegurl" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(target.Path), ".m3u8")
}

func copyRequestHeaders(dst, src http.Header) {
	for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since", "Accept"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Proxy-Connection":    {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func copyResponseHeaders(dst, src http.Header, rewritten bool) {
	for key, values := range src {
		canonical := http.CanonicalHeaderKey(key)
		if _, skip := hopByHopHeaders[canonical]; skip || (rewritten && (canonical == "Content-Length" || canonical == "Content-Encoding")) {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

func setCORSHeaders(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Range")
	header.Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range")
}

func cloneStream(stream player.Stream) player.Stream {
	stream.Headers = cloneStringMap(stream.Headers)
	stream.Backups = append([]string(nil), stream.Backups...)
	stream.Subtitle = append([]player.SubtitleTrack(nil), stream.Subtitle...)
	return stream
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func (p *Proxy) cleanupLocked(now time.Time) {
	for token, entry := range p.streams {
		if !now.Before(entry.expiresAt) {
			delete(p.streams, token)
		}
	}
}

func (p *Proxy) baseURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.listener == nil {
		return ""
	}
	return "http://" + p.listener.Addr().String()
}

// Close 停止本地监听并清除全部 token。
func (p *Proxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	server := p.server
	p.streams = make(map[string]proxyEntry)
	p.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Close()
}
