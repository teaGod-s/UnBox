# UnBox M6.1 XBPQ JAR Rule Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 UnBox 以纯 Go 方式支持 `csp_XBPQ` 规则型 JAR 站点（分类/列表/搜索/详情/播放），不下载、不执行 JAR。

**Architecture:** 新增 `internal/provider/tvbox/rule` 子包：`client.go` 负责 HTTP，`extract.go` 负责 `A&&B` 标记提取，`rule.go` 解析 XBPQ 规则 JSON，`xbpq.go` 实现引擎；`internal/provider/tvbox/jar.go` 把引擎结果适配成现有 `provider.Provider`。配置层增加 `spider` 描述符解析与 `spider`/`ext` 相对路径解析。

**Tech Stack:** Go 1.26.3、标准库 `net/http`/`encoding/json`/`html`/`regexp`、现有 `provider`/`player`/`config` 包。不新增第三方依赖。

**Spec:** `docs/superpowers/specs/2026-10-17-unbox-jar-compat-design.md`

## Global Constraints

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/provider/tvbox/rule` 不得 import Wails。
- 公开错误信息 / 注释用中文。
- TDD：先写失败测试，再写实现。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；
  Linux 额外 `CGO_ENABLED=1 go build ./...`。
- M6.1 只支持 `csp_XBPQ`；未知 `csp_` 类必须返回明确错误，不得静默伪装可用。
- 不下载、不执行 JAR；只读取 `ext` 规则 JSON（HTTP/HTTPS/file）。

---

## File Structure

| 文件 | 职责 |
|---|---|
| `internal/config/spider.go` | `SpiderDescriptor` 与 `ParseSpider`（`;md5;` 后缀） |
| `internal/config/spider_test.go` | 描述符解析测试 |
| `internal/config/model.go` | 增加 `Site.ExtString()` |
| `internal/config/urlresolve.go` | 解析 `cfg.Spider` 与字符串型 `Site.Ext` |
| `internal/config/urlresolve_test.go` | 相对路径测试扩展 |
| `internal/config/classify.go` | `csp_` → `jar`；已知引擎 `SupportYes` |
| `internal/config/classify_test.go` | 分类测试更新 |
| `internal/provider/tvbox/rule/extract.go` | `Cut`/`CutList`/`Clean` |
| `internal/provider/tvbox/rule/extract_test.go` | 提取规则测试 |
| `internal/provider/tvbox/rule/rule.go` | XBPQ 规则模型 + `ParseXBPQ` |
| `internal/provider/tvbox/rule/rule_test.go` | 规则 JSON 解析测试 |
| `internal/provider/tvbox/rule/client.go` | 带 UA/超时/大小上限的 GET/POST |
| `internal/provider/tvbox/rule/client_test.go` | HTTP 客户端测试 |
| `internal/provider/tvbox/rule/engine.go` | `Engine` 接口、`Vod`、`Supported`、`NewEngine` |
| `internal/provider/tvbox/rule/xbpq.go` | XBPQ 引擎实现 |
| `internal/provider/tvbox/rule/xbpq_test.go` | 引擎 fixture 测试 |
| `internal/provider/tvbox/jar.go` | `JarProvider` 适配器 |
| `internal/provider/tvbox/jar_test.go` | Provider 端到端测试 |
| `internal/shell/service.go` | `collectVodSites` 增加 `case "jar"` |
| `internal/shell/service_test.go` | 收集逻辑测试更新 |

---

### Task 1: 解析 `spider` 描述符

**Files:**
- Create: `internal/config/spider.go`
- Test: `internal/config/spider_test.go`

**Interfaces:**
- Produces: `config.SpiderDescriptor{Raw, Path, MD5 string}`；`config.ParseSpider(raw string) SpiderDescriptor`；`SpiderDescriptor.String() string`。

- [ ] **Step 1: 写失败测试**

```go
package config

import "testing"

func TestParseSpider(t *testing.T) {
	cases := []struct {
		raw      string
		wantPath string
		wantMD5  string
	}{
		{"./jar/XYQ.jar;md5;705BA6E23C4384C37A11DD904727520B", "./jar/XYQ.jar", "705ba6e23c4384c37a11dd904727520b"},
		{"https://x/y.jar;md5;abc", "https://x/y.jar", "abc"},
		{"https://x/y.jar", "https://x/y.jar", ""},
		{"  ./a.jar  ", "./a.jar", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		got := ParseSpider(c.raw)
		if got.Path != c.wantPath || got.MD5 != c.wantMD5 {
			t.Errorf("ParseSpider(%q) = (%q,%q)，期望 (%q,%q)",
				c.raw, got.Path, got.MD5, c.wantPath, c.wantMD5)
		}
	}
}

func TestSpiderDescriptorStringRoundTrip(t *testing.T) {
	d := SpiderDescriptor{Path: "https://x/y.jar", MD5: "abc"}
	if got := d.String(); got != "https://x/y.jar;md5;abc" {
		t.Fatalf("String() = %q", got)
	}
	if got := (SpiderDescriptor{Path: "https://x/y.jar"}).String(); got != "https://x/y.jar" {
		t.Fatalf("无 md5 时 String() = %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run 'TestParseSpider|TestSpiderDescriptorStringRoundTrip' -count=1`
Expected: FAIL（`undefined: ParseSpider` / `undefined: SpiderDescriptor`）

- [ ] **Step 3: 写最小实现**

```go
package config

import "strings"

// SpiderDescriptor 是配置里 spider 字段的解析结果。
// TVBox 的写法是 "<地址>;md5;<小写 md5>"，地址可以是相对路径。
type SpiderDescriptor struct {
	Raw  string
	Path string // 去掉 ;md5; 后缀的地址
	MD5  string // 声明的小写 MD5；未声明时为空
}

// ParseSpider 解析 "<path>;md5;<hash>"，未带 MD5 时 Path 为去空白后的原值。
func ParseSpider(raw string) SpiderDescriptor {
	d := SpiderDescriptor{Raw: raw}
	path, md5, ok := strings.Cut(raw, ";md5;")
	if !ok {
		d.Path = strings.TrimSpace(raw)
		return d
	}
	d.Path = strings.TrimSpace(path)
	d.MD5 = strings.ToLower(strings.TrimSpace(md5))
	return d
}

// String 还原成 TVBox 的 spider 写法。
func (d SpiderDescriptor) String() string {
	if d.MD5 == "" {
		return d.Path
	}
	return d.Path + ";md5;" + d.MD5
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config -run 'TestParseSpider|TestSpiderDescriptorStringRoundTrip' -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config/spider.go internal/config/spider_test.go
git commit -m "feat(config): 解析 spider 描述符与 md5 后缀"
```

---

### Task 2: 解析 `spider` 与 `ext` 的相对路径

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/urlresolve.go`
- Test: `internal/config/urlresolve_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ParseSpider`。
- Produces: `Site.ExtString() string`；`ResolveRelativeURLs` 现在也改写 `cfg.Spider` 与字符串型 `Site.Ext`。

- [ ] **Step 1: 写失败测试**

在 `internal/config/urlresolve_test.go` 的 `TestResolveRelativeURLs` 里给 `cfg` 增加：

```go
Spider: "./jar/XYQ.jar;md5;705ba6e23c4384c37a11dd904727520b",
```

给 `Sites` 增加：

```go
{Key: "xbpq", Ext: "./XBPQ/6080.json"},
{Key: "json-ext", Ext: `{"a":1}`},
{Key: "object-ext", Ext: map[string]any{"a": 1}},
```

在断言后增加：

```go
if want := "https://cfg.example.com/feed/jar/XYQ.jar;md5;705ba6e23c4384c37a11dd904727520b"; cfg.Spider != want {
	t.Errorf("Spider = %q，期望 %q", cfg.Spider, want)
}
for _, site := range cfg.Sites {
	switch site.Key {
	case "xbpq":
		if site.ExtString() != "https://cfg.example.com/feed/XBPQ/6080.json" {
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
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestResolveRelativeURLs -count=1`
Expected: FAIL（`Spider` 未改写、`ExtString` 未定义）

- [ ] **Step 3: 写实现**

`internal/config/model.go` 增加 `strings` import 和方法：

```go
// ExtString 返回 ext 的字符串形式；对象/数组等非字符串返回空串。
func (s Site) ExtString() string {
	v, _ := s.Ext.(string)
	return strings.TrimSpace(v)
}
```

`internal/config/urlresolve.go` 在 `ResolveRelativeURLs` 开头、`for i := range cfg.Sites` 之前加入：

```go
if cfg.Spider != "" {
	cfg.Spider = resolveSpider(base, cfg.Spider)
}
```

把站点循环改为：

```go
for i := range cfg.Sites {
	cfg.Sites[i].API = resolveRelativeURL(base, cfg.Sites[i].API)
	cfg.Sites[i].Ext = resolveExt(base, cfg.Sites[i].Ext)
}
```

文件末尾新增：

```go
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
	return resolveRelativeURL(base, s)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config/model.go internal/config/urlresolve.go internal/config/urlresolve_test.go
git commit -m "feat(config): 解析 spider 与 ext 的相对地址"
```

---

### Task 3: `csp_` 分类与 Provider 接线

**Files:**
- Modify: `internal/config/classify.go`
- Test: `internal/config/classify_test.go`
- Modify: `internal/shell/service.go`
- Test: `internal/shell/service_test.go`
- Create: `internal/provider/tvbox/jar.go`（本任务先放最小骨架，Task 10 补全）
- Create: `internal/provider/tvbox/jar_test.go`（本任务先测“未知类拒绝”）

**Interfaces:**
- Consumes: Task 1/2 的 `Site.ExtString`。
- Produces: `tvbox.NewJar(site config.Site) (*tvbox.JarProvider, error)`；`JarProvider` 实现 `provider.Provider`。

- [ ] **Step 1: 写失败测试**

`internal/config/classify_test.go` 修改两行并新增：

```go
{Site{Type: SiteTypeSpider, API: "csp_XBPQ"}, "jar", SupportYes},
{Site{Type: SiteTypeSpider, API: "csp_xbpq?v=1"}, "jar", SupportYes},
{Site{Type: SiteTypeSpider, API: "csp_Ftyg"}, "jar", SupportMaybe},
```

`internal/shell/service_test.go` 的 `TestCollectVodSitesSpider` 增加站点：

```go
{Key: "xbpq", Name: "XBPQ站", Type: config.SiteTypeSpider, API: "csp_XBPQ", Ext: "https://example.com/xbpq.json"},
```

把 `len(vods) != 3` 改成 `!= 4`，消息改为 `cms + spider http + js + xbpq`，并增加：

```go
if _, ok := vods["xbpq"].(*tvbox.JarProvider); !ok {
	t.Fatalf("xbpq 站点应使用 tvbox.JarProvider，实际类型 %T", vods["xbpq"])
}
```

新建 `internal/provider/tvbox/jar_test.go`：

```go
package tvbox

import (
	"testing"

	"github.com/unbox/unbox/internal/config"
)

func TestNewJarRejectsUnknownClass(t *testing.T) {
	if _, err := NewJar(config.Site{Key: "x", API: "csp_Unknown", Ext: "https://x/r.json"}); err == nil {
		t.Fatal("未知 csp 类应返回错误")
	}
}

func TestNewJarAcceptsXBPQ(t *testing.T) {
	p, err := NewJar(config.Site{Key: "x", API: "csp_XBPQ", Ext: "https://x/r.json"})
	if err != nil || p == nil {
		t.Fatalf("NewJar = %v, %v", p, err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestClassify -count=1 && go test ./internal/shell -run TestCollectVodSitesSpider -count=1`
Expected: FAIL（分类仍为 `SupportNo`；`NewJar` 未定义）

- [ ] **Step 3: 写实现**

`internal/config/classify.go` 增加：

```go
// knownJarEngines 是已实现 Go 规则引擎的 csp 类（小写比较）。
var knownJarEngines = map[string]bool{
	"csp_xbpq": true,
}
```

把 `csp_` 分支改为：

```go
case strings.HasPrefix(api, "csp_"):
	if knownJarEngines[api] {
		return "jar", SupportYes
	}
	return "jar", SupportMaybe
```

`internal/provider/tvbox/jar.go` 先写骨架：

```go
package tvbox

import (
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/config"
)

// JarProvider 是规则型 JAR 站点的 Provider。
// M6.1 只支持 csp_XBPQ；规则在首次动作时懒加载。
type JarProvider struct {
	site config.Site
}

// NewJar 校验类名；真正的规则加载发生在首次 Home/Browse/Search/Detail 时。
func NewJar(site config.Site) (*JarProvider, error) {
	class := strings.ToLower(strings.TrimSpace(site.API))
	if class != "csp_xbpq" {
		return nil, fmt.Errorf("暂不支持的 JAR 爬虫类: %s", site.API)
	}
	if site.ExtString() == "" {
		return nil, fmt.Errorf("JAR 站点 %s 缺少 ext 规则地址", site.Key)
	}
	return &JarProvider{site: site}, nil
}
```

`internal/shell/service.go` 的 `collectVodSites` switch 增加：

```go
case "jar":
	jar, err := tvbox.NewJar(site)
	if err != nil {
		continue
	}
	vods[site.Key] = jar
	names[site.Key] = site.Name
	lines[site.Key] = line
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config ./internal/provider/tvbox ./internal/shell -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config/classify.go internal/config/classify_test.go internal/provider/tvbox/jar.go internal/provider/tvbox/jar_test.go internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(jar): 接入 csp_XBPQ 站点收集与分类"
```

---

### Task 4: 标记提取与数组切分

**Files:**
- Create: `internal/provider/tvbox/rule/extract.go`
- Test: `internal/provider/tvbox/rule/extract_test.go`

**Interfaces:**
- Produces: `rule.Cut(s, spec string) string`；`rule.CutList(s, spec string) []string`；`rule.Clean(s string) string`。

- [ ] **Step 1: 写失败测试**

```go
package rule

import "testing"

func TestCut(t *testing.T) {
	const page = `<a title="甲" href="/v/1"><img data-original="https://img/1.jpg"></a>`
	cases := []struct{ spec, want string }{
		{`title="&&"`, "甲"},
		{`href="&&"`, "/v/1"},
		{`data-original="&&"`, "https://img/1.jpg"},
		{"", ""},
		{`不存在的&&"`, ""},
	}
	for _, c := range cases {
		if got := Cut(page, c.spec); got != c.want {
			t.Errorf("Cut(%q) = %q，期望 %q", c.spec, got, c.want)
		}
	}
}

func TestCutList(t *testing.T) {
	page := `<li class="module-item"><a title="甲" href="/1"></a></li>` +
		`<li class="module-item"><a title="乙" href="/2"></a></li>`
	got := CutList(page, `class="module-item"&&</li>`)
	if len(got) != 2 || !contains(got[0], "甲") || !contains(got[1], "乙") {
		t.Fatalf("CutList = %#v", got)
	}
}

func TestClean(t *testing.T) {
	if got := Clean(`<span>甲 &amp; 乙</span>`); got != "甲 & 乙" {
		t.Fatalf("Clean = %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox/rule -run 'TestCut|TestClean' -count=1`
Expected: FAIL（包/函数不存在）

- [ ] **Step 3: 写实现**

```go
package rule

import (
	"html"
	"regexp"
	"strings"
)

var tagRe = regexp.MustCompile(`<[^>]*>`)

// Cut 提取 "A&&B" 规则中 A 与 B 之间的文本。
// spec 为空返回空串；没有 "&&" 时取标记之后到结尾。
func Cut(s, spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	prefix, suffix, ok := strings.Cut(spec, "&&")
	if !ok {
		if i := strings.Index(s, prefix); i >= 0 {
			return strings.TrimSpace(s[i+len(prefix):])
		}
		return ""
	}
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	if suffix == "" {
		return strings.TrimSpace(rest)
	}
	j := strings.Index(rest, suffix)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

// CutList 按 "A&&B" 数组规则切分：以 A 分段，每段截到 B。
func CutList(s, spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	prefix, suffix, ok := strings.Cut(spec, "&&")
	if !ok {
		prefix, suffix = spec, ""
	}
	parts := strings.Split(s, prefix)
	if len(parts) <= 1 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		if suffix != "" {
			if j := strings.Index(p, suffix); j >= 0 {
				p = p[:j]
			}
		}
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Clean 去掉 HTML 标签并反转义实体。
func Clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox/rule -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/rule/extract.go internal/provider/tvbox/rule/extract_test.go
git commit -m "feat(jar): 实现 XBPQ 标记提取与数组切分"
```

---

### Task 5: XBPQ 规则模型与 HTTP 客户端

**Files:**
- Create: `internal/provider/tvbox/rule/rule.go`
- Test: `internal/provider/tvbox/rule/rule_test.go`
- Create: `internal/provider/tvbox/rule/client.go`
- Test: `internal/provider/tvbox/rule/client_test.go`

**Interfaces:**
- Produces: `rule.XBPQ`、`rule.ParseXBPQ(data []byte) (*XBPQ, error)`；
  `rule.NewClient(headers map[string]string) *Client`；`Client.Get(ctx, url) (string, error)`；
  `Client.Post(ctx, url, body, contentType string) (string, error)`。

- [ ] **Step 1: 写失败测试**

`rule_test.go`：

```go
package rule

import "testing"

func TestParseXBPQ(t *testing.T) {
	raw := []byte(`{
		"主页url":"https://a/",
		"分类":"电影$1#剧集$2",
		"分类url":"https://a/vod/{cateId}/page/{catePg}.html",
		"数组":"class=\"item\"&&</li>",
		"标题":"title=\"&&\"",
		"链接":"href=\"&&\"",
		"筛选":{"1":[{"key":"cateId","name":"类型","value":[{"n":"全部","v":"1"}]}]}
	}`)
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.HomeURL != "https://a/" || r.Classes != "电影$1#剧集$2" {
		t.Fatalf("解析结果 = %+v", r)
	}
	if len(r.Filters["1"]) != 1 || r.Filters["1"][0].Values[0].Value != "1" {
		t.Fatalf("筛选解析 = %+v", r.Filters)
	}
}

func TestParseXBPQRejectsEmpty(t *testing.T) {
	if _, err := ParseXBPQ([]byte(`{}`)); err == nil {
		t.Fatal("空规则应返回错误")
	}
}
```

`client_test.go`：

```go
package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetSetsUAAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "okhttp/3.12.11" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("X-Test") != "1" {
			t.Errorf("自定义头丢失")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewClient(map[string]string{"X-Test": "1"})
	got, err := c.Get(context.Background(), srv.URL)
	if err != nil || got != "ok" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}

func TestClientRejectsOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxBodySize+1))
	}))
	defer srv.Close()
	c := NewClient(nil)
	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("超限响应应报错")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox/rule -run 'TestParseXBPQ|TestClient' -count=1`
Expected: FAIL（类型/函数未定义）

- [ ] **Step 3: 写实现**

`rule.go`：

```go
package rule

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FilterValue 是筛选选项；JSON 用 n/v。
type FilterValue struct {
	Name  string `json:"n"`
	Value string `json:"v"`
}

// FilterGroup 是一组筛选条件。
type FilterGroup struct {
	Key    string        `json:"key"`
	Name   string        `json:"name"`
	Values []FilterValue `json:"value"`
}

// XBPQ 是 XBPQ 规则文件的字段子集。字段名保持规则文件的中文键。
type XBPQ struct {
	HomeURL      string `json:"主页url"`
	Classes      string `json:"分类"`
	CategoryURL  string `json:"分类url"`
	Array        string `json:"数组"`
	Title        string `json:"标题"`
	Link         string `json:"链接"`
	Pic          string `json:"图片"`
	Subtitle     string `json:"副标题"`
	LinkPrefix   string `json:"链接前缀"`
	LinkSuffix   string `json:"链接后缀"`
	SearchURL    string `json:"搜索url"`
	SearchMode   string `json:"搜索模式"`
	SearchArray  string `json:"搜索数组"`
	SearchTitle  string `json:"搜索标题"`
	SearchLink   string `json:"搜索链接"`
	SearchPic    string `json:"搜索图片"`
	SearchSubtitle string `json:"搜索副标题"`
	Type         string `json:"影片类型"`
	Year         string `json:"影片年代"`
	Area         string `json:"影片地区"`
	Remarks      string `json:"影片状态"`
	Director     string `json:"导演"`
	Actor        string `json:"主演"`
	Content      string `json:"简介"`
	PlayFromArray string `json:"线路数组"`
	PlayFromTitle string `json:"线路标题"`
	PlayArray    string `json:"播放数组"`
	PlayList     string `json:"播放列表"`
	PlayTitle    string `json:"播放标题"`
	PlayLink     string `json:"播放链接"`
	DirectPlay   string `json:"直接播放"`
	SniffWords   string `json:"嗅探词"`
	Headers      map[string]string   `json:"请求头"`
	Filters      map[string][]FilterGroup `json:"筛选"`
}

// ParseXBPQ 解析规则 JSON；至少要有主页url或分类url之一。
func ParseXBPQ(data []byte) (*XBPQ, error) {
	var r XBPQ
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("解析 XBPQ 规则失败: %w", err)
	}
	if strings.TrimSpace(r.HomeURL) == "" && strings.TrimSpace(r.CategoryURL) == "" {
		return nil, fmt.Errorf("XBPQ 规则缺少 主页url/分类url")
	}
	return &r, nil
}
```

`client.go`：

```go
package rule

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxBodySize = 4 << 20
	tvboxUA     = "okhttp/3.12.11"
)

// Client 是规则引擎的 HTTP 客户端：固定 UA、自定义头、内存 Cookie。
type Client struct {
	hc      *http.Client
	headers map[string]string
	mu      sync.Mutex
	cookies map[string]string
}

// NewClient 构造客户端；headers 可为 nil。
func NewClient(headers map[string]string) *Client {
	return &Client{
		hc:      &http.Client{Timeout: 15 * time.Second},
		headers: headers,
		cookies: map[string]string{},
	}
}

// Get 发起 GET 并返回正文。
func (c *Client) Get(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	return c.do(req)
}

// Post 发起 POST；contentType 为空时用 application/x-www-form-urlencoded。
func (c *Client) Post(ctx context.Context, rawURL, body, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	if contentType == "" {
		contentType = "application/x-www-form-urlencoded"
	}
	req.Header.Set("Content-Type", contentType)
	return c.do(req)
}

func (c *Client) do(req *http.Request) (string, error) {
	req.Header.Set("User-Agent", tvboxUA)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	c.mu.Lock()
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	c.mu.Unlock()

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", req.URL, err)
	}
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		c.mu.Lock()
		c.cookies[ck.Name] = ck.Value
		c.mu.Unlock()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("请求 %s 返回 HTTP %d", req.URL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return "", fmt.Errorf("读取 %s 响应失败: %w", req.URL, err)
	}
	if len(body) > maxBodySize {
		return "", fmt.Errorf("响应超过 %d 字节限制: %s", maxBodySize, req.URL)
	}
	return string(body), nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox/rule -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/rule/rule.go internal/provider/tvbox/rule/rule_test.go internal/provider/tvbox/rule/client.go internal/provider/tvbox/rule/client_test.go
git commit -m "feat(jar): 增加 XBPQ 规则模型与 HTTP 客户端"
```

---

### Task 6: 引擎接口与 `Supported`

**Files:**
- Create: `internal/provider/tvbox/rule/engine.go`

**Interfaces:**
- Produces: `rule.Vod`；`rule.Engine` 接口；`rule.Supported(class string) bool`；
  `rule.NewEngine(ctx context.Context, class, extURL string) (Engine, error)`
  （本任务只定义与拒绝未知类，XBPQ 注册在 Task 7 完成）。

- [ ] **Step 1: 写失败测试**

在 `internal/provider/tvbox/rule/rule_test.go` 增加：

```go
import "context"

func TestSupported(t *testing.T) {
	if !Supported("csp_XBPQ") || Supported("csp_Unknown") {
		t.Fatalf("Supported 结果不符")
	}
}

func TestNewEngineRejectsUnknown(t *testing.T) {
	if _, err := NewEngine(context.Background(), "csp_Unknown", "https://x/r.json"); err == nil {
		t.Fatal("未知类应返回错误")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox/rule -run 'TestSupported|TestNewEngineRejectsUnknown' -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

```go
package rule

import (
	"context"
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Vod 是引擎返回的详情原始数据；由 tvbox.JarProvider 转成 provider.Media。
type Vod struct {
	ID       string
	Name     string
	Pic      string
	TypeName string
	Remarks  string
	Year     string
	Area     string
	Content  string
	PlayFrom string // $$$ 分隔的线路名
	PlayURL  string // $$$ 线路、# 集、$ 集名/地址
}

// Engine 是一个规则型 JAR 爬虫的 Go 实现。
type Engine interface {
	Home(ctx context.Context) ([]provider.Section, error)
	Browse(ctx context.Context, cat string, page int) (provider.Page, error)
	Search(ctx context.Context, q string) ([]provider.Item, error)
	Detail(ctx context.Context, id string) (Vod, error)
	Resolve(ctx context.Context, epURL string) (player.Stream, error)
}

// Supported 判断类名是否已有 Go 规则引擎。
func Supported(class string) bool {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "csp_xbpq":
		return true
	default:
		return false
	}
}

// NewEngine 读取 ext 规则并构造引擎。
// Task 6 只定义入口；具体类在 Task 7 注册。
func NewEngine(context.Context, string, string) (Engine, error) {
	return nil, fmt.Errorf("JAR 引擎尚未注册")
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox/rule -run 'TestSupported|TestNewEngineRejectsUnknown' -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/rule/engine.go internal/provider/tvbox/rule/rule_test.go
git commit -m "feat(jar): 增加规则引擎接口与 Supported"
```

---

### Task 7: XBPQ 引擎（首页/分类/搜索/详情/播放）

**Files:**
- Create: `internal/provider/tvbox/rule/xbpq.go`
- Modify: `internal/provider/tvbox/rule/engine.go`（注册 `csp_xbpq`）
- Test: `internal/provider/tvbox/rule/xbpq_test.go`

**Interfaces:**
- Consumes: Task 4 的 `Cut/CutList/Clean`；Task 5 的 `XBPQ`/`Client`；Task 6 的 `Vod`。
- Produces: `rule.NewXBPQEngine(r *XBPQ, c *Client) *XBPQEngine`，实现 `rule.Engine`；
  `NewEngine` 现在能构造 `csp_XBPQ` 引擎。

- [ ] **Step 1: 写失败测试**

```go
package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func xbpqFixture(t *testing.T) (*XBPQEngine, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/category/1/1":
			_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"><img data-original="https://img/1.jpg"></a></li>
<li class="item"><a title="乙" href="/v/2"><img data-original="https://img/2.jpg"></a></li>`))
		case "/search":
			_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"></a></li>`))
		case "/v/1":
			_, _ = w.Write([]byte(`<title>甲</title>
<div>类型：电影</div><div>年份：2026</div><div>地区：大陆</div><div>状态：HD</div>
<div class="src"><b>线路一</b></div><div class="src"><b>线路二</b></div>
<div class="play"> <a href="/p/1"><span>第1集</span></a><a href="/p/2"><span>第2集</span></a></div>
<div class="play"> <a href="/p/3"><span>第1集</span></a></div>`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	rule := &XBPQ{
		HomeURL:     srv.URL + "/",
		Classes:     "电影$1#剧集$2",
		CategoryURL: srv.URL + "/category/{cateId}/{catePg}",
		Array:       `class="item"&&</li>`,
		Title:       `title="&&"`,
		Link:        `href="&&"`,
		Pic:         `data-original="&&"`,
		Type:        `类型：&&<`,
		Year:        `年份：&&<`,
		Area:        `地区：&&<`,
		Remarks:     `状态：&&<`,
		PlayFromArray: `class="src">&&</div>`,
		PlayFromTitle: `>&&<`,
		PlayArray:     `class="play">&&</div>`,
		PlayList:      `<a&&</a>`,
		PlayTitle:     `<span>&&</span>`,
		PlayLink:      `href="&&"`,
	}
	return NewXBPQEngine(rule, NewClient(nil)), srv
}

func TestNewEngineXBPQ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"主页url":"https://a/","分类":"电影$1","分类url":"https://a/{cateId}/{catePg}"}`))
	}))
	defer srv.Close()
	got, err := NewEngine(context.Background(), "csp_XBPQ", srv.URL)
	if err != nil || got == nil {
		t.Fatalf("NewEngine = %v, %v", got, err)
	}
}

func TestXBPQHome(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	secs, err := e.Home(context.Background())
	if err != nil || len(secs) != 2 || secs[0].ID != "1" || secs[0].Title != "电影" {
		t.Fatalf("Home = %#v, %v", secs, err)
	}
}

func TestXBPQBrowse(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	page, err := e.Browse(context.Background(), "1", 1)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("Browse = %#v, %v", page, err)
	}
	if page.Items[0].Title != "甲" || page.Items[0].ID != srv.URL+"/v/1" {
		t.Fatalf("首项 = %#v", page.Items[0])
	}
}

func TestXBPQDetail(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "甲" || v.TypeName != "电影" || v.PlayFrom != "线路一$$$线路二" {
		t.Fatalf("Detail = %#v", v)
	}
	wantPlay := "第1集$" + srv.URL + "/p/1#第2集$" + srv.URL + "/p/2$$$第1集$" + srv.URL + "/p/3"
	if v.PlayURL != wantPlay {
		t.Fatalf("PlayURL = %q，期望 %q", v.PlayURL, wantPlay)
	}
}

func TestXBPQResolve(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	stream, err := e.Resolve(context.Background(), "https://cdn/x.m3u8")
	if err != nil || stream.URL != "https://cdn/x.m3u8" {
		t.Fatalf("Resolve = %#v, %v", stream, err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox/rule -run TestXBPQ -count=1`
Expected: FAIL（`NewXBPQEngine` 未定义）

- [ ] **Step 3: 写实现**

```go
package rule

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// XBPQEngine 是 csp_XBPQ 的规则解释器。
type XBPQEngine struct {
	rule *XBPQ
	c    *Client
}

// NewXBPQEngine 构造引擎。
func NewXBPQEngine(r *XBPQ, c *Client) *XBPQEngine {
	return &XBPQEngine{rule: r, c: c}
}

func (e *XBPQEngine) Home(context.Context) ([]provider.Section, error) {
	var out []provider.Section
	for _, part := range strings.Split(e.rule.Classes, "#") {
		name, id, ok := strings.Cut(strings.TrimSpace(part), "$")
		if !ok {
			id = name
		}
		name, id = strings.TrimSpace(name), strings.TrimSpace(id)
		if name == "" || id == "" {
			continue
		}
		out = append(out, provider.Section{ID: id, Title: name})
	}
	return out, nil
}

func (e *XBPQEngine) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if page <= 0 {
		page = 1
	}
	raw, err := e.c.Get(ctx, expandURL(e.rule.CategoryURL, cat, page))
	if err != nil {
		return provider.Page{}, err
	}
	items := e.parseItems(raw, e.rule.Array, e.rule.Title, e.rule.Link, e.rule.Pic, e.rule.Subtitle)
	return provider.Page{Items: items, Page: page, HasMore: len(items) > 0}, nil
}

func (e *XBPQEngine) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if strings.TrimSpace(e.rule.SearchURL) == "" {
		return nil, nil
	}
	u := strings.NewReplacer("{wd}", url.QueryEscape(q), "{pg}", "1").Replace(e.rule.SearchURL)
	raw, err := e.c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return e.parseItems(raw,
		firstNonEmpty(e.rule.SearchArray, e.rule.Array),
		firstNonEmpty(e.rule.SearchTitle, e.rule.Title),
		firstNonEmpty(e.rule.SearchLink, e.rule.Link),
		firstNonEmpty(e.rule.SearchPic, e.rule.Pic),
		firstNonEmpty(e.rule.SearchSubtitle, e.rule.Subtitle)), nil
}

func (e *XBPQEngine) Detail(ctx context.Context, id string) (Vod, error) {
	raw, err := e.c.Get(ctx, id)
	if err != nil {
		return Vod{}, err
	}
	name := Clean(Cut(raw, e.rule.Title))
	if name == "" {
		name = titleTag(raw)
	}
	v := Vod{
		ID:       id,
		Name:     name,
		TypeName: Clean(Cut(raw, e.rule.Type)),
		Year:     Clean(Cut(raw, e.rule.Year)),
		Area:     Clean(Cut(raw, e.rule.Area)),
		Remarks:  Clean(Cut(raw, e.rule.Remarks)),
		Content:  Clean(Cut(raw, e.rule.Content)),
	}
	var froms, plays []string
	for _, seg := range CutList(raw, e.rule.PlayFromArray) {
		if name := Clean(Cut(seg, e.rule.PlayFromTitle)); name != "" {
			froms = append(froms, name)
		}
	}
	for _, seg := range CutList(raw, e.rule.PlayArray) {
		var eps []string
		for _, entry := range CutList(seg, e.rule.PlayList) {
			name := Clean(Cut(entry, e.rule.PlayTitle))
			href := Clean(Cut(entry, e.rule.PlayLink))
			if href == "" {
				continue
			}
			eps = append(eps, name+"$"+e.absURL(href))
		}
		if len(eps) > 0 {
			plays = append(plays, strings.Join(eps, "#"))
		}
	}
	v.PlayFrom = strings.Join(froms, "$$$")
	v.PlayURL = strings.Join(plays, "$$$")
	return v, nil
}

func (e *XBPQEngine) Resolve(_ context.Context, epURL string) (player.Stream, error) {
	return player.Stream{
		URL:     epURL,
		Kind:    player.KindForURL(epURL),
		Headers: map[string]string{"User-Agent": tvboxUA, "Referer": originOf(e.rule.HomeURL)},
	}, nil
}

func (e *XBPQEngine) parseItems(raw, array, title, link, pic, subtitle string) []provider.Item {
	var out []provider.Item
	for _, seg := range CutList(raw, array) {
		name := Clean(Cut(seg, title))
		href := Clean(Cut(seg, link))
		if name == "" || href == "" {
			continue
		}
		href = e.absURL(e.rule.LinkPrefix + href + e.rule.LinkSuffix)
		out = append(out, provider.Item{
			ID:    href,
			Title: name,
			Logo:  Clean(Cut(seg, pic)),
			Group: Clean(Cut(seg, subtitle)),
		})
	}
	return out
}

func (e *XBPQEngine) absURL(raw string) string {
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	base, err := url.Parse(e.rule.HomeURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func expandURL(tpl, cat string, page int) string {
	return strings.NewReplacer(
		"{cateId}", cat, "{catePg}", strconv.Itoa(page),
		"{area}", "", "{by}", "", "{class}", "", "{lang}", "",
		"{year}", "", "{letter}", "",
	).Replace(tpl)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// titleTag 是详情页标题的兜底：有的规则没有给 标题 字段。
func titleTag(s string) string {
	m := titleRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return Clean(m[1])
}

var _ Engine = (*XBPQEngine)(nil)
```

同时把 `engine.go` 的 `NewEngine` 改为：

```go
func NewEngine(ctx context.Context, class, extURL string) (Engine, error) {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "csp_xbpq":
		raw, err := NewClient(nil).Get(ctx, extURL)
		if err != nil {
			return nil, fmt.Errorf("获取 XBPQ 规则失败: %w", err)
		}
		r, err := ParseXBPQ([]byte(raw))
		if err != nil {
			return nil, err
		}
		return NewXBPQEngine(r, NewClient(r.Headers)), nil
	default:
		return nil, fmt.Errorf("暂不支持的 JAR 爬虫类: %s", class)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox/rule -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/rule/xbpq.go internal/provider/tvbox/rule/xbpq_test.go
git commit -m "feat(jar): 实现 XBPQ 首页/分类/搜索/详情/播放"
```

---

### Task 8: `JarProvider` 适配器

**Files:**
- Modify: `internal/provider/tvbox/jar.go`
- Test: `internal/provider/tvbox/jar_test.go`

**Interfaces:**
- Consumes: Task 6/7 的 `rule.Supported` / `rule.NewEngine` / `rule.Engine`；现有 `splitSources`/`parseEpisodes`。
- Produces: 完整实现 `provider.Provider` 的 `*JarProvider`。

- [ ] **Step 1: 写失败测试**

在 `internal/provider/tvbox/jar_test.go` 增加一个端到端测试：

```go
func TestJarProviderEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rule.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"主页url":"` + "http://" + r.Host + `/",
			"分类":"电影$1",
			"分类url":"http://` + r.Host + `/c/{cateId}/{catePg}",
			"数组":"class=\"item\"&&</li>",
			"标题":"title=\"&&\"",
			"链接":"href=\"&&\"",
			"播放数组":"class=\"play\">&&</div>",
			"播放列表":"<a&&</a>",
			"播放标题":"<span>&&</span>",
			"播放链接":"href=\"&&\""
		}`))
	})
	mux.HandleFunc("/c/1/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"></a></li>`))
	})
	mux.HandleFunc("/v/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title><div class="play"><a href="/p/1"><span>第1集</span></a></div>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewJar(config.Site{Key: "x", API: "csp_XBPQ", Ext: srv.URL + "/rule.json"})
	if err != nil {
		t.Fatal(err)
	}
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 1 || secs[0].ID != "1" {
		t.Fatalf("Home = %#v, %v", secs, err)
	}
	page, err := p.Browse(context.Background(), "1", 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("Browse = %#v, %v", page, err)
	}
	media, err := p.Detail(context.Background(), page.Items[0].ID)
	if err != nil || len(media.Episodes) != 1 {
		t.Fatalf("Detail = %#v, %v", media, err)
	}
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil || stream.URL != srv.URL+"/p/1" {
		t.Fatalf("Resolve = %#v, %v", stream, err)
	}
}
```

需要补 import：`context`、`net/http`、`net/http/httptest`、`config`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox -run TestJarProviderEndToEnd -count=1`
Expected: FAIL（`JarProvider` 未实现）

- [ ] **Step 3: 写实现**

把 `internal/provider/tvbox/jar.go` 扩展为：

```go
package tvbox

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/tvbox/rule"
)

// JarProvider 是规则型 JAR 站点的 Provider。
type JarProvider struct {
	site config.Site

	mu      sync.Mutex
	engine  rule.Engine
	loaded  bool
	loadErr error
	cache   map[string]provider.Episode
}

// NewJar 校验类名与 ext；规则在首次动作时懒加载，避免导入源阶段联网。
func NewJar(site config.Site) (*JarProvider, error) {
	if !rule.Supported(site.API) {
		return nil, fmt.Errorf("暂不支持的 JAR 爬虫类: %s", site.API)
	}
	if site.ExtString() == "" {
		return nil, fmt.Errorf("JAR 站点 %s 缺少 ext 规则地址", site.Key)
	}
	return &JarProvider{site: site, cache: map[string]provider.Episode{}}, nil
}

func (p *JarProvider) ID() string { return p.site.Key }

func (p *JarProvider) ensureLoaded(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.loadErr
	}
	p.engine, p.loadErr = rule.NewEngine(ctx, p.site.API, p.site.ExtString())
	p.loaded = true
	return p.loadErr
}

func (p *JarProvider) Home(ctx context.Context) ([]provider.Section, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	return p.engine.Home(ctx)
}

func (p *JarProvider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Page{}, err
	}
	return p.engine.Browse(ctx, cat, page)
}

func (p *JarProvider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	return p.engine.Search(ctx, q)
}

func (p *JarProvider) Detail(ctx context.Context, id string) (provider.Media, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Media{}, err
	}
	v, err := p.engine.Detail(ctx, id)
	if err != nil {
		return provider.Media{}, err
	}
	sources := splitSources(v.PlayFrom)
	eps := parseEpisodes(id, v.PlayURL, sources)
	p.mu.Lock()
	for _, ep := range eps {
		p.cache[ep.ID] = ep
	}
	p.mu.Unlock()
	return provider.Media{
		ID: id, Title: v.Name, Logo: v.Pic, Group: v.TypeName,
		Description: v.Content, Year: v.Year, Area: v.Area,
		Type: v.TypeName, Remarks: v.Remarks, Sources: sources, Episodes: eps,
	}, nil
}

func (p *JarProvider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return player.Stream{}, err
	}
	p.mu.Lock()
	ep, ok := p.cache[epID]
	p.mu.Unlock()
	if !ok {
		vodID, _, _, err := parseEpID(epID)
		if err != nil {
			return player.Stream{}, err
		}
		if _, err := p.Detail(ctx, vodID); err != nil {
			return player.Stream{}, err
		}
		p.mu.Lock()
		ep, ok = p.cache[epID]
		p.mu.Unlock()
		if !ok {
			return player.Stream{}, fmt.Errorf("剧集不存在: %s", epID)
		}
	}
	return p.engine.Resolve(ctx, ep.URL)
}

func (p *JarProvider) String() string {
	return "jar:" + strings.TrimSpace(p.site.API)
}
```

> 注意：`parseEpID` 在 `tvbox.go` 中是包内函数，可直接使用。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/jar.go internal/provider/tvbox/jar_test.go
git commit -m "feat(jar): 用 JarProvider 适配 XBPQ 规则引擎"
```

---

### Task 9: 全量回归与真实站点验收

**Files:**
- Modify: `docs/2026-10-17-jar-acceptance.md`（新建验收记录）

- [ ] **Step 1: 跑全量测试**

Run:
```bash
go test ./... -count=1
go vet ./...
gofmt -l .
CGO_ENABLED=1 go build ./...
```
Expected: 全部无输出/通过。

- [ ] **Step 2: 真实站点验收**

从真实 XBPQ 站点取 1–2 条规则（示例；M6.1 只验证 GET 搜索）：

```bash
curl -sSL 'https://raw.example.com/某规则仓库/tvbox/main/XBPQ/6080.json' -o /tmp/6080.json
```

用一个临时 Go 测试或 `go run` 小程序加载规则并调用：
`Home` → `Browse` → `Detail` → `Resolve`，记录：
- 分类数量
- 列表首项标题/ID
- 详情线路数、集数
- 播放地址前缀（m3u8/mp4）

把结果写入 `docs/2026-10-17-jar-acceptance.md`。

- [ ] **Step 3: 提交验收记录**

```bash
git add docs/2026-10-17-jar-acceptance.md
git commit -m "docs: 记录 XBPQ 真实站点验收结果"
```

---

### Task 10: 文档与状态同步

**Files:**
- Modify: `docs/HANDOFF.md`
- Modify: `README.md`
- Modify: `AGENTS.md`（只加一行指向 spec）

- [ ] **Step 1: 更新 HANDOFF 总体进度**

在 M6 条目中把「规划中」改为「M6.1 已完成」，并记录提交号与验收结果。

- [ ] **Step 2: 更新 README 已知限制**

把「正在按 M6 计划接入」改为「`csp_XBPQ` 已支持；`csp_XYQHiker` 计划中」。

- [ ] **Step 3: AGENTS.md 增加指针**

在「项目概述」末尾增加：

```markdown
JAR（`csp_`）兼容的调研与 M6 规划见
`docs/superpowers/specs/2026-10-17-unbox-jar-compat-design.md`；结论是
「不执行 dex，走 Go 规则引擎」。
```

- [ ] **Step 4: 提交**

```bash
git add docs/HANDOFF.md README.md AGENTS.md
git commit -m "docs: 同步 M6.1 JAR 规则引擎进度"
```

---

## Self-Review

**Spec coverage:**
- M6.0 基础设施 → Tasks 1–3。
- M6.1 XBPQ → Tasks 4–8。
- 验收 → Task 9。
- 文档同步 → Task 10。
- 非目标（不执行 JAR、不内嵌 JVM）→ Global Constraints + spec §6。

**Placeholder scan:** 无 TBD/TODO；每个代码步骤都有可执行代码与测试命令。
Task 6 只定义接口并用「尚未注册」拒绝构造；Task 7 注册 `csp_xbpq` 并补测试，
两个任务都可独立运行测试。

**Type consistency:**
- `SpiderDescriptor`、`Site.ExtString`、`rule.Engine`、`rule.Vod`、`XBPQEngine`、
  `JarProvider` 的签名在各任务间一致。
- `parseEpisodes`/`splitSources`/`parseEpID` 复用现有 `internal/provider/tvbox` 包内函数。
