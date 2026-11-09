package crawler

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func (e *Engine) installHelpers() {
	_ = e.vm.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]any, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a.Export()
		}
		log.Print(args...)
		return goja.Undefined()
	})
	_ = e.vm.Set("print", func(call goja.FunctionCall) goja.Value {
		args := make([]any, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a.Export()
		}
		log.Print(args...)
		return goja.Undefined()
	})
	urlHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return e.vm.ToValue("")
		}
		if len(call.Arguments) == 1 {
			return e.vm.ToValue(call.Argument(0).String())
		}
		return e.vm.ToValue(joinURL(call.Argument(0).String(), call.Argument(1).String()))
	}
	_ = e.vm.Set("urljoin2", urlHelper)
	_ = e.vm.Set("buildUrl", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return urlHelper(call)
		}
		if _, ok := call.Argument(1).Export().(string); ok {
			return urlHelper(call)
		}
		base := call.Argument(0).String()
		params := call.Argument(1).ToObject(e.vm)
		values := params.Export()
		if m, ok := values.(map[string]any); ok {
			u, parseErr := url.Parse(base)
			if parseErr == nil {
				query := u.Query()
				for key, value := range m {
					query.Set(key, valueString(e.vm.ToValue(value)))
				}
				u.RawQuery = query.Encode()
				return e.vm.ToValue(u.String())
			}
		}
		return urlHelper(call)
	})
	_ = e.vm.Set("urlDeal", urlHelper)
	fetch := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return e.vm.ToValue("")
		}
		opts := e.vm.NewObject()
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			opts = call.Argument(1).ToObject(e.vm)
		}
		value, err := e.Call("req", e.vm.ToValue(call.Argument(0).String()), opts)
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		return value.ToObject(e.vm).Get("content")
	}
	_ = e.vm.Set("fetch", fetch)
	_ = e.vm.Set("request", fetch)
	// http(url, opts) 是 FongMi 脚本里的另一个取正文原语（等价于 fetch）：
	// 真实脚本会写 `this.content = http(url, {headers:{Referer: ...}})`。
	// 不改名注入，脚本会直接报 `ReferenceError: http is not defined`。
	_ = e.vm.Set("http", fetch)
	_ = e.vm.Set("fetch_params", e.vm.NewObject())
	_ = e.vm.Set("base64Encode", func(call goja.FunctionCall) goja.Value {
		return e.vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	})
	_ = e.vm.Set("base64Decode", func(call goja.FunctionCall) goja.Value {
		b, _ := base64.StdEncoding.DecodeString(call.Argument(0).String())
		return e.vm.ToValue(string(b))
	})
	_ = e.vm.Set("md5", func(call goja.FunctionCall) goja.Value {
		h := md5.Sum([]byte(call.Argument(0).String()))
		return e.vm.ToValue(hex.EncodeToString(h[:]))
	})
	// md5X 是 FongMi 引擎脚本的全局 md5 别名（金牌影视等脚本
	// function md5Text(v){ return md5X(v) }），语义与 md5 完全一致。
	_ = e.vm.Set("md5X", func(call goja.FunctionCall) goja.Value {
		h := md5.Sum([]byte(call.Argument(0).String()))
		return e.vm.ToValue(hex.EncodeToString(h[:]))
	})
	_ = e.vm.Set("sleep", func(call goja.FunctionCall) goja.Value {
		time.Sleep(time.Duration(call.Argument(0).ToInteger()) * time.Millisecond)
		return goja.Undefined()
	})
	_ = e.vm.Set("cookie", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if len(call.Arguments) > 1 {
			e.cookies[key] = call.Argument(1).String()
			return goja.Undefined()
		}
		return e.vm.ToValue(e.cookies[key])
	})
	_ = e.vm.Set("header", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if len(call.Arguments) > 1 {
			e.headers[key] = call.Argument(1).String()
			return goja.Undefined()
		}
		return e.vm.ToValue(e.headers[key])
	})
	_ = e.vm.Set("join", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return e.vm.ToValue("")
		}
		var values []string
		_ = e.vm.ExportTo(call.Argument(0), &values)
		sep := ""
		if len(call.Arguments) > 1 {
			sep = call.Argument(1).String()
		}
		return e.vm.ToValue(strings.Join(values, sep))
	})
	// console shim：goja 无 console 全局，drpy2/ESM 脚本的 catch 日志会抛
	// ReferenceError 并中断动作；注入最小 console 映射到既有日志输出。
	// 参数按空格连接（浏览器 console 语义），不依赖 fmt.Sprint 的相邻字符串规则。
	console := e.vm.NewObject()
	logConsole := func(level string, call goja.FunctionCall) goja.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, a := range call.Arguments {
			parts = append(parts, consoleArgString(a))
		}
		log.Print("[console." + level + "] " + strings.Join(parts, " "))
		return goja.Undefined()
	}
	for _, level := range []string{"log", "info", "warn", "error", "debug"} {
		level := level
		_ = console.Set(level, func(call goja.FunctionCall) goja.Value {
			return logConsole(level, call)
		})
	}
	// 其余 console 方法补 no-op，避免脚本因缺方法中断（时间/分组/断言等
	// 在同步运行时没有实际语义）。
	for _, name := range []string{
		"time", "timeEnd", "timeLog", "group", "groupCollapsed", "groupEnd",
		"trace", "assert", "table", "dir", "dirxml", "count", "countReset",
	} {
		_ = console.Set(name, func(call goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
	}
	_ = e.vm.Set("console", console)
	// setTimeout shim：goja 无定时器全局，drpy2 脚本常用 setTimeout 做日志/延迟
	// （如 lf_search3_min.js 的 10ms console.log）。这里同步执行回调，并把等待
	// 上限压在 1000ms，避免脚本把引擎卡死；不承载真正异步定时的语义。
	_ = e.vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return e.vm.ToValue(0)
		}
		ms := call.Argument(1).ToInteger()
		if ms > 0 {
			if ms > 1000 {
				ms = 1000
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		if _, err := fn(goja.Undefined()); err != nil {
			log.Printf("[console.error] setTimeout 回调执行失败: %v", err)
		}
		return e.vm.ToValue(0)
	})
	// clearTimeout 为 no-op：同步 shim 的回调在返回前就已执行完，无待取消的定时器。
	_ = e.vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	installUAConstants(e)
}

// 常用 UA 常量。drpy2 引擎库在自身脚本顶部定义它们，而 UnBox 加载的是站点
// 规则（ext）而不是引擎库，规则里直接引用 PC_UA 就会报 ReferenceError。
// 取值与 drpy2.min.js 保持一致。
const (
	uaMobile = "Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/90.0.4430.91 Mobile Safari/537.36"
	uaPC = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/95.0.4638.54 Safari/537.36"
	uaUC = "Mozilla/5.0 (Linux; U; Android 9; zh-CN; MI 9 Build/PKQ1.181121.001) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/57.0.2987.108 " +
		"UCBrowser/12.5.5.1035 Mobile Safari/537.36"
	uaIOS = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) " +
		"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"
)

// installUAConstants 注入 drpy2 约定的 UA 全局常量。
func installUAConstants(e *Engine) {
	for name, value := range map[string]string{
		"MOBILE_UA": uaMobile,
		"PC_UA":     uaPC,
		"UC_UA":     uaUC,
		"IOS_UA":    uaIOS,
		"UA":        "Mozilla/5.0",
	} {
		_ = e.vm.Set(name, e.vm.ToValue(value))
	}
}

// consoleArgString 把 console 参数转成文本；undefined/null 用 JS 拼写，
// 其余走 fmt.Sprint（对象显示为 Go 结构，仅用于日志排查）。
func consoleArgString(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	return fmt.Sprint(v.Export())
}

func (e *Engine) setFetchParams(rule *Rule) {
	params := e.vm.NewObject()
	if rule != nil {
		if len(rule.Headers) > 0 {
			headers := e.vm.NewObject()
			for key, value := range rule.Headers {
				_ = headers.Set(key, value)
			}
			_ = params.Set("headers", headers)
		}
		if rule.Timeout > 0 {
			_ = params.Set("timeout", rule.Timeout)
		}
	}
	_ = e.vm.Set("fetch_params", params)
}
