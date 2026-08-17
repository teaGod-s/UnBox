# Unbox M1 — Plan 1：配置解析层 + unbox-scan CLI 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建能解析真实 TVBox 订阅源的容错配置解析层，以及基于它的 `unbox-scan` 兼容性体检 CLI。

**Architecture:** 五段式管线 `Fetch → Decode → Lenient → Resolve → Config`。除 Fetch 外全部为无 IO 的纯函数，逐段独立可测。解析正确性由 `testdata/configs/` 中 7 个真实样本定义，而非由设计假设定义。

**Tech Stack:** Go 1.26.3（mise 管理）、标准库为主、`modernc.org/sqlite`（后续 Plan 使用）。本 Plan 零 GUI 依赖、零 cgo。

**Spec:** `docs/superpowers/specs/2026-08-17-unbox-m1-design.md`

## Global Constraints

- Go 版本 `1.26.3`，由 `mise.toml` 钉死。
- 本 Plan 所有代码**不得引入 cgo**，必须能 `GOOS=windows/darwin/linux` 交叉编译通过。
- 模块路径：`github.com/unbox/unbox`。
- **禁止用正则剥离 JSON 注释**：实测 `line-12.raw` 解码后含 441 个 `//`，其中 433 个是 URL 协议分隔符，仅 7 个是真注释。必须使用带字符串状态跟踪的字符扫描器。
- 所有解析器必须对畸形输入返回 error，**不得 panic**。
- 测试命令统一为 `mise run test`（等价 `go test ./...`）。
- 提交信息使用中文，遵循 `type: 描述` 格式。

---

## File Structure

| 文件 | 职责 |
|---|---|
| `go.mod` | 模块定义 |
| `mise.toml` | 工具链与任务 |
| `internal/config/model.go` | 统一数据模型（Config/Site/Live/Channel） |
| `internal/config/lenient.go` | 容错 JSON → 标准 JSON（字符扫描器） |
| `internal/config/decode.go` | 探测式解码（`**base64`/裸 base64/gzip/BOM/明文） |
| `internal/config/fetch.go` | 拉取（HTTP UA 伪装 / file:// / clan://） |
| `internal/config/resolve.go` | 多仓递归展开（深度上限 + 环检测） |
| `internal/config/classify.go` | 站点类型分类（JAR/JS/CMS/Python/remote） |
| `cmd/unbox-scan/main.go` | CLI 入口：文本报告 + `--json` |

每个文件一个职责，测试文件与实现同目录同名加 `_test.go`。

---

## Task 1：项目骨架与工具链

**Files:**
- Create: `go.mod`, `mise.toml`, `internal/config/model.go`, `internal/config/model_test.go`

**Interfaces:**
- Consumes: 无（首个任务）
- Produces: `config.Config`, `config.Site`, `config.Live`, `config.Channel`, `config.StoreHouse`, `config.SiteType` 及常量 `SiteTypeXPath/CMS/Spider/Remote`

- [ ] **Step 1：初始化模块与工具链**

创建 `go.mod`：
```
module github.com/unbox/unbox

go 1.26.3
```

创建 `mise.toml`：
```toml
[tools]
go = "1.26.3"

[tasks.test]
run = "go test ./..."

[tasks.scan]
run = "go run ./cmd/unbox-scan"

[tasks.build]
run = "go build -o bin/ ./cmd/..."
```

- [ ] **Step 2：写失败测试**

创建 `internal/config/model_test.go`：
```go
package config

import "testing"

func TestSiteTypeString(t *testing.T) {
	cases := []struct {
		in   SiteType
		want string
	}{
		{SiteTypeXPath, "xpath"},
		{SiteTypeCMS, "cms"},
		{SiteTypeSpider, "spider"},
		{SiteTypeRemote, "remote"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("SiteType(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigCountsZeroValue(t *testing.T) {
	var c Config
	if len(c.Sites) != 0 || len(c.Lives) != 0 {
		t.Error("零值 Config 应当没有站点和直播源")
	}
}
```

- [ ] **Step 3：运行测试确认失败**

Run: `go test ./internal/config/ -run TestSiteType -v`
Expected: FAIL，编译错误 `undefined: SiteType`

- [ ] **Step 4：写最小实现**

创建 `internal/config/model.go`：
```go
// Package config 解析 TVBox 及其衍生分支的订阅配置。
package config

// SiteType 是 TVBox 配置中 site.type 字段的取值。
type SiteType int

const (
	SiteTypeXPath  SiteType = 0
	SiteTypeCMS    SiteType = 1
	SiteTypeSpider SiteType = 3
	SiteTypeRemote SiteType = 4
)

func (t SiteType) String() string {
	switch t {
	case SiteTypeXPath:
		return "xpath"
	case SiteTypeCMS:
		return "cms"
	case SiteTypeSpider:
		return "spider"
	case SiteTypeRemote:
		return "remote"
	default:
		return "unknown"
	}
}

// Site 是一个点播站点。
type Site struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Type     SiteType `json:"type"`
	API      string   `json:"api"`
	Searchable int    `json:"searchable"`
	Ext      any      `json:"ext"`
}

// Channel 是直播源中的一个频道，可含多条备用流。
type Channel struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
	Logo string   `json:"logo"`
	Group string  `json:"group"`
}

// Live 是一组直播源。
type Live struct {
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Channels []Channel `json:"channels"`
}

// StoreHouse 是多仓订阅中的一个仓库入口。
type StoreHouse struct {
	SourceName string `json:"sourceName"`
	SourceURL  string `json:"sourceUrl"`
}

// Config 是解析后的统一配置模型。
type Config struct {
	Spider    string       `json:"spider"`
	Wallpaper string       `json:"wallpaper"`
	Logo      string       `json:"logo"`
	Sites     []Site       `json:"sites"`
	Lives     []Live       `json:"lives"`
	Hosts     []string     `json:"hosts"`
	// StoreHouse 与 URLs 用于多仓/聚合结构，二者非空时表示这是索引而非终端配置。
	StoreHouse []StoreHouse `json:"storeHouse"`
	URLs       []struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	} `json:"urls"`
}
```

- [ ] **Step 5：运行测试确认通过**

Run: `mise run test`
Expected: PASS

- [ ] **Step 6：提交**

```bash
git add go.mod mise.toml internal/config/
git commit -m "feat: 项目骨架与配置数据模型"
```

---

## Task 2：容错 JSON 扫描器（本 Plan 最关键任务）

**Files:**
- Create: `internal/config/lenient.go`, `internal/config/lenient_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `func Lenient(raw []byte) []byte` — 将含注释/尾随逗号/非法控制字符的类 JSON 转为标准 JSON

**背景（必读）**：实测 `line-12.raw` 解码后含 **441 个 `//`，其中 433 个属于 URL**（`http://`、`https://`），仅 **7 个是行注释**。用正则或简单字符串替换会破坏 433 个 URL。必须逐字符扫描并跟踪「当前是否在字符串字面量内」。

- [ ] **Step 1：写失败测试**

创建 `internal/config/lenient_test.go`：
```go
package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLenientPreservesURLSlashes(t *testing.T) {
	in := []byte(`{
//数据接口
"jxurl":"http://jx.84jia.com/x.php?url=",
"api":"https://example.com//double"
}`)
	out := Lenient(in)
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("清洗后仍无法解析: %v\n输出: %s", err, out)
	}
	if m["jxurl"] != "http://jx.84jia.com/x.php?url=" {
		t.Errorf("URL 被破坏: %q", m["jxurl"])
	}
	if m["api"] != "https://example.com//double" {
		t.Errorf("URL 中的双斜杠被破坏: %q", m["api"])
	}
}

func TestLenientStripsBlockComment(t *testing.T) {
	in := []byte(`{/* 说明 */"a":1}`)
	var m map[string]int
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("块注释未被剥离: %v", err)
	}
	if m["a"] != 1 {
		t.Errorf("got %d, want 1", m["a"])
	}
}

func TestLenientStripsTrailingComma(t *testing.T) {
	in := []byte(`{"a":[1,2,3,],"b":{"c":1,},}`)
	var m map[string]any
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("尾随逗号未被处理: %v", err)
	}
}

func TestLenientEscapesControlChars(t *testing.T) {
	// 字符串内的裸制表符/换行是非法 JSON，实测 4 个真实样本含此问题
	in := []byte("{\"name\":\"ab\tcd\"}")
	var m map[string]string
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("控制字符未被转义: %v", err)
	}
	if !strings.Contains(m["name"], "ab") {
		t.Errorf("内容丢失: %q", m["name"])
	}
}

func TestLenientKeepsCommentMarkerInsideString(t *testing.T) {
	in := []byte(`{"note":"这里有 // 和 /* 不是注释"}`)
	var m map[string]string
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if m["note"] != "这里有 // 和 /* 不是注释" {
		t.Errorf("字符串内的注释标记被误删: %q", m["note"])
	}
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestLenient -v`
Expected: FAIL，`undefined: Lenient`

- [ ] **Step 3：写实现**

创建 `internal/config/lenient.go`：
```go
package config

import "bytes"

// Lenient 将 TVBox 生态中常见的非标准 JSON 转换为标准 JSON。
//
// 处理四类问题：
//   1. // 行注释与 /* */ 块注释
//   2. 数组/对象的尾随逗号
//   3. 字符串字面量内的裸控制字符（制表符、换行）
//   4. UTF-8 BOM
//
// 实现为单遍字符扫描器，全程跟踪是否处于字符串字面量内部。
// 这一点不可用正则替代：真实样本中 441 个 "//" 里有 433 个是 URL 协议分隔符。
func Lenient(raw []byte) []byte {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	var out bytes.Buffer
	out.Grow(len(raw))

	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if inString {
			switch {
			case escaped:
				out.WriteByte(c)
				escaped = false
			case c == '\\':
				out.WriteByte(c)
				escaped = true
			case c == '"':
				out.WriteByte(c)
				inString = false
			case c == '\t':
				out.WriteString(`\t`)
			case c == '\n':
				out.WriteString(`\n`)
			case c == '\r':
				out.WriteString(`\r`)
			case c < 0x20:
				// 其余控制字符直接丢弃
			default:
				out.WriteByte(c)
			}
			continue
		}

		// 以下为字符串外部
		if c == '"' {
			out.WriteByte(c)
			inString = true
			continue
		}

		// 行注释
		if c == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			continue
		}

		// 块注释
		if c == '/' && i+1 < len(raw) && raw[i+1] == '*' {
			i += 2
			for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
				i++
			}
			i++ // 循环的 i++ 会跳过 '/'
			continue
		}

		// 尾随逗号：向前看，若下一个非空白字符是 } 或 ]，则丢弃该逗号
		if c == ',' {
			j := i + 1
			for j < len(raw) && isSpace(raw[j]) {
				j++
			}
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}

		out.WriteByte(c)
	}

	return out.Bytes()
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
```

- [ ] **Step 4：运行测试确认通过**

Run: `go test ./internal/config/ -run TestLenient -v`
Expected: 全部 5 个测试 PASS

- [ ] **Step 5：提交**

```bash
git add internal/config/lenient.go internal/config/lenient_test.go
git commit -m "feat: 容错 JSON 扫描器，保护 URL 中的斜杠不被误判为注释"
```

---

## Task 3：探测式解码器

**Files:**
- Create: `internal/config/decode.go`, `internal/config/decode_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `func Decode(raw []byte) ([]byte, error)` — 剥离加密/压缩层，返回明文字节

**背景**：真实配置的加密方式**无任何标识字段**，只能按特征逐一探测。实测形式为 `jhSPAyzn**<base64>`，前缀为随机字符串。

- [ ] **Step 1：写失败测试**

创建 `internal/config/decode_test.go`：
```go
package config

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"os"
	"testing"
)

func TestDecodePlainJSON(t *testing.T) {
	in := []byte(`{"sites":[]}`)
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("明文应当原样返回: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("内容丢失: %s", out)
	}
}

func TestDecodeStarStarBase64(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(`{"sites":[]}`))
	in := []byte("jhSPAyzn**" + payload)
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("**base64 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

func TestDecodeBareBase64(t *testing.T) {
	in := []byte(base64.StdEncoding.EncodeToString([]byte(`{"sites":[]}`)))
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("裸 base64 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

func TestDecodeGzip(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(`{"sites":[]}`))
	w.Close()
	out, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("gzip 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

// 真实样本回归测试
func TestDecodeRealSample(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/configs/line-1.raw")
	if err != nil {
		t.Skipf("样本缺失: %v", err)
	}
	out, err := Decode(raw)
	if err != nil {
		t.Fatalf("真实样本解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte(`"sites"`)) {
		t.Errorf("解码后未见 sites 字段，前 120 字节: %s", out[:min(120, len(out))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestDecode -v`
Expected: FAIL，`undefined: Decode`

- [ ] **Step 3：写实现**

创建 `internal/config/decode.go`：
```go
package config

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
)

// Decode 探测并剥离配置的加密/压缩层，返回明文字节。
//
// TVBox 生态的配置不携带任何加密方式标识，因此只能按特征逐一探测：
//   1. gzip 魔数
//   2. "<随机前缀>**<base64>" 形式（实测见于双龙仓库）
//   3. 裸 base64
//   4. 明文
func Decode(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	if len(raw) == 0 {
		return nil, fmt.Errorf("配置内容为空")
	}

	// 1. gzip
	if len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("gzip 解压失败: %w", err)
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("gzip 读取失败: %w", err)
		}
		return Decode(out) // 解压后可能仍是 base64
	}

	// 2. 明文（已是 JSON）
	if raw[0] == '{' || raw[0] == '[' {
		return raw, nil
	}

	// 3. "<前缀>**<base64>"
	if i := bytes.Index(raw, []byte("**")); i >= 0 && i < 64 {
		if out, ok := tryBase64(raw[i+2:]); ok {
			return Decode(out)
		}
	}

	// 4. 裸 base64
	if out, ok := tryBase64(raw); ok {
		return Decode(out)
	}

	return nil, fmt.Errorf("无法识别的配置编码，前 32 字节: %q", raw[:min(32, len(raw))])
}

func tryBase64(b []byte) ([]byte, bool) {
	s := string(bytes.TrimSpace(b))
	if pad := len(s) % 4; pad != 0 {
		s += string(bytes.Repeat([]byte("="), 4-pad))
	}
	out, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}
```

**注意**：`min` 函数已在 `decode_test.go` 中定义会与实现冲突。将 `min` 从测试文件移除，改为在 `decode.go` 中定义（Go 1.21+ 内置 `min` 支持 int，故直接使用内置即可 —— 删除两处自定义 `min`）。

- [ ] **Step 4：删除自定义 min，使用内置**

在 `decode_test.go` 末尾删除 `func min`，`decode.go` 中不定义 `min`。Go 1.26 内置 `min` 可直接用于 int。

- [ ] **Step 5：运行测试确认通过**

Run: `go test ./internal/config/ -run TestDecode -v`
Expected: 全部 5 个测试 PASS（含真实样本 `line-1.raw`）

- [ ] **Step 6：提交**

```bash
git add internal/config/decode.go internal/config/decode_test.go
git commit -m "feat: 探测式配置解码器，支持 **base64/gzip/裸base64/明文"
```

---

## Task 4：Parse 组合函数与 7 个真实样本回归

**Files:**
- Create: `internal/config/parse.go`, `internal/config/parse_test.go`

**Interfaces:**
- Consumes: `Decode`（Task 3）、`Lenient`（Task 2）、`Config`（Task 1）
- Produces: `func Parse(raw []byte) (*Config, error)`

**这是本 Plan 的验收核心**：7 个真实样本必须全部解析通过，含 spec 中记录的 2 个当前失败用例。

- [ ] **Step 1：写失败测试**

创建 `internal/config/parse_test.go`：
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 全部 7 个真实样本必须解析通过。这是 M1 的硬性验收标准。
func TestParseAllRealSamples(t *testing.T) {
	cases := []struct {
		file      string
		wantKind  string // "index" = 多仓/聚合索引, "config" = 终端配置
	}{
		{"01-storehouse.json", "index"},
		{"02-urls-aggregate.json", "index"},
		{"line-1.raw", "config"},
		{"line-2.raw", "config"},
		{"line-3.raw", "config"},
		{"line-4.raw", "config"},
		{"line-6.raw", "config"},
		{"line-9.raw", "config"},
		{"line-12.raw", "config"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../testdata/configs", c.file))
			if err != nil {
				t.Skipf("样本缺失: %v", err)
			}
			cfg, err := Parse(raw)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			switch c.kindOf() {
			case "index":
				if len(cfg.StoreHouse) == 0 && len(cfg.URLs) == 0 {
					t.Error("索引类配置应含 storeHouse 或 urls")
				}
			case "config":
				if len(cfg.Sites) == 0 && len(cfg.Lives) == 0 {
					t.Error("终端配置应含 sites 或 lives")
				}
			}
		})
	}
}

func (c struct {
	file     string
	wantKind string
}) kindOf() string {
	return c.wantKind
}
```

**注意**：上面的匿名结构体方法写法在 Go 中非法。改用如下正确写法：

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

type sampleCase struct {
	file string
	kind string // "index" 或 "config"
}

func TestParseAllRealSamples(t *testing.T) {
	cases := []sampleCase{
		{"01-storehouse.json", "index"},
		{"02-urls-aggregate.json", "index"},
		{"line-1.raw", "config"},
		{"line-2.raw", "config"},
		{"line-3.raw", "config"},
		{"line-4.raw", "config"},
		{"line-6.raw", "config"},
		{"line-9.raw", "config"},
		{"line-12.raw", "config"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../testdata/configs", c.file))
			if err != nil {
				t.Skipf("样本缺失: %v", err)
			}
			cfg, err := Parse(raw)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			switch c.kind {
			case "index":
				if len(cfg.StoreHouse) == 0 && len(cfg.URLs) == 0 {
					t.Error("索引类配置应含 storeHouse 或 urls")
				}
			case "config":
				if len(cfg.Sites) == 0 && len(cfg.Lives) == 0 {
					t.Error("终端配置应含 sites 或 lives")
				}
			}
		})
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("这不是配置")); err == nil {
		t.Error("畸形输入应当返回 error 而非 nil")
	}
}

func TestParseNeverPanics(t *testing.T) {
	inputs := [][]byte{nil, {}, []byte("{"), []byte("**"), []byte("\x00\x01\x02")}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("输入 %q 触发 panic: %v", in, r)
				}
			}()
			_, _ = Parse(in)
		}()
	}
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestParse -v`
Expected: FAIL，`undefined: Parse`

- [ ] **Step 3：写实现**

创建 `internal/config/parse.go`：
```go
package config

import (
	"encoding/json"
	"fmt"
)

// Parse 将原始订阅内容解析为 Config。
//
// 管线：Decode（剥离加密/压缩）→ Lenient（转为标准 JSON）→ Unmarshal。
func Parse(raw []byte) (*Config, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("配置内容为空")
	}

	decoded, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("解码: %w", err)
	}

	cleaned := Lenient(decoded)

	var cfg Config
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return nil, fmt.Errorf("JSON 解析: %w", err)
	}
	return &cfg, nil
}
```

- [ ] **Step 4：运行测试确认通过**

Run: `go test ./internal/config/ -run TestParse -v`
Expected: 9 个样本子测试全部 PASS

若 `line-2.raw` 仍失败，需诊断其具体语法问题并在 `Lenient` 中补充对应处理，然后重跑。**不得通过跳过该样本来"通过"测试。**

- [ ] **Step 5：提交**

```bash
git add internal/config/parse.go internal/config/parse_test.go
git commit -m "feat: Parse 组合管线，7 个真实样本全部解析通过"
```

---

## Task 5：站点分类器

**Files:**
- Create: `internal/config/classify.go`, `internal/config/classify_test.go`

**Interfaces:**
- Consumes: `Site`（Task 1）
- Produces: `type Support int`，常量 `SupportYes/SupportMaybe/SupportNo`；`func Classify(s Site) (kind string, sup Support)`

- [ ] **Step 1：写失败测试**

创建 `internal/config/classify_test.go`：
```go
package config

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		site     Site
		wantKind string
		wantSup  Support
	}{
		{Site{Type: SiteTypeSpider, API: "csp_Ftyg"}, "jar", SupportNo},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.js"}, "js", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.py"}, "python", SupportNo},
		{Site{Type: SiteTypeSpider, API: "https://x.com/api"}, "http", SupportMaybe},
		{Site{Type: SiteTypeCMS, API: "https://x.com/api.php"}, "cms", SupportYes},
		{Site{Type: SiteTypeXPath, API: "https://x.com"}, "xpath", SupportNo},
		{Site{Type: SiteTypeRemote, API: "http://127.0.0.1:9978"}, "remote", SupportMaybe},
	}
	for _, c := range cases {
		kind, sup := Classify(c.site)
		if kind != c.wantKind || sup != c.wantSup {
			t.Errorf("Classify(%v) = (%q,%v), want (%q,%v)",
				c.site.API, kind, sup, c.wantKind, c.wantSup)
		}
	}
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestClassify -v`
Expected: FAIL，`undefined: Classify`

- [ ] **Step 3：写实现**

创建 `internal/config/classify.go`：
```go
package config

import "strings"

// Support 表示 Unbox 对某个站点的支持程度。
type Support int

const (
	SupportNo    Support = iota // 不支持，且无计划支持
	SupportMaybe                // 可能支持，需运行时验证
	SupportYes                  // 支持
)

func (s Support) String() string {
	switch s {
	case SupportYes:
		return "可用"
	case SupportMaybe:
		return "待定"
	default:
		return "不支持"
	}
}

// Classify 判断站点的爬虫种类及 Unbox 的支持程度。
//
// 依据 spec 第 2.1 节的实测分布：JAR 与 Python 爬虫在桌面端不可行，
// XPath 在当前生态实测占比为 0 且不列入实现。
func Classify(s Site) (kind string, sup Support) {
	api := strings.ToLower(s.API)

	switch s.Type {
	case SiteTypeCMS:
		return "cms", SupportYes
	case SiteTypeXPath:
		return "xpath", SupportNo
	case SiteTypeRemote:
		return "remote", SupportMaybe
	case SiteTypeSpider:
		switch {
		case strings.HasPrefix(api, "csp_"):
			return "jar", SupportNo
		case strings.Contains(api, ".js"):
			return "js", SupportYes
		case strings.Contains(api, ".py"):
			return "python", SupportNo
		case strings.HasPrefix(api, "http"):
			return "http", SupportMaybe
		}
	}
	return "unknown", SupportNo
}
```

- [ ] **Step 4：运行测试确认通过**

Run: `go test ./internal/config/ -run TestClassify -v`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add internal/config/classify.go internal/config/classify_test.go
git commit -m "feat: 站点类型分类器"
```

---

## Task 6：Fetcher（HTTP / file:// / clan://）

**Files:**
- Create: `internal/config/fetch.go`, `internal/config/fetch_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `type Fetcher struct{ Client *http.Client }`；`func NewFetcher() *Fetcher`；`func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error)`

- [ ] **Step 1：写失败测试**

创建 `internal/config/fetch_test.go`：
```go
package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchHTTPSendsOkHTTPUA(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte(`{"sites":[]}`))
	}))
	defer srv.Close()

	f := NewFetcher()
	body, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("拉取失败: %v", err)
	}
	if !strings.Contains(gotUA, "okhttp") {
		t.Errorf("应当伪装为 okhttp UA，实际: %q", gotUA)
	}
	if !strings.Contains(string(body), "sites") {
		t.Errorf("内容不符: %s", body)
	}
}

func TestFetchLocalFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	os.WriteFile(p, []byte(`{"sites":[]}`), 0o644)

	f := NewFetcher()
	body, err := f.Fetch(context.Background(), p)
	if err != nil {
		t.Fatalf("本地文件读取失败: %v", err)
	}
	if !strings.Contains(string(body), "sites") {
		t.Errorf("内容不符: %s", body)
	}
}

func TestFetchClanSchemeRejected(t *testing.T) {
	f := NewFetcher()
	_, err := f.Fetch(context.Background(), "clan://localhost/tvbox/dc.txt")
	if err == nil {
		t.Error("clan:// 在无本地仓库上下文时应当返回明确 error")
	}
	if !strings.Contains(err.Error(), "clan") {
		t.Errorf("错误信息应说明 clan 协议: %v", err)
	}
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestFetch -v`
Expected: FAIL，`undefined: NewFetcher`

- [ ] **Step 3：写实现**

创建 `internal/config/fetch.go`：
```go
package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// okhttpUA 是 TVBox 客户端使用的 User-Agent。部分源站以此做访问控制。
const okhttpUA = "okhttp/3.12.11"

// maxConfigSize 限制单个配置的大小，防止恶意源耗尽内存。
const maxConfigSize = 32 << 20 // 32 MiB

// Fetcher 负责获取配置内容，支持 http(s)、本地路径与 file:// 。
type Fetcher struct {
	Client *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: 60 * time.Second}}
}

// Fetch 获取 ref 指向的配置内容。
//
// clan:// 是 TVBox 用于引用本地仓库内文件的私有协议，脱离客户端本地仓库
// 上下文无法解析，此处返回明确错误而非静默失败。
func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error) {
	switch {
	case strings.HasPrefix(ref, "clan://"):
		return nil, fmt.Errorf("clan:// 协议需要本地仓库上下文，暂不支持: %s", ref)

	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
		if err != nil {
			return nil, fmt.Errorf("构造请求: %w", err)
		}
		req.Header.Set("User-Agent", okhttpUA)
		resp, err := f.Client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, maxConfigSize))

	default:
		p := strings.TrimPrefix(ref, "file://")
		return os.ReadFile(p)
	}
}
```

- [ ] **Step 4：运行测试确认通过**

Run: `go test ./internal/config/ -run TestFetch -v`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add internal/config/fetch.go internal/config/fetch_test.go
git commit -m "feat: 配置拉取器，支持 http/本地文件，clan 协议明确报错"
```

---

## Task 7：Resolver（多仓递归展开）

**Files:**
- Create: `internal/config/resolve.go`, `internal/config/resolve_test.go`

**Interfaces:**
- Consumes: `Fetcher`（Task 6）、`Parse`（Task 4）、`Config`（Task 1）
- Produces: `type Resolver struct{ Fetcher *Fetcher; MaxDepth int }`；`func NewResolver() *Resolver`；`func (r *Resolver) Resolve(ctx context.Context, ref string) ([]*Config, error)`

**背景**：实测结构为三层 `storeHouse → urls[] → 配置`。仓库之间可互相引用，**必须有深度上限与环检测**，否则会无限递归。

- [ ] **Step 1：写失败测试**

创建 `internal/config/resolve_test.go`：
```go
package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveFollowsStoreHouseAndURLs(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"storeHouse":[{"sourceName":"仓A","sourceUrl":"%s/house"}]}`, srv.URL)
	})
	mux.HandleFunc("/house", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/leaf","name":"线路1"}]}`, srv.URL)
	})
	mux.HandleFunc("/leaf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("应当展开到 1 个站点，实际 %d（配置数 %d）", total, len(cfgs))
	}
}

func TestResolveDetectsCycle(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// A 指向 B，B 指回 A
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/b","name":"b"}]}`, srv.URL)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/a","name":"a"}]}`, srv.URL)
	})

	r := NewResolver()
	done := make(chan struct{})
	go func() {
		_, _ = r.Resolve(context.Background(), srv.URL+"/a")
		close(done)
	}()
	select {
	case <-done: // 正常结束即通过
	case <-timeAfter():
		t.Fatal("环引用导致无限递归")
	}
}
```

在同文件末尾补充辅助函数：
```go
func timeAfter() <-chan time.Time {
	return time.After(10 * time.Second)
}
```
并在 import 中加入 `"time"`。

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./internal/config/ -run TestResolve -v`
Expected: FAIL，`undefined: NewResolver`

- [ ] **Step 3：写实现**

创建 `internal/config/resolve.go`：
```go
package config

import (
	"context"
	"fmt"
)

// Resolver 递归展开多仓/聚合结构，直到得到终端配置。
type Resolver struct {
	Fetcher  *Fetcher
	MaxDepth int
}

func NewResolver() *Resolver {
	return &Resolver{Fetcher: NewFetcher(), MaxDepth: 3}
}

// Resolve 从 ref 出发展开所有终端配置。
//
// 索引结构（storeHouse / urls）会被继续展开；含 sites 或 lives 的配置视为
// 终端。深度上限与已访问集合共同防止仓库互相引用导致的无限递归。
func (r *Resolver) Resolve(ctx context.Context, ref string) ([]*Config, error) {
	seen := make(map[string]bool)
	var out []*Config
	err := r.walk(ctx, ref, 0, seen, &out)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return out, nil
}

func (r *Resolver) walk(ctx context.Context, ref string, depth int, seen map[string]bool, out *[]*Config) error {
	if depth > r.MaxDepth {
		return fmt.Errorf("超出最大展开深度 %d", r.MaxDepth)
	}
	if seen[ref] {
		return nil // 环引用，静默跳过
	}
	seen[ref] = true

	raw, err := r.Fetcher.Fetch(ctx, ref)
	if err != nil {
		return fmt.Errorf("拉取 %s: %w", ref, err)
	}
	cfg, err := Parse(raw)
	if err != nil {
		return fmt.Errorf("解析 %s: %w", ref, err)
	}

	// 终端配置
	if len(cfg.Sites) > 0 || len(cfg.Lives) > 0 {
		*out = append(*out, cfg)
	}

	// 索引结构，继续展开。单个子节点失败不中断整体。
	for _, h := range cfg.StoreHouse {
		if h.SourceURL != "" {
			_ = r.walk(ctx, h.SourceURL, depth+1, seen, out)
		}
	}
	for _, u := range cfg.URLs {
		if u.URL != "" {
			_ = r.walk(ctx, u.URL, depth+1, seen, out)
		}
	}
	return nil
}
```

- [ ] **Step 4：运行测试确认通过**

Run: `go test ./internal/config/ -run TestResolve -v`
Expected: PASS，环检测测试在 10 秒内结束

- [ ] **Step 5：提交**

```bash
git add internal/config/resolve.go internal/config/resolve_test.go
git commit -m "feat: 多仓递归展开，含深度上限与环检测"
```

---

## Task 8：unbox-scan CLI

**Files:**
- Create: `cmd/unbox-scan/main.go`, `cmd/unbox-scan/report.go`, `cmd/unbox-scan/report_test.go`

**Interfaces:**
- Consumes: `Resolver`（Task 7）、`Classify`/`Support`（Task 5）、`Config`（Task 1）
- Produces: `type Report struct{...}`；`func BuildReport(cfgs []*config.Config) Report`；`func (r Report) Text() string`；`func (r Report) JSON() ([]byte, error)`

输出格式见 spec 第 4.1 节。

- [ ] **Step 1：写失败测试**

创建 `cmd/unbox-scan/report_test.go`：
```go
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

func TestBuildReportCounts(t *testing.T) {
	cfgs := []*config.Config{
		{
			Sites: []config.Site{
				{Type: config.SiteTypeSpider, API: "csp_A"},
				{Type: config.SiteTypeSpider, API: "csp_B"},
				{Type: config.SiteTypeSpider, API: "http://x/d.js"},
				{Type: config.SiteTypeCMS, API: "http://x/api.php"},
			},
			Lives: []config.Live{{Name: "直播1"}},
		},
	}
	r := BuildReport(cfgs)

	if r.TotalSites != 4 {
		t.Errorf("TotalSites = %d, want 4", r.TotalSites)
	}
	if r.Usable != 2 {
		t.Errorf("Usable = %d, want 2 (js + cms)", r.Usable)
	}
	if r.Unsupported != 2 {
		t.Errorf("Unsupported = %d, want 2 (两个 jar)", r.Unsupported)
	}
	if r.LiveGroups != 1 {
		t.Errorf("LiveGroups = %d, want 1", r.LiveGroups)
	}
	if r.ByKind["jar"] != 2 {
		t.Errorf("ByKind[jar] = %d, want 2", r.ByKind["jar"])
	}
}

func TestReportTextMentionsKeyNumbers(t *testing.T) {
	r := Report{TotalSites: 309, Usable: 25, LiveGroups: 32, ByKind: map[string]int{"jar": 261}}
	out := r.Text()
	for _, want := range []string{"309", "25", "32", "jar"} {
		if !strings.Contains(out, want) {
			t.Errorf("文本报告应包含 %q:\n%s", want, out)
		}
	}
}

func TestReportJSONValid(t *testing.T) {
	r := Report{TotalSites: 10, Usable: 3, ByKind: map[string]int{"js": 3}}
	b, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("输出非合法 JSON: %v", err)
	}
}
```

- [ ] **Step 2：运行测试确认失败**

Run: `go test ./cmd/unbox-scan/ -v`
Expected: FAIL，`undefined: BuildReport`

- [ ] **Step 3：写 Report 实现**

创建 `cmd/unbox-scan/report.go`：
```go
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unbox/unbox/internal/config"
)

// Report 是订阅源的兼容性体检结果。
type Report struct {
	ConfigCount int            `json:"configCount"`
	TotalSites  int            `json:"totalSites"`
	Usable      int            `json:"usable"`
	Maybe       int            `json:"maybe"`
	Unsupported int            `json:"unsupported"`
	ByKind      map[string]int `json:"byKind"`
	LiveGroups  int            `json:"liveGroups"`
	LiveChannels int           `json:"liveChannels"`
}

func BuildReport(cfgs []*config.Config) Report {
	r := Report{ByKind: map[string]int{}, ConfigCount: len(cfgs)}
	for _, c := range cfgs {
		for _, s := range c.Sites {
			kind, sup := config.Classify(s)
			r.TotalSites++
			r.ByKind[kind]++
			switch sup {
			case config.SupportYes:
				r.Usable++
			case config.SupportMaybe:
				r.Maybe++
			default:
				r.Unsupported++
			}
		}
		r.LiveGroups += len(c.Lives)
		for _, l := range c.Lives {
			r.LiveChannels += len(l.Channels)
		}
	}
	return r
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "配置解析\n  成功 %d 个配置\n\n", r.ConfigCount)
	fmt.Fprintf(&b, "点播站点  %d 个\n", r.TotalSites)
	fmt.Fprintf(&b, "  可用    %3d  (%.1f%%)\n", r.Usable, pct(r.Usable, r.TotalSites))
	fmt.Fprintf(&b, "  待定    %3d  (%.1f%%)\n", r.Maybe, pct(r.Maybe, r.TotalSites))
	fmt.Fprintf(&b, "  不支持  %3d  (%.1f%%)\n\n", r.Unsupported, pct(r.Unsupported, r.TotalSites))

	kinds := make([]string, 0, len(r.ByKind))
	for k := range r.ByKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return r.ByKind[kinds[i]] > r.ByKind[kinds[j]] })
	b.WriteString("按类型\n")
	for _, k := range kinds {
		fmt.Fprintf(&b, "  %-8s %d\n", k, r.ByKind[k])
	}

	fmt.Fprintf(&b, "\n直播源    %d 组 / %d 频道\n", r.LiveGroups, r.LiveChannels)
	return b.String()
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
```

- [ ] **Step 4：写 CLI 入口**

创建 `cmd/unbox-scan/main.go`：
```go
// unbox-scan 体检 TVBox 订阅源，报告其中有多少站点可被 Unbox 使用。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/unbox/unbox/internal/config"
)

func main() {
	asJSON := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Minute, "整体超时")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: unbox-scan [--json] <订阅链接或本地文件>")
		os.Exit(2)
	}
	ref := flag.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfgs, err := config.NewResolver().Resolve(ctx, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "体检失败: %v\n", err)
		os.Exit(1)
	}

	rep := BuildReport(cfgs)
	if *asJSON {
		b, err := rep.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "序列化失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}
	fmt.Printf("订阅源: %s\n\n%s", ref, rep.Text())
}
```

- [ ] **Step 5：运行测试确认通过**

Run: `go test ./cmd/unbox-scan/ -v`
Expected: 3 个测试全部 PASS

- [ ] **Step 6：验证编译（M1 验收标准之一）**

```bash
go build ./cmd/...
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/unbox-scan
GOOS=darwin  GOARCH=arm64 go build -o /dev/null ./cmd/unbox-scan
GOOS=linux   GOARCH=amd64 go build -o /dev/null ./cmd/unbox-scan
```
Expected: 全部成功，无 cgo 依赖

- [ ] **Step 7：对真实样本做端到端验证**

```bash
go run ./cmd/unbox-scan testdata/configs/line-3.raw
```
Expected: 输出报告，站点数与 spec 第 2.1 节记录一致（line-3 应为 96 个站点）

- [ ] **Step 8：提交**

```bash
git add cmd/unbox-scan/
git commit -m "feat: unbox-scan 订阅源兼容性体检 CLI"
```

---

## Self-Review 结果

**1. Spec 覆盖检查**

| Spec 章节 | 对应任务 |
|---|---|
| §3.1 Config 数据模型 | Task 1 |
| §3.3 Lenient 段 | Task 2 |
| §3.3 Decoder 段 | Task 3 |
| §3.3 完整管线 | Task 4 |
| §2.1 站点分类 | Task 5 |
| §3.3 Fetcher 段 | Task 6 |
| §3.3 Resolver 段 | Task 7 |
| §4.1 unbox-scan 输出 | Task 8 |
| §6.1 真实样本夹具 | Task 4（9 个子测试） |
| §7 CLI 编译通过 | Task 8 Step 6 |

§3.4 播放层、§3.5 mpv 分发、§4 直播浏览 UI 属于 Plan 2/3 范围，本 Plan 不覆盖 —— 这是有意的范围划分，非遗漏。

**2. 占位符扫描**：无 TBD/TODO；所有代码步骤含完整可运行代码。

**3. 类型一致性**：`Config`/`Site`/`Live`/`Channel`/`StoreHouse`（Task 1）→ 被 Task 4/5/7/8 使用，字段名一致；`Support` 常量（Task 5）→ Task 8 使用一致；`Fetcher`/`Resolver` 构造函数命名一致。已修正 Task 3 中 `min` 函数的重复定义问题、Task 4 测试中非法的匿名结构体方法写法。

---

## 后续 Plan

- **Plan 2**：Wails v3 壳 + Player 接口 + mpvproc（Win/Linux）+ mpvlib（macOS）
- **Plan 3**：直播 Provider + 测速切换 + SQLite 持久化 + Vue 3 前端
