# UnBox M7 drpy2 / ES Module 兼容实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 UnBox 正确加载 drpy2 站点规则脚本，并支持真实 ESM 爬虫（import/export、assets:// 资源、`__jsEvalReturn`）。

**Architecture:** M7.1 修正 `tvbox.Spider` 的脚本选源：drpy2 引擎库 + `ext=.js` 时加载 `ext`。M7.2 在 `internal/crawler` 增加纯 Go 的 ESM 模块加载器：字符串/注释掩码 → import/export 改写 → IIFE 隔离执行 → 导出提升；内置 MIT 的 cheerio/crypto-js 资源。M7.3 用自有 shim 替代 GPL 的 `cat.js`。

**Tech Stack:** Go 1.26.3、goja、标准库 `net/url`/`regexp`/`embed`、现有 `crawler`/`tvbox`/`config` 包。不新增第三方依赖。

**Spec:** `docs/superpowers/specs/2026-10-17-unbox-js-esm-drpy2-design.md`

## Global Constraints

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/crawler` 不得 import Wails。
- 公开错误信息 / 注释用中文。
- TDD：先写失败测试，再写实现。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；
  Linux 额外 `CGO_ENABLED=1 go build ./...`。
- 不内置 GPL 的 `cat.js`，不内置未声明 License 的 `模板.js`/`gbk.js`。
- 不引入 esbuild 或其他打包器；ESM 加载器只做本计划列出的语法子集。
- 不支持的语法（`export * from`、动态 `import()`、`import.meta`）必须返回
  明确错误，不得静默降级。

---

## File Structure

| 文件 | 职责 |
|---|---|
| `internal/config/model.go` | `Site.ExtString()` |
| `internal/config/urlresolve.go` | 解析字符串型 `Site.Ext` |
| `internal/config/urlresolve_test.go` | ext 相对路径测试 |
| `internal/provider/tvbox/spider.go` | `spiderScriptURL` 选源 |
| `internal/provider/tvbox/spider_test.go` | 选源测试 |
| `internal/crawler/crawler.go` | 拆分 `stripAsyncAwait`；接入 `loadWithContext` |
| `internal/crawler/esm.go` | 掩码、`needsModuleLoader`、`rewriteModule`、`moduleLoader` |
| `internal/crawler/esm_test.go` | ESM 单元/集成测试 |
| `internal/crawler/assets.go` | `//go:embed` + `assetSource` |
| `internal/crawler/assets/cheerio.min.js` | MIT 资源 |
| `internal/crawler/assets/crypto-js.js` | MIT 资源 |
| `internal/crawler/assets/LICENSE.*` | 许可证原文 |
| `docs/2026-10-17-m7-acceptance.md` | 真实站点验收记录 |

---

### Task 1: 配置层解析 `ext` 相对路径

> 与 M6 计划的 Task 2 是同一处改动。若 M6 Task 2 已落地，直接跳过本任务；
> 若先执行 M7，则按本任务实现，M6 执行时复用。

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/urlresolve.go`
- Test: `internal/config/urlresolve_test.go`

**Interfaces:**
- Produces: `Site.ExtString() string`；`ResolveRelativeURLs` 现在会改写字符串型 `Site.Ext`。

- [ ] **Step 1: 写失败测试**

在 `TestResolveRelativeURLs` 的 `Sites` 里增加：

```go
{Key: "drpy2-ext", Ext: "./dr/js/某影视站.js"},
{Key: "json-ext", Ext: `{"a":1}`},
{Key: "object-ext", Ext: map[string]any{"a": 1}},
```

断言后增加：

```go
for _, site := range cfg.Sites {
	switch site.Key {
	case "drpy2-ext":
		if site.ExtString() != "https://cfg.example.com/feed/dr/js/某影视站.js" {
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
Expected: FAIL（`ExtString` 未定义，ext 未解析）

- [ ] **Step 3: 写实现**

`internal/config/model.go` 增加 `strings` import 和方法：

```go
// ExtString 返回 ext 的字符串形式；对象/数组等非字符串返回空串。
func (s Site) ExtString() string {
	v, _ := s.Ext.(string)
	return strings.TrimSpace(v)
}
```

`internal/config/urlresolve.go` 的站点循环改为：

```go
for i := range cfg.Sites {
	cfg.Sites[i].API = resolveRelativeURL(base, cfg.Sites[i].API)
	cfg.Sites[i].Ext = resolveExt(base, cfg.Sites[i].Ext)
}
```

文件末尾新增：

```go
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
git commit -m "feat(config): 解析字符串型 ext 的相对地址"
```

---

### Task 2: drpy2 站点选源修复

**Files:**
- Modify: `internal/provider/tvbox/spider.go`
- Test: `internal/provider/tvbox/spider_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Site.ExtString`。
- Produces: `spiderScriptURL(site config.Site) string`；`isDrpyEngineLib(raw string) bool`。

- [ ] **Step 1: 写失败测试**

在 `spider_test.go` 增加：

```go
func TestSpiderScriptURL(t *testing.T) {
	cases := []struct{ name string; site config.Site; want string }{
		{
			name: "drpy2 用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js", Ext: "https://x/dr/js/某影视站.js"},
			want: "https://x/dr/js/某影视站.js",
		},
		{
			name: "带 query 的 drpy2 用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js?v=1", Ext: "https://x/dr/js/a.js"},
			want: "https://x/dr/js/a.js",
		},
		{
			name: "站点脚本直接用 api",
			site: config.Site{API: "https://x/js/apple.js", Ext: "https://x/js/other.js"},
			want: "https://x/js/apple.js",
		},
		{
			name: "ext 不是 js 时用 api",
			site: config.Site{API: "https://x/js/apple.js", Ext: "https://x/rule.json"},
			want: "https://x/js/apple.js",
		},
		{
			name: "无 ext 时用 api",
			site: config.Site{API: "https://x/js/apple.js"},
			want: "https://x/js/apple.js",
		},
	}
	for _, c := range cases {
		if got := spiderScriptURL(c.site); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox -run TestSpiderScriptURL -count=1`
Expected: FAIL（`spiderScriptURL` 未定义）

- [ ] **Step 3: 写实现**

在 `internal/provider/tvbox/spider.go` 增加：

```go
// spiderScriptURL 选择真正要加载的脚本：
// drpy2 站点的 api 指向引擎库，站点规则在 ext 里；其余情况用 api。
func spiderScriptURL(site config.Site) string {
	api := strings.TrimSpace(site.API)
	ext := site.ExtString()
	if ext == "" || !strings.HasSuffix(strings.ToLower(ext), ".js") {
		return api
	}
	if isDrpyEngineLib(api) {
		return ext
	}
	return api
}

func isDrpyEngineLib(raw string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	switch strings.ToLower(path.Base(raw)) {
	case "drpy2.min.js", "drpy.min.js", "drpy2.js", "drpy.js":
		return true
	default:
		return false
	}
}
```

需要 import `path`。

把 `ensureLoaded` 里的：

```go
p.loadErr = p.engine.LoadFromURL(ctx, p.site.API)
```

改为：

```go
p.loadErr = p.engine.LoadFromURL(ctx, spiderScriptURL(p.site))
```

同时把 `Resolve` 里的 Referer 从 `originOf(p.site.API)` 改为
`originOf(spiderScriptURL(p.site))`，避免 drpy2 站点用引擎库地址做 Referer。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/spider.go internal/provider/tvbox/spider_test.go
git commit -m "fix(spider): drpy2 站点加载 ext 规则脚本"
```

---

### Task 3: 拆分 `stripAsyncAwait`

**Files:**
- Modify: `internal/crawler/crawler.go`
- Test: `internal/crawler/crawler_test.go`

**Interfaces:**
- Produces: `stripAsyncAwait(src string) string`；`normalizeModuleSource` 行为不变。

- [ ] **Step 1: 写失败测试**

在 `crawler_test.go` 增加：

```go
func TestStripAsyncAwait(t *testing.T) {
	got := stripAsyncAwait("async function home(){\n return await req('x')\n}")
	if strings.Contains(got, "async") || strings.Contains(got, "await") {
		t.Fatalf("async/await 未剥离:\n%s", got)
	}
	if !strings.Contains(got, "function home()") {
		t.Fatalf("函数声明被误伤:\n%s", got)
	}

	arrow := stripAsyncAwait("const f = async (x) => await g(x); const h = async x => await g(x);")
	if strings.Contains(arrow, "async") || strings.Contains(arrow, "await") {
		t.Fatalf("async 箭头函数未剥离:\n%s", arrow)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/crawler -run TestStripAsyncAwait -count=1`
Expected: FAIL（`stripAsyncAwait` 未定义）

- [ ] **Step 3: 写实现**

先在 `crawler.go` 现有的 `var (...)` 块里增加：

```go
asyncArrowRe = regexp.MustCompile(`\basync\s+(\([^)]*\)\s*=>|[\p{L}_$][\p{L}\p{N}_$]*\s*=>)`)
```

然后把 `normalizeModuleSource` 的前两行抽成：

```go
// stripAsyncAwait 把 async/await 归约为同步写法。
// 已知局限与 normalizeModuleSource 相同：字符串字面量里的 "await " 会被一并改写。
func stripAsyncAwait(src string) string {
	src = asyncFnRe.ReplaceAllString(src, "$1")
	src = asyncArrowRe.ReplaceAllString(src, "$1")
	return awaitRe.ReplaceAllString(src, "")
}

func normalizeModuleSource(src string) string {
	src = stripAsyncAwait(src)
	if index := strings.LastIndex(src, "export default"); index >= 0 {
		src = src[:index] + "var __crawler_exports = " + src[index+len("export default"):]
	}
	return src
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/crawler -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/crawler.go internal/crawler/crawler_test.go
git commit -m "refactor(crawler): 抽取 stripAsyncAwait"
```

---

### Task 4: 字符串/注释掩码与 ESM 判定

**Files:**
- Create: `internal/crawler/esm.go`
- Test: `internal/crawler/esm_test.go`

**Interfaces:**
- Produces: `maskNonCode(src string) string`；`needsModuleLoader(src string) bool`。

- [ ] **Step 1: 写失败测试**

```go
package crawler

import "testing"

func TestMaskNonCode(t *testing.T) {
	src := "const a = 'import x from \"y\"';\n// import z\n/* export const q */\nconst b = `export default`;\nimport real from './r.js';"
	masked := maskNonCode(src)
	if len(masked) != len(src) {
		t.Fatalf("长度变化: %d != %d", len(masked), len(src))
	}
	if contains(masked, "import x") || contains(masked, "import z") || contains(masked, "export const q") || contains(masked, "export default") {
		t.Fatalf("字符串/注释未掩码:\n%s", masked)
	}
	if !contains(masked, "import real") {
		t.Fatalf("真实 import 被误掩码:\n%s", masked)
	}
}

func TestNeedsModuleLoader(t *testing.T) {
	cases := []struct{ src string; want bool }{
		{"var rule = {host:'x'};", false},
		{"export default { home(){} };", false},
		{"import x from './x.js';", true},
		{"import './x.js';", true},
		{"import{a}from'./m.js';", true},
		{"import*as n from'./m.js';", true},
		{"import('./m.js');", true},
		{"obj.import('./m.js');", false},
		{"export function __jsEvalReturn(){ return {} };", true},
		{"export const a = 1;", true},
		{"export { a };", true},
		{"export{a};", true},
		{"const s = 'import x';", false},
		{"const important = 1;", false},
	}
	for _, c := range cases {
		if got := needsModuleLoader(c.src); got != c.want {
			t.Errorf("needsModuleLoader(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/crawler -run 'TestMaskNonCode|TestNeedsModuleLoader' -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

> 注意（Task 10/12 后）：当前 `esm.go` 已把掩码升级为 `maskString` +
> `maskTemplate` + `maskInterpolation` + `scanRegex`，可识别正则字面量、
> 嵌套模板字面量与无空格 `export default{`。下面只是初版实现片段，保留作
> 历史记录；以仓库代码为准。

```go
package crawler

import (
	"regexp"
	"strings"
)

// maskNonCode 返回与 src 等长的副本：字符串、模板、行注释、块注释内容替换为空格。
// 保留换行符，便于错误定位。仅用于语法定位，不参与执行。
// 已知局限：不识别正则字面量（/.../），其中的 import/引号可能被误判；
// 当前爬虫脚本样本未触发，后续如遇到再升级为带 prev-token 的词法扫描。
func maskNonCode(src string) string {
	out := []byte(src)
	n := len(src)
	blank := func(i int) {
		if out[i] != '\n' && out[i] != '\r' {
			out[i] = ' '
		}
	}
	for i := 0; i < n; {
		switch src[i] {
		case '\'', '"', '`':
			quote := src[i]
			// 保留引号本身（import 正则需要匹配 "..."），只清空内容。
			i++
			for i < n {
				c := src[i]
				if c == '\\' {
					blank(i)
					if i+1 < n {
						blank(i + 1)
					}
					i += 2
					continue
				}
				if c == quote {
					i++
					break
				}
				blank(i)
				i++
			}
		case '/':
			if i+1 < n && src[i+1] == '/' {
				for i < n && src[i] != '\n' {
					blank(i)
					i++
				}
			} else if i+1 < n && src[i+1] == '*' {
				blank(i)
				blank(i + 1)
				i += 2
				for i < n {
					if src[i] == '*' && i+1 < n && src[i+1] == '/' {
						blank(i)
						blank(i + 1)
						i += 2
						break
					}
					blank(i)
					i++
				}
			} else {
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

var (
	// 注意：Go regexp 不支持 lookahead，用两组分支覆盖：
	//   import x / import{a} / import*as / import "x" / import.meta
	// 压缩器会输出 import{a}from... / import*as n from...（无空格），必须匹配。
	reImportKeyword = regexp.MustCompile(`\bimport\s+[\p{L}_$][\p{L}\p{N}_$]*|\bimport\s*[{\*"'\.]`)
	// 动态 import：前面不能是 . 或标识符字符，避免把 obj.import(...) 误判。
	reDynamicImport = regexp.MustCompile(`(?:^|[^.\p{L}\p{N}_$])import\s*\(`)
	//   export default / export function / export {x} / export{x}
	reExportKeyword = regexp.MustCompile(`\bexport\s+([\p{L}_$][\p{L}\p{N}_$]*|\{|\*)|\bexport\s*[\{\*]`)
)

// needsModuleLoader 判断是否需要 ESM 加载器：
// 有 import（含无空格 named/namespace/dynamic），或有 export default 之外的 export。
func needsModuleLoader(src string) bool {
	masked := maskNonCode(src)
	if reImportKeyword.MatchString(masked) || reDynamicImport.MatchString(masked) {
		return true
	}
	for _, m := range reExportKeyword.FindAllStringSubmatch(masked, -1) {
		if !strings.EqualFold(m[1], "default") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/crawler -run 'TestMaskNonCode|TestNeedsModuleLoader' -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/esm.go internal/crawler/esm_test.go
git commit -m "feat(crawler): 增加 ESM 语法掩码与判定"
```

---

### Task 5: ESM 语法改写

**Files:**
- Modify: `internal/crawler/esm.go`
- Test: `internal/crawler/esm_test.go`

**Interfaces:**
- Produces: `rewriteModule(vm *goja.Runtime, src, baseURL string) (string, error)`。

- [ ] **Step 1: 写失败测试**

```go
func TestRewriteModuleImports(t *testing.T) {
	vm := goja.New()
	src := `
import def from "./a.js";
import { x, y as z } from "./b.js";
import * as ns from "./c.js";
import "./d.js";
`
	got, err := rewriteModule(vm, src, "https://x/app/main.js")
	if err != nil { t.Fatal(err) }
	for _, want := range []string{
		`__module("https://x/app/a.js")`,
		`__module("https://x/app/b.js")`,
		`__module("https://x/app/c.js")`,
		`__module("https://x/app/d.js")`,
	} {
		if !contains(got, want) { t.Fatalf("缺少 %q:\n%s", want, got) }
	}
	if contains(got, "import ") { t.Fatalf("import 残留:\n%s", got) }
}

func TestRewriteModuleExports(t *testing.T) {
	vm := goja.New()
	src := `export function __jsEvalReturn(){ return { home(){} } }
export const a = 1;
const b = 2;
export { b as c };`
	got, err := rewriteModule(vm, src, "https://x/m.js")
	if err != nil { t.Fatal(err) }
	for _, want := range []string{
		"function __jsEvalReturn", "__exports.__jsEvalReturn = __jsEvalReturn;",
		"const a = 1;", "__exports.a = a;", "__exports.c = b;",
	} {
		if !contains(got, want) { t.Fatalf("缺少 %q:\n%s", want, got) }
	}
	if contains(got, "export ") { t.Fatalf("export 残留:\n%s", got) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/crawler -run 'TestRewriteModule' -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

把 `esm.go` 的 import 块改为（Task 7 会再补 `context`/`io`/`net/http`/`sync`）：

```go
import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

type moduleEdit struct {
	start, end int
	text       string
}

func addModuleEdit(edits *[]moduleEdit, start, end int, text string) bool {
	for _, e := range *edits {
		if start < e.end && e.start < end {
			return false
		}
	}
	*edits = append(*edits, moduleEdit{start: start, end: end, text: text})
	return true
}

var (
	reImpDefault = regexp.MustCompile(`import\s+([\p{L}_$][\p{L}\p{N}_$]*)\s*(?:,\s*\{([^}]*)\})?\s+from\s*(["'][^"']*["'])`)
	reImpNamed   = regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*(["'][^"']*["'])`)
	reImpNS      = regexp.MustCompile(`import\s*\*\s*as\s+([\p{L}_$][\p{L}\p{N}_$]*)\s+from\s*(["'][^"']*["'])`)
	reImpSide    = regexp.MustCompile(`import\s*(["'][^"']*["'])`)
	reExpDefault = regexp.MustCompile(`export\s+default\s`)
	reExpFunc    = regexp.MustCompile(`export\s+function\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpVar     = regexp.MustCompile(`export\s+(const|let|var)\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpList    = regexp.MustCompile(`export\s*\{([^}]*)\}`)
	reExpStar    = regexp.MustCompile(`export\s*\*\s*from`)
)

func resolveImportSpec(vm *goja.Runtime, spec, baseURL string) (string, error) {
	v, err := vm.RunString(spec)
	if err != nil {
		return "", fmt.Errorf("导入地址不是字符串字面量: %s", spec)
	}
	ref := v.String()
	if strings.HasPrefix(ref, "assets://") ||
		strings.HasPrefix(ref, "http://") ||
		strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	if baseURL == "" {
		return "", fmt.Errorf("相对导入 %q 缺少 base URL", ref)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(r).String(), nil
}

func rewriteModule(vm *goja.Runtime, src, baseURL string) (string, error) {
	masked := maskNonCode(src)
	if reExpStar.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 export * from", ErrESModuleUnsupported)
	}
	if strings.Contains(masked, "import.meta") || reDynamicImport.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 import.meta / 动态 import()", ErrESModuleUnsupported)
	}

	var edits []moduleEdit
	var exports []string

	// imports
	for _, idx := range reImpDefault.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpDefault.FindStringSubmatch(region)
		if m == nil { continue }
		abs, err := resolveImportSpec(vm, m[3], baseURL)
		if err != nil { return "", err }
		repl := fmt.Sprintf("const %s = (function(m){return m&&m.default!==undefined?m.default:m;})(__module(%s));", m[1], strconv.Quote(abs))
		if m[2] != "" {
			repl += fmt.Sprintf(" const {%s} = __module(%s);", strings.ReplaceAll(m[2], " as ", ": "), strconv.Quote(abs))
		}
		if !addModuleEdit(&edits, idx[0], idx[1], repl) { return "", fmt.Errorf("import 语句重叠: %s", region) }
	}
	for _, idx := range reImpNamed.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNamed.FindStringSubmatch(region)
		if m == nil { continue }
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil { return "", err }
		repl := fmt.Sprintf("const {%s} = __module(%s);", strings.ReplaceAll(m[1], " as ", ": "), strconv.Quote(abs))
		if !addModuleEdit(&edits, idx[0], idx[1], repl) { continue }
	}
	for _, idx := range reImpNS.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNS.FindStringSubmatch(region)
		if m == nil { continue }
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil { return "", err }
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("const %s = __module(%s);", m[1], strconv.Quote(abs))) { continue }
	}
	for _, idx := range reImpSide.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpSide.FindStringSubmatch(region)
		if m == nil { continue }
		abs, err := resolveImportSpec(vm, m[1], baseURL)
		if err != nil { return "", err }
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("__module(%s);", strconv.Quote(abs))) { continue }
	}

	// exports
	for _, idx := range reExpDefault.FindAllStringIndex(masked, -1) {
		if !addModuleEdit(&edits, idx[0], idx[1], "__exports.default = ") { return "", fmt.Errorf("重复的 export default") }
	}
	for _, idx := range reExpFunc.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpFunc.FindStringSubmatch(region)
		if m == nil { continue }
		if !addModuleEdit(&edits, idx[0], idx[1], "function "+m[1]) { continue }
		exports = append(exports, m[1])
	}
	for _, idx := range reExpVar.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpVar.FindStringSubmatch(region)
		if m == nil { continue }
		if !addModuleEdit(&edits, idx[0], idx[1], m[1]+" "+m[2]) { continue }
		exports = append(exports, m[2])
	}
	for _, idx := range reExpList.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpList.FindStringSubmatch(region)
		if m == nil { continue }
		if !addModuleEdit(&edits, idx[0], idx[1], "") { continue }
		for _, item := range strings.Split(m[1], ",") {
			item = strings.TrimSpace(item)
			if item == "" { continue }
			local, exported := item, item
			if parts := strings.Split(item, " as "); len(parts) == 2 {
				local, exported = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
			exports = append(exports, exported+" = "+local)
		}
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		src = src[:e.start] + e.text + src[e.end:]
	}
	for _, item := range exports {
		if parts := strings.SplitN(item, " = ", 2); len(parts) == 2 {
			src += "\n__exports." + parts[0] + " = " + parts[1] + ";"
		} else {
			src += "\n__exports." + item + " = " + item + ";"
		}
	}
	return stripAsyncAwait(src), nil
}
```

> 注意：`reExpStar`/`import.meta` 的检查在掩码后执行；字符串里的同类文本不会被误判。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/crawler -run 'TestRewriteModule' -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/esm.go internal/crawler/esm_test.go
git commit -m "feat(crawler): 实现 ESM import/export 改写"
```

---

### Task 6: 内置 MIT JS 资源

**Files:**
- Create: `internal/crawler/assets/cheerio.min.js`
- Create: `internal/crawler/assets/crypto-js.js`
- Create: `internal/crawler/assets/LICENSE.cheerio.txt`
- Create: `internal/crawler/assets/LICENSE.crypto-js.txt`
- Create: `internal/crawler/assets.go`
- Test: `internal/crawler/esm_test.go`

**Interfaces:**
- Produces: `assetSource(url string) (string, bool)`；`cheerioAsset`、`cryptoJSAsset`。

- [ ] **Step 1: 下载并校验资源**

```bash
mkdir -p internal/crawler/assets
curl -sSL https://raw.githubusercontent.com/FongMi/TV/fongmi/quickjs/src/main/assets/js/lib/cheerio.min.js \
  -o internal/crawler/assets/cheerio.min.js
curl -sSL https://raw.githubusercontent.com/FongMi/TV/fongmi/quickjs/src/main/assets/js/lib/crypto-js.js \
  -o internal/crawler/assets/crypto-js.js
curl -sSL https://raw.githubusercontent.com/cheeriojs/cheerio/main/LICENSE \
  -o internal/crawler/assets/LICENSE.cheerio.txt
curl -sSL https://raw.githubusercontent.com/brix/crypto-js/develop/LICENSE \
  -o internal/crawler/assets/LICENSE.crypto-js.txt
```

校验：`cheerio.min.js` 约 356 KB，`crypto-js.js` 约 198 KB；两个文件必须是 JS
（不是 HTML）。在文件头注释中记录来源 URL、版本日期。

- [ ] **Step 2: 写失败测试**

在 `esm_test.go` 增加：

```go
func TestAssetSource(t *testing.T) {
	for _, u := range []string{
		"assets://js/lib/cheerio.min.js",
		"https://x/lib/cheerio.min.js",
		"https://x/js/crypto-js.js",
	} {
		if src, ok := assetSource(u); !ok || src == "" {
			t.Errorf("assetSource(%q) 未命中", u)
		}
	}
	if _, ok := assetSource("assets://js/lib/cat.js"); ok {
		t.Fatal("cat.js 不应内置")
	}
}
```

- [ ] **Step 3: 写实现**

`internal/crawler/assets.go`：

```go
package crawler

import (
	_ "embed"
	"path"
	"strings"
)

//go:embed assets/cheerio.min.js
var cheerioAsset string

//go:embed assets/crypto-js.js
var cryptoJSAsset string

// assetSource 按 basename 映射内置资源。
// 已知 basename 优先于远端请求，避免源站把 ./lib/crypto-js.js 返回成 404 HTML。
func assetSource(rawURL string) (string, bool) {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	switch path.Base(rawURL) {
	case "cheerio.min.js":
		return cheerioAsset, true
	case "crypto-js.js":
		return cryptoJSAsset, true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/crawler -run TestAssetSource -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/assets.go internal/crawler/assets
git commit -m "feat(crawler): 内置 MIT 的 cheerio/crypto-js 资源"
```

---

### Task 7: ESM 模块加载器

**Files:**
- Modify: `internal/crawler/esm.go`
- Modify: `internal/crawler/crawler.go`
- Test: `internal/crawler/esm_test.go`

**Interfaces:**
- Produces: `Engine.loadESM(ctx, src, baseURL) error`；`Engine.loadWithContext(ctx, src, baseURL) error`；
  `moduleLoader.load(url string) (goja.Value, error)`。

- [ ] **Step 1: 写失败测试**

```go
func TestLoadESMWithRelativeModule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dep.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`export default "dep";`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import dep from "./dep.js";
export function __jsEvalReturn(){ return { home(){ return dep } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil { t.Fatal(err) }
	if v.String() != "dep" {
		t.Fatalf("home() = %q", v.String())
	}
}

func TestLoadESMIIFEIsolation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`const C = "a"; export default C;`))
	})
	mux.HandleFunc("/b.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`const C = "b"; export default C;`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import a from "./a.js";
import b from "./b.js";
export function __jsEvalReturn(){ return { home(){ return a+b } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil { t.Fatal(err) }
	if v.String() != "ab" {
		t.Fatalf("home() = %q", v.String())
	}
}

func TestLoadESMUmdModule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/umd.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`(function(root,factory){
  if (typeof exports === "object") { module.exports = factory(); }
  else { root.UmdValue = factory(); }
})(this, function(){ return { v: 42 }; });`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import u from "./umd.js";
export function __jsEvalReturn(){ return { home(){ return String(u.v) } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil { t.Fatal(err) }
	if v.String() != "42" {
		t.Fatalf("home() = %q", v.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/crawler -run 'TestLoadESM' -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

把 `internal/crawler/esm.go` 的 import 块补全为：

```go
import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)
```

然后追加：

```go
type moduleLoader struct {
	e     *Engine
	ctx   context.Context
	mu    sync.Mutex
	cache map[string]goja.Value
}

func (l *moduleLoader) load(rawURL string) (goja.Value, error) {
	l.mu.Lock()
	if v, ok := l.cache[rawURL]; ok {
		l.mu.Unlock()
		return v, nil
	}
	l.mu.Unlock()

	var src string
	if s, ok := assetSource(rawURL); ok {
		src = s
	} else if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		b, err := l.e.fetchModule(l.ctx, rawURL)
		if err != nil {
			return nil, err
		}
		src = b
	} else {
		return nil, fmt.Errorf("模块地址协议不支持: %s", rawURL)
	}
	return l.loadSource(src, rawURL)
}

func (l *moduleLoader) loadSource(src, rawURL string) (goja.Value, error) {
	exports := l.e.vm.NewObject()
	module := l.e.vm.NewObject()
	_ = module.Set("exports", exports)

	if rawURL != "" {
		l.mu.Lock()
		l.cache[rawURL] = exports
		l.mu.Unlock()
	}

	rewritten, err := rewriteModule(l.e.vm, src, rawURL)
	if err != nil {
		return nil, fmt.Errorf("模块 %s 改写失败: %w", rawURL, err)
	}
	code := "(function(__exports, module, exports){" + rewritten +
		"\n;if(Object.keys(__exports).length>0){return __exports;}" +
		"return module.exports;})"
	fnVal, err := l.e.vm.RunString(code)
	if err != nil {
		return nil, fmt.Errorf("模块 %s 编译失败: %w", rawURL, err)
	}
	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		return nil, fmt.Errorf("模块 %s 不是可执行函数", rawURL)
	}
	result, err := fn(goja.Undefined(), exports, module, exports)
	if err != nil {
		l.mu.Lock()
		delete(l.cache, rawURL)
		l.mu.Unlock()
		return nil, fmt.Errorf("模块 %s 执行失败: %w", rawURL, err)
	}
	return result, nil
}

func (e *Engine) fetchModule(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", crawlerUA)
	resp, err := e.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载模块失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载模块 %s 返回 HTTP %d", rawURL, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("读取模块失败: %w", err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "<") {
		return "", fmt.Errorf("模块 %s 返回了 HTML 而不是 JS", rawURL)
	}
	return string(b), nil
}

func (e *Engine) loadESM(ctx context.Context, src, baseURL string) error {
	l := &moduleLoader{e: e, ctx: ctx, cache: map[string]goja.Value{}}
	_ = e.vm.Set("__module", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(e.vm.NewTypeError("__module 缺少 URL"))
		}
		v, err := l.load(call.Argument(0).String())
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		return v
	})
	exports, err := l.loadSource(src, baseURL)
	if err != nil {
		return err
	}
	return e.promoteModuleExports(exports)
}

func (e *Engine) promoteModuleExports(exports goja.Value) error {
	if exports == nil || goja.IsUndefined(exports) || goja.IsNull(exports) {
		return nil
	}
	obj := exports.ToObject(e.vm)
	// 先提升直接导出。
	for _, key := range obj.Keys() {
		if key == "default" || key == "__jsEvalReturn" {
			continue
		}
		_ = e.vm.Set(key, obj.Get(key))
	}
	// drpyS/ESM 入口：调用 __jsEvalReturn()，提升返回对象。
	if fn, ok := goja.AssertFunction(obj.Get("__jsEvalReturn")); ok {
		v, err := fn(goja.Undefined())
		if err != nil {
			return fmt.Errorf("__jsEvalReturn 执行失败: %w", err)
		}
		if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			ret := v.ToObject(e.vm)
			for _, key := range ret.Keys() {
				_ = e.vm.Set(key, ret.Get(key))
			}
		}
		return nil
	}
	// FongMi js0：export default 对象。
	if def := obj.Get("default"); def != nil && !goja.IsUndefined(def) && !goja.IsNull(def) {
		if o, ok := def.(*goja.Object); ok {
			for _, key := range o.Keys() {
				_ = e.vm.Set(key, o.Get(key))
			}
		}
	}
	return nil
}
```

在 `crawler.go` 中替换 `Load`/`LoadFromURL` 的执行路径：

```go
func (e *Engine) Load(src string) error {
	return e.loadWithContext(context.Background(), src, "")
}

func (e *Engine) loadWithContext(ctx context.Context, src, baseURL string) error {
	if e == nil || e.vm == nil {
		return fmt.Errorf("爬虫引擎未初始化")
	}
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
```

`LoadFromURL` 最后改为：

```go
return e.loadWithContext(ctx, string(b), u)
```

把 `ErrESModuleUnsupported` 的文案改为：

```go
var ErrESModuleUnsupported = errors.New(
	"脚本使用了当前加载器不支持的 ES Module 语法（支持 import、export default、export function/const/let/var、export {}）")
```

同步更新 `crawler_test.go` 的 `TestLoadReportsESModuleUnsupported`，改用
`export * from "https://x/y.js"` 触发：

```go
func TestLoadReportsESModuleUnsupported(t *testing.T) {
	e := New()
	err := e.Load(`export * from "https://x/y.js";`)
	if err == nil || !errors.Is(err, ErrESModuleUnsupported) {
		t.Fatalf("不支持的 ESM 语法应报错: %v", err)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/crawler -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/esm.go internal/crawler/crawler.go internal/crawler/esm_test.go internal/crawler/crawler_test.go
git commit -m "feat(crawler): 增加 ESM 模块加载器与导出提升"
```

---

### Task 8: `__jsEvalReturn` 与 Provider 端到端

**Files:**
- Modify: `internal/provider/tvbox/spider_test.go`

**Interfaces:**
- Consumes: Task 7 的 ESM 加载器；现有 `tvbox.Spider`。
- Produces: 端到端测试：ESM 脚本暴露 `__jsEvalReturn` → Provider 可调用 `Home`。

- [ ] **Step 1: 写失败测试**

在 `spider_test.go` 增加：

```go
func TestSpiderESMJsEvalReturn(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dep.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`export default "1";`))
	})
	mux.HandleFunc("/spider.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import tid from "./dep.js";
export function __jsEvalReturn(){
  return { homeContent(){ return JSON.stringify({class:[{type_id:tid,type_name:"电影"}]}) } }
}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "esm", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil { t.Fatal(err) }
	sections, err := p.Home(context.Background())
	if err != nil { t.Fatal(err) }
	if len(sections) != 1 || sections[0].ID != "1" || sections[0].Title != "电影" {
		t.Fatalf("Home = %#v", sections)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox -run TestSpiderESMJsEvalReturn -count=1`
Expected: FAIL（当前 ESM 加载失败）

- [ ] **Step 3: 确认实现**

本任务不需要新实现；Task 7 完成后测试应通过。如果失败，检查：
- `promoteModuleExports` 是否把 `homeContent` 提升为全局；
- `homeContent` 返回值是 JSON 字符串，`decodeActionValue` 是否解析。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/spider_test.go
git commit -m "test(spider): 覆盖 ESM __jsEvalReturn 端到端"
```

---

### Task 9: 真实站点验收与文档同步

**Files:**
- Create: `docs/2026-10-17-m7-acceptance.md`
- Modify: `docs/HANDOFF.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/specs/2026-08-29-unbox-m5-js-engine-design.md`

- [ ] **Step 1: 跑全量验证**

```bash
go test ./... -count=1
go vet ./...
gofmt -l .
CGO_ENABLED=1 go build ./...
```

- [ ] **Step 2: 真实站点验收**

drpy2（Provider 选源）：

```bash
UNBOX_REAL_JS_PATH=/tmp/drpy-research/scripts/drpy2_360.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

或用真实配置里的 `api=drpy2.min.js` + `ext=某影视站.js`，确认 Provider 实际加载
的是 `ext`。

ESM：

```bash
UNBOX_REAL_JS_URL=https://clun.example.com/js/某短剧站.js \
UNBOX_REAL_SEARCH=测试 \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

```bash
UNBOX_REAL_JS_URL=https://raw.example.com/某规则仓库/gao/master/lib/lf_search3_min.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

把分类数、列表首项、详情线路/集数、播放地址前缀写入
`docs/2026-10-17-m7-acceptance.md`。

- [ ] **Step 3: 更新文档**

- `docs/HANDOFF.md`：M7 进度 + 已知限制 + 后续待办。
- `README.md`：支持源表、路线图、已知限制。
- `AGENTS.md`：在 JAR 指针后增加 M7 spec/plan 指针。
- M5 spec：在 M5.1 处标注「2026-10-17 修正：drpy2 站点脚本是 var rule；
  ESM 见 M7」。

- [ ] **Step 4: 提交**

```bash
git add docs/2026-10-17-m7-acceptance.md docs/HANDOFF.md README.md AGENTS.md docs/superpowers/specs/2026-08-29-unbox-m5-js-engine-design.md
git commit -m "docs: 同步 M7 drpy2/ESM 兼容进度与验收结果"
```

---

## 后续任务（Task 10–12，已实施）

### Task 10: 真实站点暴露的 console / 掩码修复

- 注入 `console` shim（`log/info/warn/error/debug` → 日志）。
- `maskNonCode` 识别正则字面量、嵌套模板字面量、无空格 `export default{`。
- 结果：`某短剧站.js` 恢复 9 个分类；`lf_search3_min.js` 恢复编译加载。

### Task 11: setTimeout / clearTimeout shim

- 注入同步 `setTimeout`（等待上限 1000ms，返回 0）与 no-op `clearTimeout`。
- 结果：`lf_search3_min.js` 不再因 `setTimeout` 中断；新阻塞点是脚本自身对
  Netflix 榜单 URL 的 3000ms 超时短于本机约 5s 的访问耗时。

### Task 12: 最终 review 修复波

- `export async function` 改写；`export default` 对象内嵌 `__jsEvalReturn` 提升。
- 显式拒绝 `export * as ns from`、`export {…} from`、`export class`。
- cheerio 换成官方 npm `cheerio@1.0.0-rc.12` + esbuild 0.25.0 bundle，
  许可证（含打包依赖）随各平台安装包/应用资源分发。
- drpy2 引擎库识别放宽为 `drpy*.js`（忽略 `?`/`#`）；ext 带 query 也识别为 js。
- `Spider.Home` 在分类失败且无 rule/home 回退时返回原始错误。
- loader 失败清理预执行缓存；主模块错误标签用 `<main>`。
- 新增 vendored cheerio/crypto-js 执行测试。

---

## Self-Review

**Spec coverage:**
- M7.1 选源 → Tasks 1–2。
- M7.2 ESM 加载器 → Tasks 3–8。
- M7.3 cat.js shim → 不在本计划（spec 中列为后续）。
- M7.4 验收/文档 → Task 9。
- 许可证约束 → Task 6（只内置 MIT 资源）+ Global Constraints。

**Placeholder scan:** 无 TBD/TODO；每个代码步骤都有可执行代码与测试命令。
Task 8 依赖 Task 7，已显式说明；Task 7 的测试覆盖相对模块与 IIFE 隔离。

**Type consistency:**
- `Site.ExtString`、`spiderScriptURL`、`stripAsyncAwait`、`maskNonCode`、
  `needsModuleLoader`、`rewriteModule`、`moduleLoader.load`、
  `Engine.loadESM`、`promoteModuleExports` 的签名在各任务间一致。
- `assetSource` 在 Task 6 定义，Task 7 的 `moduleLoader.load` 使用。
