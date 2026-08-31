package crawler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const crawlerUA = "okhttp/3.12.11"

type reqOpts struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Data    string            `json:"data"`
	Timeout int64             `json:"timeout"`
}

// installReq 把 req(url, opts) 注入 vm。opts.data 为字符串时作 body。
func (e *Engine) installReq(hc *http.Client) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	e.hc = hc
	_ = e.vm.Set("req", func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		var o reqOpts
		if !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			obj := call.Argument(1).ToObject(e.vm)
			o.Method = valueString(obj.Get("method"))
			o.Data = valueString(obj.Get("data"))
			if value := obj.Get("timeout"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
				o.Timeout = value.ToInteger()
			}
			if hv := obj.Get("headers"); hv != nil && !goja.IsUndefined(hv) && !goja.IsNull(hv) {
				var headers map[string]string
				if err := e.vm.ExportTo(hv, &headers); err == nil {
					o.Headers = headers
				}
			}
		}
		method := strings.ToUpper(o.Method)
		if method == "" {
			method = http.MethodGet
		}
		var body io.Reader
		if o.Data != "" {
			body = strings.NewReader(o.Data)
		}
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			panic(e.vm.NewGoError(fmt.Errorf("req 构造请求失败: %w", err)))
		}
		for k, v := range e.headers {
			req.Header.Set(k, v)
		}
		for k, v := range o.Headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", crawlerUA)
		}
		if len(e.cookies) > 0 && req.Header.Get("Cookie") == "" {
			parts := make([]string, 0, len(e.cookies))
			for k, v := range e.cookies {
				parts = append(parts, k+"="+v)
			}
			req.Header.Set("Cookie", strings.Join(parts, "; "))
		}
		client := hc
		if o.Timeout > 0 {
			copyClient := *hc
			copyClient.Timeout = time.Duration(o.Timeout) * time.Millisecond
			client = &copyClient
		}
		resp, err := client.Do(req)
		if err != nil {
			panic(e.vm.NewGoError(fmt.Errorf("req 请求失败: %w", err)))
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		if err != nil {
			panic(e.vm.NewGoError(fmt.Errorf("req 读响应失败: %w", err)))
		}
		out := e.vm.NewObject()
		_ = out.Set("content", decodeBody(b, resp.Header.Get("Content-Type")))
		_ = out.Set("statusCode", resp.StatusCode)
		if resp.Request != nil && resp.Request.URL != nil {
			_ = out.Set("finalUrl", resp.Request.URL.String())
		} else {
			_ = out.Set("finalUrl", url)
		}
		headers := e.vm.NewObject()
		for k, values := range resp.Header {
			_ = headers.Set(k, strings.Join(values, ", "))
		}
		_ = out.Set("headers", headers)
		for _, c := range resp.Cookies() {
			e.cookies[c.Name] = c.Value
		}
		return out
	})
}

func decodeBody(b []byte, contentType string) string {
	contentType = strings.ToLower(contentType)
	gbk := strings.Contains(contentType, "gbk") || strings.Contains(contentType, "gb2312")
	if !gbk && (contentType != "" || utf8.Valid(b)) {
		return string(b)
	}
	decoded, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), string(b))
	if err != nil {
		return string(b)
	}
	return decoded
}

func valueString(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	return value.String()
}
