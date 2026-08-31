package crawler

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"log"
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
	_ = e.vm.Set("buildUrl", urlHelper)
	_ = e.vm.Set("urlDeal", urlHelper)
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
