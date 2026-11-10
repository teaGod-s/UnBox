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

	"github.com/unbox/unbox/internal/httpx"
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
		client = httpx.NewClient(probeTimeout)
	}
	return &Resolver{client: client}
}

// Resolve 解析间接播放页；普通媒体 URL 原样返回。
// bilibili 复合地址（<aid>+<cid>+…）不是合法 URL，先识别并换真实流。
func (r *Resolver) Resolve(ctx context.Context, stream player.Stream) (player.Stream, error) {
	if aid, cid, qs, ok := parseBiliComposite(stream.URL); ok {
		return r.resolveBili(ctx, aid, cid, qs)
	}
	u, err := url.Parse(stream.URL)
	if err != nil {
		return player.Stream{}, fmt.Errorf("播放地址无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		stream.Kind = player.KindForURL(stream.URL)
		return stream, nil
	}
	// 平台播放页（B站/腾讯/爱奇艺…）要登录和会员，HTML 里没有可直接播放的地址。
	// 规则站（如 某影视源）常常把这类页面原样透传，早识别早报清楚，别白等一次抓取。
	if platform := platformPageName(u); platform != "" {
		return player.Stream{}, fmt.Errorf("该线路指向%s平台页面，需要平台会员或解析服务，无法直接播放", platform)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
	if err != nil {
		return player.Stream{}, fmt.Errorf("创建播放地址请求失败: %w", err)
	}
	applyHeaders(req.Header, stream.Headers)
	resp, err := httpx.Do(r.client, req)
	if err != nil {
		return player.Stream{}, fmt.Errorf("探测播放地址失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return player.Stream{}, fmt.Errorf("站点反爬或源站异常：探测播放地址返回 HTTP %d（可换线路或稍后重试）", resp.StatusCode)
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

// platformPageName 识别「平台播放页」并返回平台名（不是平台页返回空串）。
// 只匹配各家的页面域名，命中不了 CDN 直链域名（那些是真能播的）。
func platformPageName(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "www.bilibili.com" || host == "m.bilibili.com" || host == "b23.tv":
		return "B站"
	case host == "v.qq.com" || host == "m.v.qq.com":
		return "腾讯视频"
	case host == "www.iqiyi.com" || host == "m.iqiyi.com":
		return "爱奇艺"
	case host == "v.youku.com" || host == "m.youku.com":
		return "优酷"
	case host == "www.mgtv.com" || host == "m.mgtv.com":
		return "芒果TV"
	case host == "tv.sohu.com" || host == "m.tv.sohu.com":
		return "搜狐视频"
	case host == "www.le.com" || host == "m.le.com":
		return "乐视"
	}
	return ""
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
