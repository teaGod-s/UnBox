package config

import "testing"

func TestResolveRelativeURLs(t *testing.T) {
	cfg := &Config{
		SourceURL: "https://cfg.example.com/feed/api.json",
		Spider:    "./jar/XYQ.jar;md5;705ba6e23c4384c37a11dd904727520b",
		Sites: []Site{
			{Key: "dot-slash", API: "./lib/drpy2.min.js"},
			{Key: "root-path", API: "/spider.js"},
			{Key: "bare", API: "spider.js"},
			{Key: "parent", API: "../common/a.js"},
			{Key: "absolute", API: "https://cdn.example.com/down.php/x.js"},
			{Key: "protocol-relative", API: "//cdn.example.com/x.js"},
			{Key: "jar-class", API: "csp_WexAppV7Guard"},
			{Key: "asset", API: "assets://js/lib/cheerio.min.js"},
			{Key: "empty", API: ""},
			{Key: "drpy2-ext", Ext: "./dr/js/某影视源.js"},
			{Key: "json-ext", Ext: `{"a":1}`},
			{Key: "object-ext", Ext: map[string]any{"a": 1}},
			{Key: "absolute-ext", Ext: "https://cdn.example.com/x/%E4%B8%AD.js"},
			{Key: "protocol-relative-ext", Ext: "//cdn.example.com/x/%E4%B8%AD.js"},
		},
		Lives: LiveList{{Name: "本地", URL: "./live/iptv.m3u"}},
	}

	ResolveRelativeURLs(cfg)

	want := map[string]string{
		"dot-slash":         "https://cfg.example.com/feed/lib/drpy2.min.js",
		"root-path":         "https://cfg.example.com/spider.js",
		"bare":              "https://cfg.example.com/feed/spider.js",
		"parent":            "https://cfg.example.com/common/a.js",
		"absolute":          "https://cdn.example.com/down.php/x.js",
		"protocol-relative": "//cdn.example.com/x.js",
		"jar-class":         "csp_WexAppV7Guard",
		"asset":             "assets://js/lib/cheerio.min.js",
		"empty":             "",
	}
	for _, site := range cfg.Sites {
		if got := site.API; got != want[site.Key] {
			t.Errorf("站点 %s: API = %q，期望 %q", site.Key, got, want[site.Key])
		}
	}
	for _, site := range cfg.Sites {
		switch site.Key {
		case "drpy2-ext":
			if site.ExtString() != "https://cfg.example.com/feed/dr/js/某影视源.js" {
				t.Errorf("ext 未解析: %v", site.Ext)
			}
		case "json-ext":
			if site.ExtString() != `{"a":1}` {
				t.Errorf("JSON 字符串 ext 不应被改写: %v", site.Ext)
			}
		case "object-ext":
			if _, ok := site.Ext.(map[string]any); !ok {
				t.Errorf("对象 ext 不应被改写: %v", site.Ext)
			}
		case "absolute-ext":
			if site.ExtString() != "https://cdn.example.com/x/%E4%B8%AD.js" {
				t.Errorf("绝对地址 ext 不应被改写: %v", site.Ext)
			}
		case "protocol-relative-ext":
			if site.ExtString() != "//cdn.example.com/x/%E4%B8%AD.js" {
				t.Errorf("协议相对 ext 不应被改写: %v", site.Ext)
			}
		}
	}
	if got := cfg.Lives[0].URL; got != "https://cfg.example.com/feed/live/iptv.m3u" {
		t.Errorf("直播 URL = %q", got)
	}
	wantSpider := "https://cfg.example.com/feed/jar/XYQ.jar;md5;705ba6e23c4384c37a11dd904727520b"
	if cfg.Spider != wantSpider {
		t.Errorf("Spider = %q，期望 %q", cfg.Spider, wantSpider)
	}
}

// restoreNonASCII 的边界：只还原 >= 0x80 的字节；ASCII 转义与 malformed 序列
// 必须原样保留，且不能 panic。
func TestRestoreNonASCIIBoundaries(t *testing.T) {
	cases := map[string]string{
		"a%2Fb":                     "a%2Fb",
		"a%20b":                     "a%20b",
		"%2F":                       "%2F",
		"%20":                       "%20",
		"%":                         "%",
		"%2":                        "%2",
		"%ZZ":                       "%ZZ",
		"%E4%B8%AD":                 "中",
		"%e4%b8%ad":                 "中",
		"https://x/%E4%B8%AD.js":    "https://x/中.js",
		"https://x/a%2Fb/%E4%B8%AD": "https://x/a%2Fb/中",
	}
	for in, want := range cases {
		if got := restoreNonASCII(in); got != want {
			t.Errorf("restoreNonASCII(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveRelativeURLsWithoutSourceURL(t *testing.T) {
	cfg := &Config{Sites: []Site{{Key: "a", API: "./x.js"}}}
	ResolveRelativeURLs(cfg)
	if cfg.Sites[0].API != "./x.js" {
		t.Fatalf("无来源地址时不应改写: %q", cfg.Sites[0].API)
	}
	ResolveRelativeURLs(nil) // 不应 panic
}
