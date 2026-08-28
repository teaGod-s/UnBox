package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/player"
)

const (
	maxHTMLBytes   = int64(1 << 20)
	probeBodyBytes = int64(64 << 10)
	// probeTimeout 是播放前探测（share 页解析、HLS 编码探测）的 HTTP 超时。
	// 这些请求只读几十 KB 到 1MB 的清单/网页，超时应短，避免死源挂起播放。
	probeTimeout = 10 * time.Second
)

var shareURLPattern = regexp.MustCompile(`(?is)(?:const|let|var)\s+url\s*=\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)

// Resolver 把 share 等 HTML 播放页解析为真正的媒体 URL。
type Resolver struct {
	client *http.Client
}

// NewResolver 创建解析器。client 为 nil 时使用带超时的独立 HTTP 客户端。
func NewResolver(client *http.Client) *Resolver {
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	return &Resolver{client: client}
}

// Resolve 解析间接播放页；普通媒体 URL 原样返回。
func (r *Resolver) Resolve(ctx context.Context, stream player.Stream) (player.Stream, error) {
	u, err := url.Parse(stream.URL)
	if err != nil {
		return player.Stream{}, fmt.Errorf("播放地址无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		stream.Kind = player.KindForURL(stream.URL)
		return stream, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
	if err != nil {
		return player.Stream{}, fmt.Errorf("创建播放地址请求失败: %w", err)
	}
	applyHeaders(req.Header, stream.Headers)
	resp, err := r.client.Do(req)
	if err != nil {
		return player.Stream{}, fmt.Errorf("探测播放地址失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return player.Stream{}, fmt.Errorf("探测播放地址失败: HTTP %d", resp.StatusCode)
	}

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	isHTML := mediaType == "text/html" || mediaType == "application/xhtml+xml"
	limit := probeBodyBytes
	if isHTML {
		limit = maxHTMLBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return player.Stream{}, fmt.Errorf("读取播放页失败: %w", err)
	}
	if isHTML && int64(len(body)) > maxHTMLBytes {
		return player.Stream{}, errors.New("播放页过大，拒绝解析")
	}

	match := shareURLPattern.FindSubmatch(body)
	if len(match) == 0 {
		if isHTML {
			return player.Stream{}, errors.New("播放页中未找到真实播放地址")
		}
		return stream, nil
	}
	decoded, err := decodeJSString(string(match[1]))
	if err != nil {
		return player.Stream{}, fmt.Errorf("解析播放页地址失败: %w", err)
	}
	resolved, err := resp.Request.URL.Parse(decoded)
	if err != nil {
		return player.Stream{}, fmt.Errorf("播放页地址无效: %w", err)
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return player.Stream{}, errors.New("播放页返回了不支持的地址协议")
	}
	stream.URL = resolved.String()
	stream.Kind = player.KindForURL(stream.URL)
	return stream, nil
}

func decodeJSString(raw string) (string, error) {
	if len(raw) < 2 {
		return "", errors.New("字符串为空")
	}
	if raw[0] == '\'' {
		body := raw[1 : len(raw)-1]
		body = strings.ReplaceAll(body, `\'`, `'`)
		body = strings.ReplaceAll(body, `"`, `\"`)
		raw = `"` + body + `"`
	}
	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", err
	}
	return decoded, nil
}

func applyHeaders(dst http.Header, headers map[string]string) {
	for key, value := range headers {
		if strings.TrimSpace(key) != "" {
			dst.Set(key, value)
		}
	}
}
