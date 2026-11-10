// Package crawler 在桌面端运行 FongMi多线路 爬虫 JS（goja 运行时）。
// 本包不依赖 provider/Wails，只暴露爬虫原语与返回类型。
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/unbox/unbox/internal/httpx"
)

// ErrESModuleUnsupported 表示脚本用了当前加载器不支持的 ES Module 语法。
var ErrESModuleUnsupported = errors.New(
	"脚本使用了当前加载器不支持的 ES Module 语法（支持 import、export default、export function/const/let/var、export {}）")

// Engine 封装一个 goja VM，加载并运行单个爬虫脚本。
type Engine struct {
	vm          *goja.Runtime
	hc          *http.Client
	headers     map[string]string
	cookies     map[string]string
	initialized bool
	// baseURL 是主脚本自身的地址，用于把 req() 里的相对路径解析成绝对地址。
	baseURL string
	// ext 是站点配置里的 ext 字段，作为 init(ext) 的参数传给脚本。
	// 真实脚本（如 lf_p2p2_min.js）会直接读它，例如 ext.match(/18\+/)。
	ext string
	// siteKey / siteType 供 init(cfg) 形态的脚本读取（cfg.skey / cfg.stype）。
	siteKey  string
	siteType int
	// ctx 是本次调用的上下文（全站搜索用它给单站点限时）；JS 里的 req/http
	// 都带它跑，否则一个连不上的站点会按自己的 15s 超时拖住线程几分钟。
	// 由 ctxMu 守护：同一站点的搜索与详情可能并发（详情页预热、搜索重入），
	// 裸读会让一次请求拿到别人那次调用的 ctx，被别人结束时的 cancel 殃及。
	ctxMu sync.RWMutex
	ctx   context.Context
}

// SetContext 绑定本次调用的上下文（搜索/详情等）。JS 里的 req 请求会携带它，
// 上层可以借用它给单个站点限时（siteSearchTimeout）。
func (e *Engine) SetContext(ctx context.Context) {
	e.ctxMu.Lock()
	e.ctx = ctx
	e.ctxMu.Unlock()
}

// activeCtx 返回请求要用的上下文：绑定了就用绑定的值，否则退回 Background
// （交给 e.hc 自带的客户端超时兜底）。
func (e *Engine) activeCtx() context.Context {
	e.ctxMu.RLock()
	ctx := e.ctx
	e.ctxMu.RUnlock()
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// New 返回已安装 FongMi多线路 爬虫原语的引擎。
func New() *Engine {
	e := &Engine{
		vm:      goja.New(),
		hc:      httpx.NewClient(30 * time.Second),
		headers: make(map[string]string),
		cookies: make(map[string]string),
	}
	e.vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	e.installReq(e.hc)
	e.installRule()
	e.installHelpers()
	e.installGBK()
	installMuban(e)
	return e
}

// Load 执行一段爬虫 JS（声明式 rule 或函数定义），失败返回错误。
func (e *Engine) Load(src string) error {
	return e.loadWithContext(context.Background(), src, "")
}

func (e *Engine) loadWithContext(ctx context.Context, src, baseURL string) error {
	if e == nil || e.vm == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
	e.baseURL = baseURL
	if needsModuleLoader(src) {
		return e.loadESM(ctx, src, baseURL)
	}
	_, err := e.vm.RunString(normalizeModuleSource(src))
	if err != nil {
		if esModuleRe.MatchString(src) {
			return fmt.Errorf("%w（原始错误: %v）", ErrESModuleUnsupported, err)
		}
		return err
	}
	return e.promoteModuleExports(e.vm.Get("__crawler_exports"))
}

// asyncFnRe / asyncArrowRe / awaitRe 用词边界 + 跨空白符剥离 async/await，
// 覆盖 `await\n`、`await\t`、`async  function`（多空格）、`async (x) =>`、
// `async x =>` 等形态，且不误伤 `awaitable`、`asyncTask` 这类标识符。
var (
	asyncFnRe    = regexp.MustCompile(`\basync\s+(function\b)`)
	asyncArrowRe = regexp.MustCompile(`\basync\s+(\([^)]*\)\s*=>|[\p{L}_$][\p{L}\p{N}_$]*\s*=>)`)
	awaitRe      = regexp.MustCompile(`\bawait\s+`)
	// esModuleRe 匹配顶层的 ES Module import/export 语句。
	esModuleRe = regexp.MustCompile(`(?m)^\s*(import|export)\s`)
)

// stripAsyncAwait 把 async/await 归约为同步写法。
// 已知局限与 normalizeModuleSource 相同：字符串字面量里的 "await " 会被一并改写。
func stripAsyncAwait(src string) string {
	src = asyncFnRe.ReplaceAllString(src, "$1")
	src = asyncArrowRe.ReplaceAllString(src, "$1")
	return awaitRe.ReplaceAllString(src, "")
}

func normalizeModuleSource(src string) string {
	// req() 在嵌入运行时中是同步的，JS0 动作只 await 注入的同步 API，
	// 因此把 async/await 归约为普通函数直接返回。仅支持 FongMi JS0 形态：
	//   async function 声明 + 同步 req + 末尾 export default {...}。
	// 已知局限：这是文本级剥离，字符串字面量里的 "await " 会被一并改写，
	// 无法靠正则区分。真实爬虫若在字符串里含 "await "，需升级到 goja 原生
	// Promise 处理（M5.1 真实爬虫验收时校准）。
	src = stripAsyncAwait(src)
	if index := strings.LastIndex(src, "export default"); index >= 0 {
		src = src[:index] + "var __crawler_exports = " + src[index+len("export default"):]
	}
	return src
}

// ruleScriptWrapper 把规则脚本包进函数作用域执行。
//
// 规则脚本普遍以 `let html=...` / `const x=...` 开头，而引擎按站点复用同一个
// VM：第二次执行同一条脚本时，顶层 let/const 会撞上
// "Identifier 'html' has already been declared"（某影视源详情第二次就报这个）。
// drpy2 用函数内的直接 eval 执行规则，let/const 天然不落到全局；这里用 IIFE
// 还原同样的作用域，同时把脚本想交回的 input / VOD 作为返回值带出来
// （脚本也常用 `var input = ...`，那种写法只活在函数作用域里，靠全局读不到）。
const ruleScriptWrapper = `(function(){
%s
return {input: (typeof input !== "undefined") ? input : undefined,
        VOD: (typeof VOD !== "undefined") ? VOD : undefined};
})()`

// runRuleScript 执行一条规则脚本，返回脚本作用域内的 input 与 VOD。
func (e *Engine) runRuleScript(script string) (input, vod goja.Value, err error) {
	value, err := e.vm.RunString(fmt.Sprintf(ruleScriptWrapper, script))
	if err != nil {
		return nil, nil, err
	}
	obj := value.ToObject(e.vm)
	if obj == nil {
		return nil, nil, nil
	}
	return obj.Get("input"), obj.Get("VOD"), nil
}

// LoadFromURL 下载并执行一个 JS 爬虫文件。
func (e *Engine) LoadFromURL(ctx context.Context, u string) error {
	if e == nil || e.hc == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
	parsed, err := url.Parse(strings.TrimSpace(u))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("爬虫地址不是完整 URL（%s）：配置里的相对路径未解析成功（若为旧版订阅快照遗留，重新导入一次该源即可）", u)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("爬虫地址协议不支持（%s）：仅支持 http/https 的爬虫脚本", u)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", crawlerUA)
	resp, err := httpx.Do(e.hc, req)
	if err != nil {
		return fmt.Errorf("下载爬虫失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载爬虫失败: %s 返回 HTTP %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("读取爬虫失败: %w", err)
	}
	// 源站失效时常把 HTML 错误页当脚本返回；直接交给 goja 只会得到
	// `SyntaxError: Line 1:1 Unexpected token <`，看不出真正原因。
	if looksLikeHTML(b) {
		return fmt.Errorf("下载爬虫失败: %s 返回了网页而不是 JS 脚本（该地址可能已失效或被拦截）", u)
	}
	return e.loadWithContext(ctx, string(b), u)
}

// looksLikeHTML 判断响应体是否是一个网页。
func looksLikeHTML(b []byte) bool {
	trimmed := strings.TrimSpace(string(b))
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "\ufeff"))
	if trimmed == "" {
		return false
	}
	if trimmed[0] == '<' {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html")
}

// resolveRequestURL 把 req() 收到的相对路径按主脚本地址补成绝对 URL。
//
// drpy2 规则里大量出现 req('/category/data?...') 这类以站点根为基准的写法，
// 直接交给 net/http 只会得到 `unsupported protocol scheme ""`。脚本地址本身
// 就在站点上，用它做 base 与 TVBox 客户端的行为一致。
func (e *Engine) resolveRequestURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || e.baseURL == "" {
		return raw
	}
	base, err := url.Parse(e.baseURL)
	if err != nil || base.Host == "" {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	// 已带协议（http/https/assets/data…）的地址保持原样。
	if ref.Scheme != "" {
		return raw
	}
	return base.ResolveReference(ref).String()
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
		// FongMi js0 的 init 需要一个参数；不传会让脚本里
		// `ext.match(...)` / `ext.indexOf(...)` 抛 TypeError。
		if _, err := e.Call("init", e.initArg()); err != nil {
			return err
		}
	}
	e.initialized = true
	return nil
}

// SetExt 记下站点 ext，供 init(ext) 使用。
func (e *Engine) SetExt(ext string) {
	if e == nil {
		return
	}
	e.ext = ext
}

// SetSiteMeta 记下站点 key 与类型，供 init(cfg) 形态读取 cfg.skey / cfg.stype。
func (e *Engine) SetSiteMeta(key string, siteType int) {
	if e == nil {
		return
	}
	e.siteKey = key
	e.siteType = siteType
}

// initArg 构造 init 的实参，兼容两种真实形态：
//
//   - init(ext)        —— ext 是普通字符串（如 lf_p2p2_min.js 的 ext.match）；
//   - init(cfg)        —— cfg = {skey, stype, ext:对象}（如 cat 系脚本的
//     bili_open.js，直接读 cfg.ext.hasOwnProperty）。
//
// 判断依据是 ext 本身是否是 JSON 对象/数组：是则按 cfg 形态传，否则按字符串传。
func (e *Engine) initArg() goja.Value {
	raw := strings.TrimSpace(e.ext)
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			cfg := e.vm.NewObject()
			_ = cfg.Set("skey", e.siteKey)
			_ = cfg.Set("stype", e.siteType)
			_ = cfg.Set("ext", e.vm.ToValue(parsed))
			return cfg
		}
	}
	return e.vm.ToValue(e.ext)
}
