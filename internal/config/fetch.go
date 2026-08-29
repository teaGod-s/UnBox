package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// okhttpUA 是 FongMi多线路 安卓客户端使用的 User-Agent。实测发现部分源站会依据
// UA 决定返回内容甚至直接拒绝访问，因此这里伪装成同一客户端。
const okhttpUA = "okhttp/3.12.11"

// fetchTimeout 是单次 HTTP 拉取的超时时间。真实配置正文都很小
// （约 400 字节到 114 KB），15 秒足以覆盖慢速网络；更短的超时配合
// 并发拉取，能让死掉的源快速失败，避免一个死 URL 拖住整次导入。
const fetchTimeout = 15 * time.Second

// Fetcher 负责获取配置内容，支持 http(s)://、file:// 以及裸本地路径。
//
// clan:// 是 FongMi多线路 安卓客户端用于引用其自身本地仓库内文件的私有协议，
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

// NewFetcherWithTimeout 返回一个指定超时的 Fetcher。直播源 m3u 数量多、
// 死源占比高，用更短的超时让死源快速失败，避免拖慢整次导入。
func NewFetcherWithTimeout(d time.Duration) *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: d}}
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
		return nil, fmt.Errorf("clan:// 是 FongMi多线路 安卓客户端本地仓库协议，脱离其运行环境无法解析，不支持: %s", ref)

	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return f.fetchHTTP(ctx, ref)

	case strings.HasPrefix(ref, "file://"):
		return os.ReadFile(localPath(ref))

	default:
		if scheme, ok := unsupportedScheme(ref); ok {
			return nil, fmt.Errorf("不支持的协议 %q，仅支持 http、https、file 及裸本地路径: %s", scheme, ref)
		}
		return os.ReadFile(localPath(ref))
	}
}

// localPath 将 file:// URL 或裸本地路径转换为可交给 os.ReadFile 的路径。
//
// file:// URL 的规范形式在不同平台上并不统一：Windows 下常见的形式
// 是 "file:///C:/x/y.json"（三个斜杠），若只是简单去掉 "file://" 前缀，
// 会留下一个多余的前导斜杠 "/C:/x/y.json"，这在 Windows 上不是合法路径。
// POSIX 下 "file:///tmp/x.json" 的前导斜杠则是路径本身的一部分，必须保留。
// 因此只在紧跟着看起来像盘符（单个 ASCII 字母 + 冒号）时才去掉前导斜杠。
func localPath(ref string) string {
	if !strings.HasPrefix(ref, "file://") {
		return ref
	}
	p := strings.TrimPrefix(ref, "file://")
	if len(p) >= 3 && p[0] == '/' && isASCIILetter(p[1]) && p[2] == ':' {
		p = p[1:]
	}
	// 真实订阅路径和 Windows 路径都可能含空格（%20 等），file:// URL 中
	// 这些字符会被百分号编码。用 net/url 而非手写逻辑解码；解码失败时
	// 回退到原始值而不是报错，因为此时原始值大概率本就是未编码的路径。
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	return p
}

// unsupportedScheme 检测 ref 是否形如 "<scheme>://..."，但不是 Fetch
// 支持的协议之一。仅在存在 "://" 时才判定为协议，这样像 Windows 路径
// "C:\x\y.json"（冒号后紧跟反斜杠而不是双斜杠）之类的裸路径不会被
// 误判为协议。
//
// 按 RFC 3986 验证 scheme：首字符必须是 ASCII 字母，后续字符可以是
// ASCII 字母、ASCII 数字、加号、减号、点号。不符合规范的 ref 返回
// ("", false) 以作为裸路径处理。
func unsupportedScheme(ref string) (string, bool) {
	i := strings.Index(ref, "://")
	if i <= 0 {
		return "", false
	}
	scheme := ref[:i]

	// 验证 scheme 首字符必须是 ASCII 字母
	if !isASCIILetter(scheme[0]) {
		return "", false
	}

	// 验证后续字符：ASCII 字母、数字、+、-、.
	for j := 1; j < len(scheme); j++ {
		b := scheme[j]
		if !isASCIILetter(b) && !isASCIIDigit(b) && b != '+' && b != '-' && b != '.' {
			return "", false
		}
	}

	// 检查是否在已支持的协议列表中
	switch strings.ToLower(scheme) {
	case "http", "https", "file", "clan":
		return "", false
	}
	return scheme, true
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
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
