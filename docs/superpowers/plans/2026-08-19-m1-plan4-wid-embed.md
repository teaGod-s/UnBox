# Unbox M1 Plan 4 实现计划：mpv `--wid` 窗口嵌入（Linux + Windows）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 mpv 的视频画面从「独立窗口」改为「嵌入 Unbox 主窗口内」，达成 M1 验收 §7 的「画面嵌入主窗口」。Linux 用 `--wid=<X11 XID>`，Windows 用 `--wid=<HWND>`。macOS 走 libmpv（本计划不涉及，仍顺延待 macOS 机器）。

**Architecture:** mpvproc 增加可选的嵌入窗口句柄（`SetEmbedWindow`），Load 时按句柄有无在 `--wid` 与 `--force-window` 之间切换；shell 层新增平台相关的 `NativeWindowID()` 把 Wails 的 `NativeWindow()` 转成 XID/HWND（Linux 需 cgo 调 `gdk_x11_window_get_xid`）；main 在窗口显示后拿到句柄并注入 mpvproc。控制条采用 mpv 自带 OSC（用户已选），前端不改。

**Tech Stack:** Go 1.26.3、Wails v3.0.0-beta.9（钉死）、mpv 子进程 + JSON IPC、cgo（仅 Linux 的 `embed_linux.go`）。

**Spec:** `docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（§3.4 播放层按平台分叉、§7 验收「三平台画面均嵌入主窗口内」）

## Global Constraints

- Go 1.26.3；module 路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/player/` 不得 import Wails。
- 新增 cgo 仅限 `internal/shell/embed_linux.go`（`//go:build linux`），Windows/macOS 不引入 cgo（用 build tag 隔离）。
- 嵌入失败/未拿到句柄时必须**优雅回退**到 `--force-window`（独立窗口），不得 panic、不得破坏既有播放链路。
- 公开错误信息/注释用中文。
- TDD；提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；Linux 额外 `CGO_ENABLED=1 go build ./...`；Windows 用 `GOOS=windows go build ./internal/player/... ./internal/shell/...`（编译检查，非运行）。

## 关键事实（已从 Wails v3 beta.9 源码核实）

- `WebviewWindow.NativeWindow() unsafe.Pointer`（`webview_window.go:1649`）：
  - Windows：返回 `unsafe.Pointer(hwnd)`，即 HWND，直接可用。
  - Linux：返回 `unsafe.Pointer(w.window)`，即 GtkWidget 指针，需 cgo `gtk_widget_get_window` + `gdk_x11_window_get_xid` 转 XID。
- X11 的 GtkWidget 在窗口「realize（显示）」后 `gtk_widget_get_window` 才非 NULL，故句柄须在窗口显示后获取（本计划用轮询 + 回退）。

---

### Task 1: mpvproc `--wid` 支持 + OSC 切换

**Files:**
- Modify: `internal/player/player.go`（新增 `Embedder` 接口）
- Modify: `internal/player/mpvproc/mpvproc.go`（wid 字段 + SetEmbedWindow + buildArgs 抽取）
- Test: `internal/player/mpvproc/args_test.go`（新建）

**Interfaces:**
- Consumes: 无新增。
- Produces: `player.Embedder`（`SetEmbedWindow(id uintptr)`）；`mpvProc` 实现它。

- [ ] **Step 1: 写失败测试**

`internal/player/mpvproc/args_test.go`：

```go
package mpvproc

import (
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestBuildArgsForceWindowWhenNoWid(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 0)
	if !contains(args, "--force-window=yes") {
		t.Fatalf("无 wid 时应有 --force-window=yes: %v", args)
	}
	if contains(args, "--wid=") {
		t.Fatalf("无 wid 时不应有 --wid: %v", args)
	}
	if !contains(args, "--osc=no") {
		t.Fatalf("无 wid 时应关 OSC: %v", args)
	}
}

func TestBuildArgsWidWhenSet(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 42)
	if !contains(args, "--wid=42") {
		t.Fatalf("有 wid 时应有 --wid=42: %v", args)
	}
	if contains(args, "--force-window") {
		t.Fatalf("有 wid 时不应有 --force-window: %v", args)
	}
	if !contains(args, "--osc=yes") {
		t.Fatalf("有 wid 时应开 OSC: %v", args)
	}
}

func TestBuildArgsCarriesHeaders(t *testing.T) {
	s := player.Stream{URL: "http://x/a.m3u8", Headers: map[string]string{"Referer": "http://x/"}}
	args := buildArgs(s, "/tmp/x.sock", 0)
	if !contains(args, "--http-header-fields=Referer: http://x/") {
		t.Fatalf("应携带 http header: %v", args)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/player/mpvproc/ -run TestBuildArgs -v`
Expected: FAIL（`undefined: buildArgs`）

- [ ] **Step 3: 实现**

`internal/player/player.go` 末尾新增：

```go
// Embedder 是可嵌入到指定宿主窗口的播放器（mpvproc 用 --wid 实现）。
// mpvlib（macOS）经 CAMetalLayer 分层渲染，不需要宿主窗口句柄，故不实现本接口。
type Embedder interface {
	SetEmbedWindow(id uintptr)
}
```

`internal/player/mpvproc/mpvproc.go`：

1. struct 增加字段 `wid uintptr`（由 `lifecycleMu` 守护，与 conn/cmd 同级）。
2. 新增方法：

```go
// SetEmbedWindow 设置 mpv 嵌入的宿主窗口句柄（X11 XID / Windows HWND）。
// 为 0 表示不嵌入，Load 时回退为 --force-window 独立窗口。
func (p *mpvProc) SetEmbedWindow(id uintptr) {
	p.lifecycleMu.Lock()
	p.wid = id
	p.lifecycleMu.Unlock()
}
```

3. 把 Load 里构造 args 的代码抽成纯函数，并在 Load 里快照 wid：

```go
// buildArgs 构造 mpv 启动参数。wid != 0 时嵌入宿主窗口并开 OSC；
// wid == 0 时独立开窗并关 OSC（前端控制）。
func buildArgs(s player.Stream, ipcPath string, wid uintptr) []string {
	args := []string{
		"--idle=yes",
		"--input-ipc-server=" + ipcPath,
		"--keep-open=yes",
		"--volume=80",
	}
	if wid != 0 {
		args = append(args, "--wid="+strconv.FormatUint(uint64(wid), 10), "--osc=yes")
	} else {
		args = append(args, "--force-window=yes", "--osc=no")
	}
	for k, v := range s.Headers {
		args = append(args, "--http-header-fields="+k+": "+v)
	}
	args = append(args, s.URL)
	return args
}
```

Load 中原来：

```go
args := []string{
	"--idle=yes",
	"--input-ipc-server=" + ipcPath,
	"--force-window=yes",
	"--osc=no",
	"--keep-open=yes",
	"--volume=80",
}
for k, v := range s.Headers {
	args = append(args, "--http-header-fields="+k+": "+v)
}
args = append(args, s.URL)
```

改为：

```go
p.lifecycleMu.Lock()
wid := p.wid
p.lifecycleMu.Unlock()
args := buildArgs(s, ipcPath, wid)
```

（`strconv` 加入 import。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/player/mpvproc/ -count=1 -v`
Expected: PASS（buildArgs 3 用例 + 既有 IPC/LoadPlayClose/ConcurrentReload 全绿）

- [ ] **Step 5: Commit**

```bash
git add internal/player/player.go internal/player/mpvproc/mpvproc.go internal/player/mpvproc/args_test.go
git commit -m "feat(mpvproc): --wid 嵌入支持 + Embedder 接口 + OSC 切换"
```

---

### Task 2: shell 原生窗口句柄提取（Linux cgo XID / Windows HWND）

**Files:**
- Create: `internal/shell/embed_linux.go`（`//go:build linux`，cgo）
- Create: `internal/shell/embed_windows.go`（`//go:build windows`）
- Create: `internal/shell/embed_other.go`（`//go:build !linux && !windows`）
- Test: `internal/shell/embed_test.go`（Linux 下验证函数存在；XID 非零需真实窗口，用「未创建窗口返回 0」的最小断言）

**Interfaces:**
- Consumes: `application.WebviewWindow.NativeWindow()`。
- Produces: `func NativeWindowID(w *application.WebviewWindow) uintptr`。

- [ ] **Step 1: 前置检查（Linux）**

```bash
pkg-config --exists gdk-x11-3.0 && echo "gdk-x11-3.0 OK" || echo "缺 libgtk-3-dev（需 apt install libgtk-3-dev）"
```
确认 gdk-x11-3.0 可用（Wails 已用 gtk3，一般已装）。

- [ ] **Step 2: 写失败测试**

`internal/shell/embed_test.go`：

```go
package shell

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestNativeWindowIDNilWindow(t *testing.T) {
	// 一个尚未创建/显示的窗口，其 NativeWindow() 应为 nil，NativeWindowID 返回 0。
	var w *application.WebviewWindow
	if NativeWindowID(w) != 0 {
		t.Fatalf("NativeWindowID(nil) = 非 0")
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/shell/ -run TestNativeWindowID -v`
Expected: FAIL（`undefined: NativeWindowID`）

- [ ] **Step 4: 实现**

`internal/shell/embed_linux.go`：

```go
//go:build linux

package shell

/*
#cgo pkg-config: gtk+-3.0 gdk-x11-3.0
#include <gtk/gtk.h>
#include <gdk/gdkx.h>

static unsigned long unbox_window_xid(void* widget) {
	GdkWindow* gdk = gtk_widget_get_window((GtkWidget*)widget);
	if (gdk == NULL) {
		return 0;
	}
	return gdk_x11_window_get_xid(gdk);
}
*/
import "C"
import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// NativeWindowID 返回用于 mpv --wid 嵌入的 X11 窗口 ID（XID）。
// 窗口未 realize 时返回 0（调用方应回退为独立窗口）。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	if w == nil {
		return 0
	}
	p := w.NativeWindow()
	if p == nil {
		return 0
	}
	return uintptr(C.unbox_window_xid(p))
}
```

`internal/shell/embed_windows.go`：

```go
//go:build windows

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// NativeWindowID 返回用于 mpv --wid 嵌入的 HWND。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	if w == nil {
		return 0
	}
	p := w.NativeWindow()
	if p == nil {
		return 0
	}
	return uintptr(p)
}
```

`internal/shell/embed_other.go`：

```go
//go:build !linux && !windows

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// NativeWindowID 在非 mpvproc 平台（darwin，走 mpvlib）恒返回 0。
func NativeWindowID(w *application.WebviewWindow) uintptr { return 0 }
```

- [ ] **Step 5: 运行确认通过**

Run: `CGO_ENABLED=1 go test ./internal/shell/ -count=1 -v` 及 `go vet ./...`、`gofmt -l internal/shell/`
Expected: PASS、vet/gofmt 干净。

> 注：Windows 的 `embed_windows.go` 编译检查**不能在 Linux 做**——shell 依赖 Wails，而 Wails 在 Windows 用 cgo，交叉编译不可行（spec §5.2）。Windows 侧的编译+运行验证须在 Windows 宿主机原生做（见验收对照）。本任务 Linux 侧只验证 `embed_linux.go` 的 cgo 编译与 `embed_other.go` 的 build-tag 互斥。

- [ ] **Step 6: Commit**

```bash
git add internal/shell/embed_linux.go internal/shell/embed_windows.go internal/shell/embed_other.go internal/shell/embed_test.go
git commit -m "feat(shell): 原生窗口句柄提取（Linux XID cgo / Windows HWND）"
```

---

### Task 3: main 接线（轮询注入 + 优雅回退）

**Files:**
- Modify: `internal/shell/app.go`（`OpenWindow` 返回 `*application.WebviewWindow`）
- Modify: `cmd/unbox/main.go`（拿句柄 + 注入 Embedder + 回退）

**Interfaces:**
- Consumes: `player.Embedder`（Task 1）、`shell.NativeWindowID`（Task 2）、`shell.OpenWindow` 返回窗口。
- Produces: 无。

- [ ] **Step 1: 改 OpenWindow 返回窗口**

`internal/shell/app.go` 的 `OpenWindow` 从 `func OpenWindow(app *application.App)` 改为：

```go
// OpenWindow 在 app 上创建并打开主窗口，并返回窗口（供后续拿原生句柄嵌入播放）。
func OpenWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Unbox",
		Width:            1000,
		Height:           618,
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})
}
```

- [ ] **Step 2: 改 main.go 接线**

`cmd/unbox/main.go`：

```go
func main() {
	p, err := shell.PickPlayer()
	if err != nil {
		log.Printf("播放器初始化失败（继续以未就绪状态启动）: %v", err)
	}
	pl := p // player.Player，可能 nil
	if p != nil {
		pl = failover.New(p, probe.NewProber())
	}
	st, serr := store.Open(appDataPath())
	if serr != nil {
		log.Printf("数据库初始化失败（收藏/最近不可用）: %v", serr)
	}
	var pv provider.Provider
	app := shell.NewApp(pl, pv, st)
	win := shell.OpenWindow(app)

	// 窗口显示后拿原生句柄注入 mpvproc，实现嵌入；拿不到则回退独立窗口。
	if embed, ok := p.(player.Embedder); ok {
		go embedWindow(embed, win)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// embedWindow 轮询等待窗口 realize 后拿到 XID/HWND 并注入；超时则回退。
func embedWindow(embed player.Embedder, win *application.WebviewWindow) {
	for i := 0; i < 100; i++ {
		if id := shell.NativeWindowID(win); id != 0 {
			embed.SetEmbedWindow(id)
			log.Printf("已启用窗口嵌入 (id=%d)", id)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("未能在超时内获取原生窗口句柄，回退为独立窗口播放")
}
```

（`main.go` import 增加：`time`、`github.com/unbox/unbox/internal/player`、`github.com/wailsapp/wails/v3/pkg/application`。）

- [ ] **Step 3: 编译 + 构建验证**

Run: `go build ./...` 与 `mise run build:linux`（产出 `bin/unbox`，确认嵌入代码编入且 cgo 正常链接）
Expected: build 通过、bin/unbox 产出。

- [ ] **Step 4: 冒烟（人工，WSLg）**

Run: `./bin/unbox` → 导入 `/tmp/unbox-smoke/sample.m3u` → 点击播放。
Expected: 视频出现在**主窗口内**（不再弹独立窗口），鼠标移到视频上出现 mpv OSC（进度/暂停/音量/关闭）；点 OSC 的 ✕ 关闭后回到频道列表。

- [ ] **Step 5: Commit**

```bash
git add internal/shell/app.go cmd/unbox/main.go
git commit -m "feat(main): --wid 嵌入接线（轮询注入 + 回退）"
```

---

## 验收对照（spec §7）

- [ ] `unbox` 在 Linux 编译通过、`bin/unbox` 产出（Task 3）
- [ ] Linux 画面嵌入主窗口内（mpv OSC 控制，无独立窗口）（Task 3 冒烟）
- [ ] Windows 画面嵌入（`embed_windows.go` 的 HWND 逻辑 + `--wid=<HWND>`，需在 Windows 宿主机原生编译+运行验证，WSL 不可交叉编译）
- [ ] macOS 画面嵌入（libmpv + CAMetalLayer，仍需 macOS 机器，顺延）

## 停车项（转入后续）

- 前端主题化控制条（当前用 mpv OSC；后续可选「独立浮层控制窗」或回退前端控制）。
- 播放区精确布局（当前 mpv 占满窗口；后续可做「视频区 + 控制区」的 GTK 容器布局）。
- 窗口 resize 时 mpv 子窗口的跟随（依赖 mpv 对父窗口 resize 的处理，实测确认）。
