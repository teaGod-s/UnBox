package crawler

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
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
