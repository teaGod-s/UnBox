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
	req.Header.Set("User-Agent", tvboxUA)
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
