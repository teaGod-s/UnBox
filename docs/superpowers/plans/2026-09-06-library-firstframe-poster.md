# 本地媒体库首帧兜底海报 实现计划

> 状态：已完成并合入 `master`（2026-09-07）。实际实现采用 `--vo-image-outdir` 临时目录，
> 首帧海报只运行时回填，不写入带随机端口/token 的 URL；片单最终布局为播放器与片单间距
> `6px`、片单右侧留白 `12px`、滚动条宽度 `6px`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 本地媒体库中无海报的条目，用视频本身的一帧生成缩略图海报（web-canvas 主路径 + mpv 兜底），懒生成、按 `(path, mtime)` 缓存。

**Architecture:** 后端加三件事——`serve.go` 补 CORS 头（跨源 video 画 canvas 免污染）+ `register` 按路径去重；新 `internal/library/thumb` 包提供 mpv 一次性抓帧（`--vo=image --frames=1`）；`library` 门面新增 `EnsureThumb`/`SaveThumb`/`GenerateThumbMpv` 封装缓存路径与注册。前端在媒体库列表渲染时对无海报条目跑限流管线：cached 检查放开并发、仅「生成」阶段限 1–2。

**Tech Stack:** Go 1.26、Wails v3.0.0-beta.9、Vue 3（`frontend/src/App.vue`）、`internal/library`（已含 `server` 本地 HTTP）。

**Spec:** `docs/superpowers/specs/2026-09-06-local-library-firstframe-poster-design.md`（本计划是对该设计的实现展开，含两处修订：见 Global Constraints 第 3、4 条）。

## Global Constraints

- 不引入 ffmpeg / 新原生依赖 / mpv 强依赖（mpv 兜底为可选，缺席即纯文字）。
- 不改 `Scan()` / DB schema / 既有播放路径；`Poster` 仍以 `""` 入库，前端运行时装饰。
- **修订 A（对设计稿 §4.4）**：前端管线**分两段限流**——cached 检查（`EnsureThumb` 命中 `cached=true`）放开并发（仅便宜 IPC）；只有「生成」段（web-canvas 主路径 + mpv 兜底）限并发 1–2。**不做**「persist posterURL 到 DB」——海报 URL 含随机端口 + token，每次启动变化，持久化无意义。
- **修订 B（对设计稿 §4.3）**：`server.register` 增加**按路径去重**（避免同一视频在「播放」与「抓帧」各注册一次造成 `ids` map 冗余增长）。
- 缓存目录 `<UserConfigDir>/unbox/posters/`，文件 `<sha1(path\x00mtime)>.jpg`。
- 开发走新分支 `feat/library-firstframe-poster`。
- TDD；提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；前端 `npm test`（前端测试目录见仓库 `frontend/`）。
- 公开错误信息/注释用中文。

---

### Task 1: thumb 包 —— mpv 一次性抓帧生成器

**Files:**
- Create: `internal/library/thumb/thumb.go`
- Test: `internal/library/thumb/thumb_test.go`

**Interfaces:**
- Consumes: 无（纯逻辑，依赖 `os/exec`）。
- Produces: `var ErrNoMpv`；`type Generator interface { Generate(videoPath, cachePath string) error }`；`type MpvGenerator struct { mpvPath string; timeout time.Duration }` 及其 `Generate`。

- [x] **Step 1: 写失败测试**

`internal/library/thumb/thumb_test.go`（fake mpv = 一段可执行脚本，写一张最小 JPEG 到
`--vo-image-outdir=` 指定的输出目录）：

```go
package thumb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeMPV 写一个脚本到临时目录，脚本把一个小 JPEG 写到 --vo-image-outdir= 后跟的目录，返回其路径。
func fakeMPV(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-mpv.sh")
	body := `#!/bin/sh
# 提取 --vo-image-outdir=<dirname>，写入一个最小 JPEG 字节
for i in "$@"; do case "$i" in --vo-image-outdir=*) out="${i#--vo-image-outdir=}";; esac; done
mkdir -p "$out"
printf '\xff\xd8\xff\xd9' > "$out/00000001.jpg"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestGenerateWritesCache(t *testing.T) {
	g := &MpvGenerator{mpvPath: fakeMPV(t), timeout: 5 * time.Second}
	cache := filepath.Join(t.TempDir(), "k.jpg")
	if err := g.Generate("/v/a.mkv", cache); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	b, err := os.ReadFile(cache)
	if err != nil || len(b) == 0 {
		t.Fatalf("cache 未写入或为空: %v", err)
	}
}

func TestGenerateNoMpv(t *testing.T) {
	g := &MpvGenerator{mpvPath: "", timeout: 5 * time.Second}
	if err := g.Generate("/v/a.mkv", "/tmp/x.jpg"); !errors.Is(err, ErrNoMpv) {
		t.Fatalf("want ErrNoMpv, got %v", err)
	}
}
```

- [x] **Step 2: 运行确认失败**

Run: `go test ./internal/library/thumb/ -count=1`
Expected: FAIL（`undefined: MpvGenerator` / `undefined: ErrNoMpv`）。

- [x] **Step 3: 实现**

`internal/library/thumb/thumb.go`：

```go
// Package thumb 为本地媒体库无海报条目生成首帧缩略图。
package thumb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrNoMpv 表示 mpv 未安装，无法走兜底抓帧。
var ErrNoMpv = errors.New("mpv 未安装")

// Generator 抽象缩略图生成，便于注入 fake 测试。
type Generator interface {
	Generate(videoPath, cachePath string) error
}

// MpvGenerator 用一次性 mpv 子进程抓帧写 JPEG。
type MpvGenerator struct {
	mpvPath string
	timeout time.Duration
}

// NewMpvGenerator 构造抓帧器；mpvPath 为空时 Generate 返回 ErrNoMpv。
func NewMpvGenerator(mpvPath string, timeout time.Duration) *MpvGenerator {
	return &MpvGenerator{mpvPath: mpvPath, timeout: timeout}
}

// Generate 抓取 videoPath 的 10% 处一帧写入 cachePath。
// mpv 先输出到临时目录，再把生成的图片搬运到缓存路径。
func (g *MpvGenerator) Generate(videoPath, cachePath string) error {
	if g.mpvPath == "" {
		return ErrNoMpv
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	temporaryDir, err := os.MkdirTemp(filepath.Dir(cachePath), ".unbox-thumb-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryDir)
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	// --vo-image-format=jpg 使输出为 JPEG，与 .jpg 扩展一致。
	cmd := exec.CommandContext(ctx, g.mpvPath,
		"--no-config", "--vo=image", "--vo-image-format=jpg",
		"--frames=1", "--start=10%", "--vo-image-outdir="+temporaryDir, videoPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mpv 抓帧失败: %w", err)
	}
	// 从 temporaryDir 找到非空 .jpg/.jpeg 后原子搬运到 cachePath。
	if err := moveGeneratedJpeg(temporaryDir, cachePath); err != nil {
		return fmt.Errorf("mpv 抓帧未产出文件: %s", cachePath)
	}
	return nil
}
```

> 实现：`NewMpvGenerator` 在 `timeout <= 0` 时统一使用默认 15s。

- [x] **Step 4: 运行确认通过**

Run: `go test ./internal/library/thumb/ -count=1`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/library/thumb/thumb.go internal/library/thumb/thumb_test.go
git commit -m "feat(library): thumb 包 mpv 一次性抓帧生成器"
```

---

### Task 2: serve.go 加 CORS 头 + register 按路径去重

**Files:**
- Modify: `internal/library/serve.go`
- Test: `internal/library/serve_test.go`（新建，或并入既有）

**Interfaces:**
- Consumes: `server.register(path)`、`server.handler()`（现状见 `serve.go`）。
- Produces: handler 响应带 `Access-Control-Allow-Origin: *`；`register` 对同一绝对路径返回同一 id/URL。

- [x] **Step 1: 写失败测试**

`internal/library/serve_test.go`：

```go
package library

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerSetsCORS(t *testing.T) {
	s := newServer()
	s.register(t.TempDir() + "/a.mp4")
	req := httptest.NewRequest("GET", s.registeredURL("1")+"/x", nil) // 见下：拿 token
	// 简化：直接带 token 请求已知 id
	req = httptest.NewRequest("GET", "/v/1?t="+s.token, nil)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("CORS 头 = %q, want *", got)
	}
}

func TestRegisterDedupsByPath(t *testing.T) {
	s := newServer()
	p := t.TempDir() + "/a.mp4"
	u1 := s.register(p)
	u2 := s.register(p)
	if u1 != u2 {
		t.Fatalf("同一路径应复用注册: %q != %q", u1, u2)
	}
}
```

（`TestHandlerSetsCORS` 里访问 `s.registeredURL` 是伪代码，实现者按现有 `register` 的返回逻辑自行取得带 token 的 URL，或直接拼 `"/v/1?t="+s.token`。重点是断言 CORS 头。）

- [x] **Step 2: 运行确认失败**

Run: `go test ./internal/library/ -run 'TestHandlerSetsCORS|TestRegisterDedupsByPath' -count=1`
Expected: FAIL（CORS 头缺失 / 去重不成立）。

- [x] **Step 3: 实现**

`serve.go`：

1. `server` struct 增加 `rev map[string]string`（absPath → id）。
2. `newServer` 初始化 `rev: make(map[string]string)`。
3. `register`：`absPath` 计算后，先查 `s.rev[absPath]`，命中则复用现有 id 拼 URL 返回；未命中才 `seq++` 并记入 `ids`/`rev`。
4. `handler` 在 `http.ServeFile` 前加 `w.Header().Set("Access-Control-Allow-Origin", "*")`。

注意：CORS 头加在**鉴权通过之后**、`ServeFile` 之前即可；鉴权失败路径（403）可不加，但加了也无害。

- [x] **Step 4: 运行确认通过 + 回归**

Run: `go test ./internal/library/ -count=1` 及 `go test ./internal/... -count=1`
Expected: PASS；既有 token 鉴权 / 防穿越 / register 用例不回归。

- [x] **Step 5: 提交**

```bash
git add internal/library/serve.go internal/library/serve_test.go
git commit -m "feat(library): serve 加 CORS 头 + register 按路径去重"
```

---

### Task 3: library 门面 —— 缩略图编排（EnsureThumb / SaveThumb / GenerateThumbMpv）

**Files:**
- Modify: `internal/library/library.go`（或新增 `internal/library/thumb_cache.go`）
- Test: `internal/library/thumb_cache_test.go`

**Interfaces:**
- Consumes: `server.register`（Task 2）、`thumb.Generator`（Task 1）、`store.Store`。
- Produces:
  - `func (l *Library) EnsureThumb(path string, mtime int64) (videoURL, posterURL string, cached bool, err error)`
  - `func (l *Library) SaveThumb(path string, mtime int64, jpeg []byte) (posterURL string, err error)`
  - `func (l *Library) GenerateThumbMpv(path string, mtime int64) (posterURL string, err error)`
  - `Library` 新增字段 `postersDir string`、`thumbGen thumb.Generator`；`New` 签名加 `postersDir` 与 `thumbGen` 参数（或新增 setter）。

- [x] **Step 1: 写失败测试**

`internal/library/thumb_cache_test.go`（注入 fake `thumb.Generator`、临时 `postersDir`）：

```go
package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeGen struct{ err error }

func (f fakeGen) Generate(videoPath, cachePath string) error {
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(cachePath, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644)
}

func newTestLib(t *testing.T, gen thumb.Generator) (*Library, string) {
	t.Helper()
	postersDir := filepath.Join(t.TempDir(), "posters")
	l := New(nil, postersDir, gen) // nil store：本测试不触 DB
	return l, postersDir
}

func TestEnsureThumbCacheHit(t *testing.T) {
	l, _ := newTestLib(t, fakeGen{})
	p := filepath.Join(t.TempDir(), "a.mp4")
	_, poster, cached, err := l.EnsureThumb(p, 123)
	if err != nil || cached {
		t.Fatalf("首次应 cached=false: %v %v", err, cached)
	}
	if err := l.SaveThumb(p, 123, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, _, cached, _ = l.EnsureThumb(p, 123); !cached {
		t.Fatal("二次应 cached=true")
	}
	_ = poster
}

func TestGenerateThumbMpvDelegates(t *testing.T) {
	l, _ := newTestLib(t, fakeGen{})
	p := filepath.Join(t.TempDir(), "a.mkv")
	poster, err := l.GenerateThumbMpv(p, 456)
	if err != nil || poster == "" {
		t.Fatalf("GenerateThumbMpv = %q, %v", poster, err)
	}
}

func TestGenerateThumbMpvNoMpv(t *testing.T) {
	l, _ := newTestLib(t, fakeGen{err: thumb.ErrNoMpv})
	_, err := l.GenerateThumbMpv(filepath.Join(t.TempDir(), "a.mkv"), 1)
	if !errors.Is(err, thumb.ErrNoMpv) {
		t.Fatalf("want ErrNoMpv, got %v", err)
	}
}
```

- [x] **Step 2: 运行确认失败**

Run: `go test ./internal/library/ -run 'TestEnsureThumb|TestGenerateThumbMpv' -count=1`
Expected: FAIL（`undefined: EnsureThumb` 等）。

- [x] **Step 3: 实现**

`internal/library/thumb_cache.go`（新文件，包 `library`）：

- cache 键：`sha1(fmt.Sprintf("%s\x00%d", path, mtime))`；`cachePath = filepath.Join(l.postersDir, key+".jpg")`。
- `EnsureThumb`：`cached = fileExists(cachePath)`；`videoURL = l.server.register(path)`（真实文件，始终可访问）；`posterURL = l.server.register(cachePath)`（可能尚未生成，前端仅在生成成功后才挂 `item.Poster`）。返回三者。
- `SaveThumb`：`os.MkdirAll(postersDir)` → 写临时文件 → `os.Rename` 覆盖 `cachePath`（原子）；`singleflight` 按 `key` 去重并发写。返回 `l.server.register(cachePath)`。
- `GenerateThumbMpv`：`l.thumbGen == nil` → `thumb.ErrNoMpv`；否则 `l.thumbGen.Generate(path, cachePath)`，成功 → 返回 `l.server.register(cachePath)`，失败透传。
- `Library` struct 加字段并在 `New` 注入；`New` 签名改为 `New(st *store.Store, postersDir string, thumbGen thumb.Generator)`，并同步改 `internal/shell/service.go` 的调用点（Task 4 一起做，但编译须在 Task 4 才恢复，故 Task 3 末尾先以「临时传 nil」保持编译，或直接把 Task 4 的接线合并到本任务末尾）。

> 说明：`New` 签名变更会暂时打断 `internal/shell/service.go` 的编译。为避免中间态，**Task 3 与 Task 4 作为一个连贯提交或紧邻完成**：Task 3 实现 library 侧 + 单测，Task 4 立即改 service.go 调用点恢复全库编译，再一起 `go build ./...` 验证。计划按两步走但合并提交亦可。

- [x] **Step 4: 运行确认通过**

Run: `go test ./internal/library/ -count=1`
Expected: PASS。

- [x] **Step 5: 提交（与 Task 4 一并编译验证后提交）**

见 Task 4 Step 5。

---

### Task 4: ShellService 绑定 + 注入 generator

**Files:**
- Modify: `internal/shell/service.go`
- Test: `internal/shell/service_test.go`（或并入既有）

**Interfaces:**
- Consumes: `library.Library`（Task 3）、`mpvplugin.Manager.Status().Path`。
- Produces:
  - `func (s *ShellService) EnsureThumb(path string, mtime int64) (videoURL, posterURL string, cached bool, err error)`
  - `func (s *ShellService) SaveThumb(path string, mtime int64, jpeg []byte) (posterURL string, err error)`
  - `func (s *ShellService) GenerateThumbMpv(path string, mtime int64) (posterURL string, err error)`

- [x] **Step 1: 改 library 构造点**

`internal/shell/service.go` 构造 `ShellService` 处（现 `library: library.New(st)`）：

```go
root, _ := os.UserConfigDir()           // 若此处已有 root，直接复用
postersDir := filepath.Join(root, "unbox", "posters")
lib := library.New(st, postersDir, nil) // thumbGen 延迟注入，见下
```

`GenerateThumbMpv` 委托时**懒解析** mpvPath（mpv 可能在启动后才装）：

```go
func (s *ShellService) GenerateThumbMpv(path string, mtime int64) (string, error) {
	if s.library == nil {
		return "", errors.New("媒体库未就绪")
	}
	mpvPath := s.mpvPlugin.Status().Path
	gen := thumb.NewMpvGenerator(mpvPath, 15*time.Second)
	if err := s.library.SetThumbGenerator(gen); err != nil { /* 无此方法则直接传 */ }
	return s.library.GenerateThumbMpv(path, mtime)
}
```

> 具体注入方式二选一，实现者择一落地并保持一致：(a) `Library` 加 `SetThumbGenerator(thumb.Generator)` 方法，`GenerateThumbMpv` 前用懒解析的 generator 覆盖；(b) `GenerateThumbMpv` 直接接受 `thumb.Generator` 参数，由 ShellService 每次构造传入。**推荐 (b)**（更纯，`Library` 不持有可变 generator 状态），即 `Library.GenerateThumbMpv(path, mtime int64, gen thumb.Generator)`。

- [x] **Step 2: 写失败测试（shell 侧，注入 fake）**

断言 `EnsureThumb`/`SaveThumb`/`GenerateThumbMpv` 正确委托 `s.library`（用 fake library 或直接构造带临时目录的 Library）。核心覆盖：`GenerateThumbMpv` 在 mpv 缺席时透传 `thumb.ErrNoMvp`。

- [x] **Step 3: 运行确认失败 → 实现 → 通过**

Run: `go test ./internal/shell/ -count=1`、`go build ./...`
Expected: 先 FAIL 后 PASS；全库编译恢复（`go build ./...` 绿）。

- [x] **Step 4: 全量回归**

Run: `go test ./... -count=1`、`go vet ./...`、`gofmt -l`
Expected: 全绿。

- [x] **Step 5: 提交（含 Task 3 的 library 侧改动）**

```bash
git add internal/library/thumb_cache.go internal/library/thumb_cache_test.go internal/library/library.go internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(library): 缩略图编排 + ShellService 绑定"
```

---

### Task 5: 前端懒生成管线（web-canvas 主路径 + mpv 兜底，cached 检查不限流）

**Files:**
- Modify: `frontend/src/App.vue`
- Test: `frontend/src/__tests__/`（或仓库既有测试目录）新增管线测试

**Interfaces:**
- Consumes: `ShellService.EnsureThumb/SaveThumb/GenerateThumbMpv`（Task 4）、`LibraryItem{Path,MTime,Poster}`。
- Produces: 无导出；内部响应式 `thumbAttempted` Map、`thumbSemaphore`（生成段并发 1–2）。

- [x] **Step 1: 写失败测试（mock ShellService + fake video/canvas）**

覆盖（对应设计稿 §6 前端部分）：
- cached → 直接设 `Poster`，不建 video。
- 主路径成功 → `SaveThumb` 被调、`Poster` 被设。
- 主路径 `error` → 转 `GenerateThumbMpv`。
- 兜底 `ErrNoMpv` → 标记 attempted、`Poster` 仍空。
- **cached 检查段并发不受 1–2 限制；生成段并发 ≤ 2**。
- 重渲染不重试已 attempted 条目。

- [x] **Step 2: 运行确认失败**

Run: `cd frontend && npm test`
Expected: FAIL（管线尚未实现）。

- [x] **Step 3: 实现**

在 `App.vue` 的媒体库列表渲染处，对 `Poster == ""` 且 `!thumbAttempted.has(item.Path)` 的条目触发管线（设计稿 §4.4 流程）：

1. **cached 检查段（放开并发）**：直接并发调 `EnsureThumb(path, mtime)`；`cached=true` → 设 `item.Poster = posterURL` 即结束（不占生成信号量）。
2. **生成段（信号量 1–2）**：`cached=false` 的条目进入信号量队列，先试主路径 web-canvas（隐藏 `<video crossorigin="anonymous">` → `loadeddata` → seek `min(duration*0.1,60)` → `seeked` → `drawImage` → `toBlob('image/jpeg',0.8)` → `SaveThumb`），`error` 则转 `GenerateThumbMpv`；两者都失败 → `thumbAttempted.set(path,true)` 留纯文字。
3. 超时：主路径 ~10s、兜底后端 15s；超时视同失败走下一步。
4. 现有 `imgError`（隐藏坏图）保留兜底。

实现细节以设计稿 §4.4 为准；关键差异仅在**分段限流**（本计划 Global Constraints 修订 A）。

- [x] **Step 4: 运行确认通过 + 生产构建**

Run: `cd frontend && npm test`、`npm run build`
Expected: PASS、构建通过。

- [x] **Step 5: 提交**

```bash
git add frontend/src/App.vue frontend/src/__tests__/
git commit -m "feat(frontend): 媒体库首帧缩略图懒生成管线"
```

---

## 风险与验收（承自设计稿 §7–§8）

- **Canvas 污染**：依赖 CORS + `crossorigin="anonymous"`，三平台 WebView 各验一次；**Linux WebKitGTK 最先验**（GStreamer 解码的 video 画 canvas 最可能出岔子），失败则该平台主路径退化（web 可解码但污染 → 只能 mpv 兜底 → 纯文字）。
- **`--vo=image` 输出参数**：使用 `--vo-image-format=jpg` 与 `--vo-image-outdir=`；
  `--o=` 是通用编码输出参数，不能作为图片输出路径。实现会从临时目录选取非空 JPG 后
  原子搬运到缓存，已用实际 mpv 验证。
- **非 faststart MP4**：seek 10% 可能多拉数据，本地 loopback 下 1–2s 量级，可接受。
- **手动验收**（本地 WSL/Linux）：无海报 MP4 主路径出图 → 无海报 MKV 装 mpv 出图 → 卸 mpv 纯文字不卡 → 重启秒回填不重抓 → 改 mtime 重生成 → 大量条目滚动时生成段限流生效。
