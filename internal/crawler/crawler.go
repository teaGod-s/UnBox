// Package crawler 在桌面端运行 FongMi多线路 爬虫 JS（goja 运行时）。
// 本包不依赖 provider/Wails，只暴露爬虫原语与返回类型。
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// Engine 封装一个 goja VM，加载并运行单个爬虫脚本。
type Engine struct {
	vm          *goja.Runtime
	hc          *http.Client
	headers     map[string]string
	cookies     map[string]string
	initialized bool
}

// New 返回已安装 FongMi多线路 爬虫原语的引擎。
func New() *Engine {
	e := &Engine{
		vm:      goja.New(),
		hc:      &http.Client{Timeout: 30 * time.Second},
		headers: make(map[string]string),
		cookies: make(map[string]string),
	}
	e.vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	e.installReq(e.hc)
	e.installRule()
	e.installHelpers()
	installMuban(e)
	return e
}

// Load 执行一段爬虫 JS（声明式 rule 或函数定义），失败返回错误。
func (e *Engine) Load(src string) error {
	if e == nil || e.vm == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
	_, err := e.vm.RunString(normalizeModuleSource(src))
	if err != nil {
		return err
	}
	// FongMi JS0 scripts expose actions through export default. Copy exported
	// methods to global scope to keep one action dispatcher for all script forms.
	value := e.vm.Get("__crawler_exports")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	object := value.ToObject(e.vm)
	for _, key := range object.Keys() {
		_ = e.vm.Set(key, object.Get(key))
	}
	return nil
}

// asyncFnRe / awaitRe 用词边界 + 跨空白符剥离 async/await，覆盖
// `await\n`、`await\t`、`async  function`（多空格）等形态，且不误伤
// `awaitable`、`asyncTask` 这类标识符。
var (
	asyncFnRe = regexp.MustCompile(`\basync\s+(function\b)`)
	awaitRe   = regexp.MustCompile(`\bawait\s+`)
)

func normalizeModuleSource(src string) string {
	// req() 在嵌入运行时中是同步的，JS0 动作只 await 注入的同步 API，
	// 因此把 async/await 归约为普通函数直接返回。仅支持 FongMi JS0 形态：
	//   async function 声明 + 同步 req + 末尾 export default {...}。
	// 已知局限：这是文本级剥离，字符串字面量里的 "await " 会被一并改写，
	// 无法靠正则区分。真实爬虫若在字符串里含 "await "，需升级到 goja 原生
	// Promise 处理（M5.1 真实爬虫验收时校准）。
	src = asyncFnRe.ReplaceAllString(src, "$1")
	src = awaitRe.ReplaceAllString(src, "")
	if index := strings.LastIndex(src, "export default"); index >= 0 {
		src = src[:index] + "var __crawler_exports = " + src[index+len("export default"):]
	}
	return src
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

// Init runs the optional module initialization action once.
func (e *Engine) Init() error {
	if e.initialized {
		return nil
	}
	if hasFunction(e, "init") {
		if _, err := e.Call("init"); err != nil {
			return err
		}
	}
	e.initialized = true
	return nil
}
