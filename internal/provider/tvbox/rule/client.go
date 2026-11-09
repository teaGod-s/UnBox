package rule

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxBodySize = 4 << 20
	tvboxUA     = "okhttp/3.12.11"
	// browserUA 是 403 时的 UA 兜底：黑狐影院等站点针对 TVBox 默认的
	// okhttp UA 定向封锁，换浏览器形态 UA 一般可放行。
	browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Client 是规则引擎的 HTTP 客户端：固定 UA、自定义头、内存 Cookie。
type Client struct {
	hc      *http.Client
	headers map[string]string
	mu      sync.Mutex
	cookies map[string]string
}

// NewClient 构造客户端；headers 可为 nil。
func NewClient(headers map[string]string) *Client {
	return &Client{
		hc:      &http.Client{Timeout: 15 * time.Second},
		headers: headers,
		cookies: map[string]string{},
	}
}

// Get 发起 GET 并返回正文。
func (c *Client) Get(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	return c.do(req)
}

// Post 发起 POST；contentType 为空时用 application/x-www-form-urlencoded。
func (c *Client) Post(ctx context.Context, rawURL, body, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	if contentType == "" {
		contentType = "application/x-www-form-urlencoded"
	}
	req.Header.Set("Content-Type", contentType)
	return c.do(req)
}

func (c *Client) do(req *http.Request) (string, error) {
	body, err := c.doSend(req)
	if err == nil || !isRetryable403(err, c.headers) {
		return body, err
	}
	// 403 + 默认 okhttp UA（规则没显式配 UA）：黑狐影院等站点定向封
	// TVBox UA，换浏览器 UA 重试一次。
	req2 := req.Clone(req.Context())
	if req2.Body != nil {
		// 重放原请求体（POST 等）；请求体是 *strings.Reader / *bytes.Reader
		// 时可复用，否则读回后重建（这里引擎只用 strings.Reader 传 POST body）。
		if r, ok := req.Body.(io.ReadSeeker); ok {
			_, _ = r.Seek(0, io.SeekStart)
			req2.Body = req.Body
		}
	}
	req2.Header.Set("User-Agent", browserUA)
	body2, err2 := c.doSend(req2)
	if err2 != nil {
		return body, err // 重试仍失败时返回第一个错误（含 403 上下文）
	}
	return body2, nil
}

// isRetryable403 判断该错误是否值得做 UA 重试：HTTP 403 + 规则没有显式
// 配置 User-Agent（若显式配了说明规则作者明确要这个 UA，不该被覆盖）。
func isRetryable403(err error, headers map[string]string) bool {
	if err == nil || !strings.Contains(err.Error(), "返回 HTTP 403") {
		return false
	}
	for _, k := range []string{"User-Agent", "user-agent", "UA", "ua"} {
		if _, ok := headers[k]; ok {
			return false
		}
	}
	return true
}

func (c *Client) doSend(req *http.Request) (string, error) {
	// 默认只在请求未带 UA 时给 okhttp；403 重试已换成浏览器 UA，不能覆盖。
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", tvboxUA)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	c.mu.Lock()
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	c.mu.Unlock()

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", req.URL, err)
	}
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		c.mu.Lock()
		c.cookies[ck.Name] = ck.Value
		c.mu.Unlock()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("请求 %s 返回 HTTP %d", req.URL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return "", fmt.Errorf("读取 %s 响应失败: %w", req.URL, err)
	}
	if len(body) > maxBodySize {
		return "", fmt.Errorf("响应超过 %d 字节限制: %s", maxBodySize, req.URL)
	}
	return string(body), nil
}
