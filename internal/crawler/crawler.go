// Package crawler 在桌面端运行 TVBox 爬虫 JS（goja 运行时）。
// 本包不依赖 provider/Wails，只暴露爬虫原语与返回类型。
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dop251/goja"
)

// Engine 封装一个 goja VM，加载并运行单个爬虫脚本。
type Engine struct {
	vm      *goja.Runtime
	hc      *http.Client
	headers map[string]string
	cookies map[string]string
}

// New 返回已安装 TVBox 爬虫原语的引擎。
func New() *Engine {
	e := &Engine{
		vm:      goja.New(),
		hc:      &http.Client{Timeout: 30 * time.Second},
		headers: make(map[string]string),
		cookies: make(map[string]string),
	}
	e.installReq(e.hc)
	e.installRule()
	e.installHelpers()
	return e
}

// Load 执行一段爬虫 JS（声明式 rule 或函数定义），失败返回错误。
func (e *Engine) Load(src string) error {
	if e == nil || e.vm == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
	_, err := e.vm.RunString(src)
	return err
}

// LoadFromURL 下载并执行一个 JS 爬虫文件。
func (e *Engine) LoadFromURL(ctx context.Context, u string) error {
	if e == nil || e.hc == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", crawlerUA)
	resp, err := e.hc.Do(req)
	if err != nil {
		return fmt.Errorf("下载爬虫失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载爬虫失败: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("读取爬虫失败: %w", err)
	}
	return e.Load(string(b))
}

// Call 调用爬虫脚本里定义的全局函数 name，返回其 JS 值。
func (e *Engine) Call(name string, args ...goja.Value) (goja.Value, error) {
	if e == nil || e.vm == nil {
		return nil, fmt.Errorf("爬虫引擎未初始化")
	}
	fn, ok := goja.AssertFunction(e.vm.Get(name))
	if !ok {
		return nil, fmt.Errorf("爬虫未定义函数 %s", name)
	}
	return fn(goja.Undefined(), args...)
}
