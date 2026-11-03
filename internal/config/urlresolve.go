package config

import (
	"net/url"
	"strings"
)

// ResolveRelativeURLs 把站点与直播源里写成相对路径的地址解析成绝对地址。
//
// 真实配置里常见 `./lib/drpy2.min.js`、`/spider.js` 这类写法，它们是相对于配置
// 自身地址的。不解析就会把相对路径当 URL 去请求，报
// `Get "./lib/drpy2.min.js": unsupported protocol scheme ""`。
//
// 只处理相对地址：已带协议（http/https/assets 等）、协议相对（//host/x）、
// 以及 `csp_` 这种蜘蛛类名保持原样。cfg.SourceURL 为空时不做任何改动。
func ResolveRelativeURLs(cfg *Config) {
	if cfg == nil || cfg.SourceURL == "" {
		return
	}
	base, err := url.Parse(cfg.SourceURL)
	if err != nil || base.Host == "" {
		return
	}
	if cfg.Spider != "" {
		cfg.Spider = resolveSpider(base, cfg.Spider)
	}
	for i := range cfg.Sites {
		cfg.Sites[i].API = resolveRelativeURL(base, cfg.Sites[i].API)
		cfg.Sites[i].Ext = resolveExt(base, cfg.Sites[i].Ext)
	}
	for i := range cfg.Lives {
		cfg.Lives[i].URL = resolveRelativeURL(base, cfg.Lives[i].URL)
	}
}

// resolveRelativeURL 把单个地址按 base 解析；不需要或无法解析时原样返回。
func resolveRelativeURL(base *url.URL, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || isSpiderClassName(trimmed) {
		return raw
	}
	ref, err := url.Parse(trimmed)
	if err != nil {
		return raw
	}
	// 已有协议（http://、assets://）或协议相对（//host/x）时不改写。
	if ref.Scheme != "" || ref.Host != "" {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// isSpiderClassName 判断是否是蜘蛛类名（csp_xxx），它不是地址。
func isSpiderClassName(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "csp_")
}

// resolveSpider 解析 spider 的地址部分，保留 ;md5; 后缀。
func resolveSpider(base *url.URL, raw string) string {
	d := ParseSpider(raw)
	if d.Path == "" {
		return raw
	}
	d.Path = resolveRelativeURL(base, d.Path)
	return d.String()
}

// resolveExt 只解析字符串型 ext；JSON 字符串与对象/数组原样保留。
func resolveExt(base *url.URL, ext any) any {
	s, ok := ext.(string)
	if !ok {
		return ext
	}
	t := strings.TrimSpace(s)
	if t == "" || strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return ext
	}
	out := resolveRelativeURL(base, s)
	// 绝对地址、协议相对、assets://、csp_ 类名与解析失败都会原样返回；
	// 这类值不应被 restoreNonASCII 改动任何字节。
	if out == s {
		return ext
	}
	return restoreNonASCII(out)
}

// restoreNonASCII 把 url.URL.String() 对非 ASCII 字节做的百分号编码还原成原文。
// 配置里的中文文件名（如 360影视.js）通常原样书写，保留转义会让上层拼路径时对不上。
// 只还原 >= 0x80 的字节，%20、%2F 这类 ASCII 转义保持原样，避免改变路径语义。
func restoreNonASCII(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				if v := hi<<4 | lo; v >= 0x80 {
					b.WriteByte(v)
					i += 3
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
