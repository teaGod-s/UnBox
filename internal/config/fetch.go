package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// okhttpUA 是 TVBox 安卓客户端使用的 User-Agent。实测发现部分源站会依据
// UA 决定返回内容甚至直接拒绝访问，因此这里伪装成同一客户端。
const okhttpUA = "okhttp/3.12.11"

// fetchTimeout 是单次 HTTP 拉取的超时时间。真实配置正文都很小
// （约 400 字节到 114 KB），60 秒足以覆盖慢速网络，同时避免
// 恶意或卡死的服务端无限期占用连接。
const fetchTimeout = 60 * time.Second

// Fetcher 负责获取配置内容，支持 http(s)://、file:// 以及裸本地路径。
//
// clan:// 是 TVBox 安卓客户端用于引用其自身本地仓库内文件的私有协议，
// 在桌面端没有对应含义，无法解析，Fetch 会直接返回明确错误。
type Fetcher struct {
	Client *http.Client
}

// NewFetcher 返回一个带有显式超时的 Fetcher。
//
// 不使用 http.DefaultClient ——它没有超时，一个卡死或蓄意放慢响应的
// 服务端会导致调用方永久阻塞。
func NewFetcher() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: fetchTimeout}}
}

// Fetch 获取 ref 指向的配置原始字节。
//
// ref 可以是：
//   - http:// 或 https:// URL
//   - file:// URL
//   - 裸本地文件路径
//
// clan:// 会被明确拒绝，而不是静默返回空内容或尝试当作本地路径读取。
func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error) {
	switch {
	case strings.HasPrefix(ref, "clan://"):
		return nil, fmt.Errorf("clan:// 是 TVBox 安卓客户端本地仓库协议，脱离其运行环境无法解析，不支持: %s", ref)

	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return f.fetchHTTP(ctx, ref)

	case strings.HasPrefix(ref, "file://"):
		return os.ReadFile(strings.TrimPrefix(ref, "file://"))

	default:
		return os.ReadFile(ref)
	}
}

func (f *Fetcher) fetchHTTP(ctx context.Context, ref string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", okhttpUA)

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", ref, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("请求 %s 返回非成功状态: %d %s", ref, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// 读取 maxDecodedSize+1 字节：如果读满了上限+1，说明正文超限，
	// 返回明确错误而不是静默截断（截断后的配置会在后续解析阶段
	// 产生令人困惑的语法错误，而不是诚实的网络错误）。
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDecodedSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 响应体失败: %w", ref, err)
	}
	if len(body) > maxDecodedSize {
		return nil, fmt.Errorf("响应体超过 %d 字节限制: %s", maxDecodedSize, ref)
	}
	return body, nil
}
