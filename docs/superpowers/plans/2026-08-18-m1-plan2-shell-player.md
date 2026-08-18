# Unbox M1 Plan 2：Wails v3 应用壳 + Player 播放层

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 产出 Unbox 桌面主程序 `unbox`——一个能编译、能启动、能在内嵌窗口里加载并播放一条流的 Wails v3 应用，以及支撑它的 `Player` 接口与按平台分叉的两种播放实现（`mpvproc` 用于 Win/Linux，`mpvlib` 用于 macOS）。

**Architecture:** 分两层。`internal/player/` 是纯 Go 的播放层，定义 `Player` 接口与媒体类型，并给出 `mpvproc`（mpv 子进程 + JSON IPC）与 `mpvlib`（libmpv，macOS）两个实现——**不依赖 Wails**。`internal/shell/` + `cmd/unbox/` + `frontend/` 是 Wails v3 壳层，把 Wails 的 beta API 波动全部收敛在此，业务层（player）零接触 Wails。壳层通过 `Player` 接口调用播放层。

**Tech Stack:** Go 1.26.3、Wails v3（beta，版本钉死）、mpv（子进程 + JSON IPC）、Vue 3 + TypeScript + Tailwind（前端脚手架）。

**Spec:** [docs/superpowers/specs/2026-08-17-unbox-m1-design.md](../specs/2026-08-17-unbox-m1-design.md)（本计划实现其 §3.1 的 Player 接口、§3.4 播放层分叉、§5 工具链，其余 M1 功能属 Plan 3）

## Global Constraints

- Go 1.26.3；module 路径 `github.com/unbox/unbox`。
- Wails v3 版本**钉死**（当前安装为 beta.8，spec 定 beta.9，Task 1 决定最终钉版并显式提交），**禁止用 `@latest`**。
- 交叉编译**不可行**（Wails 与 libmpv 均引入 cgo）。当前机器是 Linux，本计划只要求 **Linux 编译通过**；Windows/macOS 构建交由 CI 三 runner，不在本机验证。
- Wails 相关代码**只允许**出现在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/player/` 及其子包**不得 import Wails**。
- 前端框架 Vue 3 + TypeScript + Tailwind（spec 既定）。
- mpv 随安装包分发，用户无需预装；运行时查找顺序「内置路径 → 系统 PATH」。
- 现有代码（`internal/config/`、`cmd/unbox-scan/`）不动，本计划只新增。

---

### Task 1: mise 工具链补齐 + 系统依赖

**Files:**
- Modify: `mise.toml`

**Interfaces:**
- Consumes: 现有 `mise.toml`（只有 `go` 与 test/scan/build 任务）。
- Produces: `mise.toml` 增加 `node` 与 `wails3` 工具项与 `dev`/`build:*` 任务；本机具备 node / mpv / webkit2gtk。

- [ ] **Step 1: 确认目标 Wails v3 版本**

运行 `wails3 version`，记录当前版本（应为 beta.8）。目标钉版为 spec 的 `v3.0.0-beta.9`；若 `mise install` 拉不到 beta.9，退回当前已装的 beta.8 并记录在计划文档里（版本升级本就是独立的显式提交，不阻塞本任务）。

- [ ] **Step 2: 在 mise.toml 补工具项**

把 `[tools]` 段改为：

```toml
[tools]
go = "1.26.3"
node = "22"
"go:github.com/wailsapp/wails/v3/cmd/wails3" = "v3.0.0-beta.9"
```

若 Step 1 决定钉 beta.8，则此处的版本号改为 `v3.0.0-beta.8`。

- [ ] **Step 3: 补 mise 任务**

在 `[tasks.*]` 段追加：

```toml
[tasks.dev]
run = "wails3 dev"

[tasks."build:linux"]
run = "wails3 build -platform linux/amd64"

[tasks."build:win"]
run = "wails3 build -platform windows/amd64"

[tasks."build:mac"]
run = "wails3 build -platform darwin/universal"
```

- [ ] **Step 4: 安装并核验工具**

运行 `mise install`，然后逐一核验：`node --version`（应 ≥ 22）、`wails3 version`（应等于钉版）、`mpv --version`（存在则通过）、`pkg-config --exists webkit2gtk-4.1 && echo yes`。

**系统依赖说明（写进本任务报告，不在此命令化）**：Wails v3 的 Linux 构建需要 webkit2gtk（4.1 或 4.0，取决于 Wails 版本）与 mpv。若 `pkg-config` 与 `mpv` 缺失，需先通过发行版包管理器安装（Ubuntu/Debian 为 `libwebkit2gtk-4.1-dev`、`mpv`），这属于宿主机层面的前置条件，记入报告即可，不由本任务脚本自动执行。

- [ ] **Step 5: 提交**

```bash
git add mise.toml
git commit -m "chore: mise 补齐 node 与 wails3 工具项及构建任务"
```

---

### Task 2: Player 接口与媒体类型

**Files:**
- Create: `internal/player/player.go`
- Create: `internal/player/player_test.go`

**Interfaces:**
- Consumes: 无（纯类型定义，是本计划的根）。
- Produces: `Player` 接口、`Stream`/`StreamKind`/`SubtitleTrack`/`TrackKind`/`State`/`Event` 类型。Task 3、5、6 依赖这些签名的**确切拼写**。

- [ ] **Step 1: 写失败测试**

创建 `internal/player/player_test.go`：

```go
package player

import "testing"

func TestStreamKindString(t *testing.T) {
	cases := map[StreamKind]string{
		StreamHLS:  "hls",
		StreamMP4:  "mp4",
		StreamFLV:  "flv",
		StreamRTMP: "rtmp",
		StreamLocal: "local",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("StreamKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestTrackKindString(t *testing.T) {
	cases := map[TrackKind]string{
		TrackAudio:    "audio",
		TrackSubtitle: "subtitle",
		TrackVideo:    "video",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("TrackKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestStateZeroValueIsStopped(t *testing.T) {
	var s State
	if s.Playing != StateStopped {
		t.Errorf("零值 State.Playing = %v, want %v", s.Playing, StateStopped)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/player/ -count=1`
Expected: FAIL（`undefined: StreamKind` 等）。

- [ ] **Step 3: 写实现**

创建 `internal/player/player.go`：

```go
// Package player 定义 Unbox 的播放层抽象与媒体类型。
//
// 本包不依赖 Wails，也不依赖任何具体播放后端；UI 层与 Provider 层只面对
// Player 接口，更换播放实现（mpvproc ↔ mpvlib）不触动它们。
package player

import "context"

// StreamKind 是播放流的容器/传输形态。
type StreamKind int

const (
	StreamHLS StreamKind = iota
	StreamMP4
	StreamFLV
	StreamRTMP
	StreamLocal // 本地文件或本地代理地址
)

func (k StreamKind) String() string {
	switch k {
	case StreamHLS:
		return "hls"
	case StreamMP4:
		return "mp4"
	case StreamFLV:
		return "flv"
	case StreamRTMP:
		return "rtmp"
	case StreamLocal:
		return "local"
	default:
		return "unknown"
	}
}

// TrackKind 是可被 SelectTrack 选中的轨道类型。
type TrackKind int

const (
	TrackAudio TrackKind = iota
	TrackSubtitle
	TrackVideo
)

func (k TrackKind) String() string {
	switch k {
	case TrackAudio:
		return "audio"
	case TrackSubtitle:
		return "subtitle"
	case TrackVideo:
		return "video"
	default:
		return "unknown"
	}
}

// SubtitleTrack 是一条外挂字幕轨。
type SubtitleTrack struct {
	URL     string
	Lang    string
	Default bool
}

// Stream 是播放一条媒体所需的一切信息。
type Stream struct {
	URL      string
	Headers  map[string]string // Referer / UA / Cookie 等
	Kind     StreamKind
	Subtitle []SubtitleTrack
	Backups  []string // 同频道备用流，供测速切换使用
}

// PlayState 是播放状态机的基本态。
type PlayState int

const (
	StateStopped PlayState = iota
	StatePlaying
	StatePaused
	StateBuffering
)

// State 是播放器当前的完整可观测状态。
type State struct {
	Playing  PlayState
	Position float64 // 秒
	Duration float64 // 秒；未知时为 -1
	Volume   int     // 0–100
}

// EventKind 是播放器通过 Events() 上报的事件类型。
type EventKind int

const (
	EventPosition EventKind = iota // 周期性位置更新，携带 Position
	EventBuffering                 // 缓冲中
	EventError                     // 播放出错，Err 非空
	EventEOF                       // 播放自然结束
)

// Event 是播放器上报的异步事件。
type Event struct {
	Kind     EventKind
	Position float64
	Err      error
}

// Player 是所有播放实现的统一接口。
type Player interface {
	Load(ctx context.Context, s Stream) error
	Play() error
	Pause() error
	Seek(sec float64) error
	SetVolume(v int) error
	SelectTrack(kind TrackKind, id int) error
	State() State
	Events() <-chan Event
	Close() error
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/player/ -count=1`
Expected: PASS（3 个测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/player/player.go internal/player/player_test.go
git commit -m "feat(player): Player 接口与媒体类型定义"
```

---

### Task 3: mpvproc 播放实现（Linux/Windows）

**Files:**
- Create: `internal/player/mpvproc/ipc.go`（JSON IPC 编解码，纯函数，可测）
- Create: `internal/player/mpvproc/ipc_test.go`
- Create: `internal/player/mpvproc/mpvproc.go`（进程启动 + 接口实现）
- Create: `internal/player/mpvproc/mpvproc_test.go`（进程层集成测试，mpv 缺失时跳过）

**Interfaces:**
- Consumes: Task 2 的 `player.Stream`/`player.Player` 等签名（确切拼写以 Task 2 为准）。
- Produces: `mpvproc.New(exePath string) (player.Player, error)`。Task 5 消费它来实例化播放器。

- [ ] **Step 1: 写 IPC 编解码的失败测试**

创建 `internal/player/mpvproc/ipc_test.go`：

```go
package mpvproc

import (
	"encoding/json"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestEncodeCommand(t *testing.T) {
	got := encodeCommand([]any{"loadfile", "/tmp/a.m3u8", "replace"})
	want := `{"command":["loadfile","/tmp/a.m3u8","replace"]}` + "\n"
	if got != want {
		t.Fatalf("encodeCommand = %q, want %q", got, want)
	}
}

func TestEncodeSetProperty(t *testing.T) {
	got := encodeCommand([]any{"set_property", "volume", 80})
	// volume 是数值，必须原样编码，不能被引号包成字符串
	var probe struct {
		Command []json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal([]byte(got), &probe); err != nil {
		t.Fatalf("encodeCommand 产出非法 JSON: %v", err)
	}
	if len(probe.Command) != 3 {
		t.Fatalf("command 长度 = %d, want 3", len(probe.Command))
	}
	if string(probe.Command[2]) != "80" {
		t.Fatalf("volume 被编码为 %s, want 80（数值）", probe.Command[2])
	}
}

func TestParseEvent(t *testing.T) {
	// mpv 的位置观察者上报形如 {"event":"property-change","name":"time-pos","data":12.5}
	evt, ok := parseEvent([]byte(`{"event":"property-change","name":"time-pos","data":12.5}`))
	if !ok || evt.Kind != player.EventPosition || evt.Position != 12.5 {
		t.Fatalf("parseEvent = (%+v,%v), want position 12.5", evt, ok)
	}

	// EOF 事件形如 {"event":"end-file","reason":"eof"}
	evt, ok = parseEvent([]byte(`{"event":"end-file","reason":"eof"}`))
	if !ok || evt.Kind != player.EventEOF {
		t.Fatalf("parseEvent = (%+v,%v), want EOF", evt, ok)
	}

	// 非目标事件应返回 ok=false 而不是误报
	if _, ok := parseEvent([]byte(`{"event":"idle"}`)); ok {
		t.Fatal("idle 事件不应被当作位置/EOF 事件")
	}
}
```

（`encodeCommand` 与 `parseEvent` 返回的 `player.Event` 需 import Task 2 的包：`github.com/unbox/unbox/internal/player`。）

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/player/mpvproc/ -count=1`
Expected: FAIL（`undefined: encodeCommand` 等）。

- [ ] **Step 3: 写 IPC 编解码实现**

创建 `internal/player/mpvproc/ipc.go`：

```go
package mpvproc

import (
	"encoding/json"
	"strconv"

	"github.com/unbox/unbox/internal/player"
)

// encodeCommand 把一条 mpv JSON IPC 命令编码为以换行结尾的请求行。
func encodeCommand(args []any) string {
	b, err := json.Marshal(map[string]any{"command": args})
	if err != nil {
		// args 均为字符串与数值，Marshal 不会失败；此处防御性兜底
		return "{}" + "\n"
	}
	return string(b) + "\n"
}

// parseEvent 解析 mpv 上报的事件行，只返回播放器关心的位置/缓冲/EOF 事件。
// 其余事件（idle、pause、start-file 等）返回 ok=false。
func parseEvent(line []byte) (player.Event, bool) {
	var raw struct {
		Event  string          `json:"event"`
		Name   string          `json:"name"`
		Data   json.RawMessage `json:"data"`
		Reason string          `json:"reason"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return player.Event{}, false
	}
	switch {
	case raw.Event == "end-file" && raw.Reason == "eof":
		return player.Event{Kind: player.EventEOF}, true
	case raw.Event == "property-change" && raw.Name == "time-pos":
		f, err := strconv.ParseFloat(string(raw.Data), 64)
		if err != nil {
			return player.Event{}, false
		}
		return player.Event{Kind: player.EventPosition, Position: f}, true
	}
	return player.Event{}, false
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/player/mpvproc/ -count=1`
Expected: PASS。

- [ ] **Step 5: 写进程层实现**

创建 `internal/player/mpvproc/mpvproc.go`：

```go
// Package mpvproc 通过 mpv 子进程 + JSON IPC 实现 player.Player。
//
// 用于 Windows（--wid=<HWND> 嵌入）与 Linux（--wid=<X11 Window> 嵌入）。
// macOS 不用本包，见 ../mpvlib。
package mpvproc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"

	"github.com/unbox/unbox/internal/player"
)

type mpvProc struct {
	exePath string
	ipcPath string // --input-ipc-server 暴露的 Unix socket 路径

	cmd *exec.Cmd
	conn net.Conn

	mu    sync.Mutex
	state player.State

	events chan player.Event
}

// New 以指定 mpv 可执行文件启动一个播放器实例。
//
// 实际把视频嵌入窗口需要 --wid=<窗口句柄>，但那是 shell 层的事；本层只
// 负责 mpv 进程生命周期与 JSON IPC 对话，窗口句柄由 shell 通过后续扩展
// 参数传入（M1 阶段先用 --force-window 保证无嵌入也能独立开窗冒烟）。
func New(exePath string) (player.Player, error) {
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("mpv 可执行文件不可用: %w", err)
	}
	p := &mpvProc{
		exePath: exePath,
		state:   player.State{Playing: player.StateStopped, Duration: -1},
		events:  make(chan player.Event, 64),
	}
	return p, nil
}

func (p *mpvProc) Load(ctx context.Context, s player.Stream) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		_ = p.closeLocked()
	}
	sock, err := os.CreateTemp("", "unbox-mpv-*.sock")
	if err != nil {
		return err
	}
	ipcPath := sock.Name()
	_ = sock.Close()
	_ = os.Remove(ipcPath)

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

	p.cmd = exec.CommandContext(ctx, p.exePath, args...)
	p.cmd.Stdout = io.Discard
	p.cmd.Stderr = io.Discard
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("启动 mpv 失败: %w", err)
	}
	if err := p.dialIPC(ipcPath); err != nil {
		return fmt.Errorf("连接 mpv IPC 失败: %w", err)
	}
	p.ipcPath = ipcPath
	p.state = player.State{Playing: player.StatePlaying, Duration: -1, Volume: 80}
	go p.readEvents()
	return nil
}

func (p *mpvProc) Play() error    { return p.send("set", "pause", false) }
func (p *mpvProc) Pause() error   { return p.send("set", "pause", true) }
func (p *mpvProc) Seek(sec float64) error {
	return p.send("seek", sec, "absolute")
}
func (p *mpvProc) SetVolume(v int) error {
	p.mu.Lock()
	p.state.Volume = v
	p.mu.Unlock()
	return p.send("set", "volume", v)
}
func (p *mpvProc) SelectTrack(kind player.TrackKind, id int) error {
	return errors.New("mpvproc: SelectTrack 未实现")
}
func (p *mpvProc) State() player.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}
func (p *mpvProc) Events() <-chan player.Event { return p.events }

func (p *mpvProc) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeLocked()
}

func (p *mpvProc) closeLocked() error {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	if p.ipcPath != "" {
		_ = os.Remove(p.ipcPath)
		p.ipcPath = ""
	}
	return nil
}

// send 发送一条 mpv JSON IPC 命令并等待成功应答。
func (p *mpvProc) send(args ...any) error {
	if p.conn == nil {
		return errors.New("mpvproc: 尚未 Load")
	}
	if _, err := p.conn.Write([]byte(encodeCommand(args))); err != nil {
		return err
	}
	return p.readResponse()
}

func (p *mpvProc) readResponse() error {
	br := bufio.NewReader(p.conn)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(line, &resp) // 命令应答里 error 为空即成功
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (p *mpvProc) dialIPC(path string) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	p.conn = conn
	return p.readResponse() // 消费 mpv 启动后的首条握手/欢迎行
}

func (p *mpvProc) readEvents() {
	br := bufio.NewReader(p.conn)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		if evt, ok := parseEvent(line); ok {
			p.mu.Lock()
			if evt.Kind == player.EventPosition {
				p.state.Position = evt.Position
			}
			p.mu.Unlock()
			select {
			case p.events <- evt:
			default: // 事件通道满则丢弃，避免阻塞读循环
			}
		}
	}
}
```

- [ ] **Step 6: 写进程层集成测试（mpv 缺失时跳过）**

创建 `internal/player/mpvproc/mpvproc_test.go`：

```go
package mpvproc

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

func TestLoadPlayClose(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("本机无 mpv，跳过集成测试")
	}
	p, err := New(mpv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 用本地生成的静音短视频？M1 不引入 ffmpeg，这里用一个不会立即 EOF 的
	// 短循环流不好造，退而求其次：验证 Load 能启动 mpv 并建立 IPC 即可，
	// 播放本身由 Task 5 的冒烟在真实环境人工确认。
	if err := p.Load(ctx, player.Stream{URL: "https://example.com/none.m3u8"}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st := p.State(); st.Playing != player.StatePlaying {
		t.Fatalf("Load 后 Playing = %v, want playing", st.Playing)
	}
	if err := p.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
}
```

（说明：这个集成测试只验证「mpv 能启动、IPC 能连通、命令有应答」，URL 拉流失败与否不属于本测试断言的范畴——`Load` 只要把进程和 IPC 立起来就算成功，真正的拉流结果在 Task 5 冒烟里人工看画面确认。）

- [ ] **Step 7: 跑全部测试**

Run: `go test ./internal/player/... -count=1`
Expected: PASS（无 mpv 时 mpvproc 集成测试显示 SKIP，其余 PASS）。

- [ ] **Step 8: 提交**

```bash
git add internal/player/mpvproc/
git commit -m "feat(player): mpvproc 子进程 + JSON IPC 播放实现"
```

---

### Task 4: Wails v3 壳 + Vue3 前端脚手架

**Files:**
- Create: `cmd/unbox/main.go`
- Create: `internal/shell/app.go`（应用/窗口生命周期）
- Create: `frontend/`（Vue 3 + TS + Tailwind 脚手架，含一个最小页面）
- Modify: `.gitignore`（如需忽略前端构建产物与 Wails 生成目录）

**Interfaces:**
- Consumes: Task 1 就绪的 `wails3` 工具链与 node。
- Produces: 可被 `wails3 build` 编译为 `unbox` 的完整壳；`internal/shell` 不 import 业务层。

- [ ] **Step 1: 用官方脚手架生成骨架**

```bash
# 在仓库根下，用 wails3 官方模板初始化到临时目录，再把需要的文件并入
wails3 init -n unbox -t vue-ts -d /tmp/unbox-scaffold 2>&1 || \
  wails3 init -n unbox -t vanilla -d /tmp/unbox-scaffold
```

Expected: `/tmp/unbox-scaffold` 下出现可编译的 Wails v3 项目（`main.go`、`build/`、前端目录等）。

**说明**：Wails v3 beta 的 `init` 模板名称与参数可能随版本变动（`vue-ts`/`vanilla`/`svelte` 等），以实际 `wails3 init --help` 输出为准。本步的目标是拿到一个「当前钉版下能编译」的官方骨架，避免手写 beta API 抄错。

- [ ] **Step 2: 把骨架并入本仓库的目录结构**

按 spec §3.2 的布局，把骨架的入口拆到 `cmd/unbox/main.go`，Wails 相关 glue 收敛进 `internal/shell/`，前端进 `frontend/`。以 `wails3 init` 生成的实际文件为准做等价迁移；迁移后 `main.go` 只保留「解析配置 → 创建 app → 开窗 → Run」，窗口/应用选项细节放 `internal/shell/app.go`。

- [ ] **Step 3: 前端最小页面**

`frontend/` 下保留脚手架自带的最小页面（一个标题 + 一个「当前平台 / 播放器就绪」占位），确保 `npm install` + `npm run build`（或 wails3 内嵌的前端构建）能产出前端资源。

- [ ] **Step 4: 编译校验**

Run: `mise run build:linux`
Expected: 产出 `unbox` 可执行文件，无编译错误。

**注意**：若此步因 webkit2gtk 缺失而失败，说明 Task 1 的系统依赖未就位——先装 `libwebkit2gtk-4.1-dev`（或 Wails 当前钉版要求的版本）再重试，装包命令记入本任务报告。

- [ ] **Step 5: 提交**

```bash
git add cmd/unbox internal/shell frontend .gitignore
git commit -m "feat(shell): Wails v3 应用壳与 Vue3 前端脚手架"
```

---

### Task 5: 壳与 Player 接线 + 冒烟

**Files:**
- Modify: `internal/shell/app.go`（注入 player，暴露「加载测试流」入口）
- Modify: `cmd/unbox/main.go`（创建 mpvproc 播放器并传入 shell）

**Interfaces:**
- Consumes: Task 3 的 `mpvproc.New(exePath string) (player.Player, error)`；Task 4 的 shell 结构。
- Produces: 一个「启动 → 加载一条流 → mpv 出画面」的最小闭环。

- [ ] **Step 1: 写 shell 层可测的纯逻辑**

在 `internal/shell/` 下新增一个不依赖 Wails 的纯函数，把「根据当前平台选择播放实现 + 解析 mpv 可执行文件路径」抽出来单测：

```go
// pickPlayer 返回当前平台应使用的播放器实例；不 import Wails，可被 go test 直接测。
func pickPlayer() (player.Player, error) {
	if runtime.GOOS == "darwin" {
		return mpvlib.New()
	}
	exe, err := exec.LookPath("mpv")
	if err != nil {
		return nil, fmt.Errorf("未找到 mpv 可执行文件: %w", err)
	}
	return mpvproc.New(exe)
}
```

Run: `go test ./internal/shell/ -count=1`
Expected: PASS（在 Linux 上返回 `mpvproc` 实例；`mpv` 缺失时返回明确错误而非 panic）。

- [ ] **Step 2: 在 shell 里接线**

`main.go` 启动时调用 `pickPlayer()`，把得到的 `player.Player` 实例交给 `internal/shell` 的应用对象；应用对象持有该接口，供后续 UI 调用（M1 阶段先接一个硬编码的「加载示例流」入口）。

- [ ] **Step 3: 冒烟验证**

Run: `mise run build:linux && ./bin/unbox`（或 `wails3 dev`）

Expected（人工/截图确认）：窗口打开，触发「加载示例流」后 mpv 进程启动并显示画面（用一条公开的测试 HLS/MP4 流即可）。无头环境（无 DISPLAY）下本步改为「进程能启动到开窗失败前不崩溃，日志给出明确原因」，并记录到报告。

- [ ] **Step 4: 提交**

```bash
git add internal/shell cmd/unbox
git commit -m "feat(shell): 壳层接入 mpvproc 播放器，打通加载-播放最小闭环"
```

---

### Task 6: mpvlib（macOS）骨架

**Files:**
- Create: `internal/player/mpvlib/mpvlib_darwin.go`（`//go:build darwin`）
- Create: `internal/player/mpvlib/mpvlib_stub.go`（`//go:build !darwin`，仅声明包，保证非 macOS 平台包存在）

**Interfaces:**
- Consumes: Task 2 的 `player.Player` 接口。
- Produces: `mpvlib.New() (player.Player, error)`（darwin 上为真实 libmpv 实现的占位，M1 阶段只要求「能在 macOS 编译通过」）。

- [ ] **Step 1: 写非 macOS 的占位文件**

创建 `internal/player/mpvlib/mpvlib_stub.go`：

```go
//go:build !darwin

// Package mpvlib 是 macOS 上基于 libmpv + CAMetalLayer 的播放实现。
// 非 macOS 平台只保留包占位，真实实现见 mpvlib_darwin.go。
package mpvlib
```

- [ ] **Step 2: 写 macOS 骨架**

创建 `internal/player/mpvlib/mpvlib_darwin.go`：

```go
//go:build darwin

// Package mpvlib 是 macOS 上基于 libmpv + CAMetalLayer 的播放实现（spec §3.4）。
//
// M1 阶段本文件只给出结构与 build tag，确保包能在 macOS 上编译通过；
// 真正的 libmpv cgo 绑定与 CAMetalLayer 分层渲染在拿到 macOS 构建机后补齐。
package mpvlib

import (
	"context"
	"errors"

	"github.com/unbox/unbox/internal/player"
)

type libmpvPlayer struct{}

// New 返回 macOS 的 libmpv 播放器。
func New() (player.Player, error) {
	return &libmpvPlayer{}, nil
}

func (p *libmpvPlayer) Load(ctx context.Context, s player.Stream) error {
	return errors.New("mpvlib: 尚未实现（M1 macOS 构建机就绪后补齐）")
}
func (p *libmpvPlayer) Play() error    { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) Pause() error   { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) Seek(sec float64) error {
	return errors.New("mpvlib: 未实现")
}
func (p *libmpvPlayer) SetVolume(v int) error { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) SelectTrack(kind player.TrackKind, id int) error {
	return errors.New("mpvlib: 未实现")
}
func (p *libmpvPlayer) State() player.State {
	return player.State{Playing: player.StateStopped, Duration: -1}
}
func (p *libmpvPlayer) Events() <-chan player.Event { return nil }
func (p *libmpvPlayer) Close() error                { return nil }
```

- [ ] **Step 3: 校验 Linux 构建不受影响**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: 全绿（`mpvlib_stub.go` 在 Linux 上生效，`mpvlib_darwin.go` 被 build tag 排除）。

- [ ] **Step 4: 交叉校验 macOS 可编译（可选，需 macOS 环境或 CI）**

Run: `GOOS=darwin GOARCH=arm64 go build ./internal/player/mpvlib/`
Expected: 若本机 Go 无法因 cgo 交叉编译 libmpv 而失败，属预期（spec §5.2 交叉编译不可行），记录即可；真实校验留给 macOS CI。

- [ ] **Step 5: 提交**

```bash
git add internal/player/mpvlib/
git commit -m "feat(player): mpvlib macOS 骨架（build tag 隔离，M1 仅保证可编译）"
```

---

## Self-Review

**Spec 覆盖**：§3.1 Player 接口 → Task 2；§3.4 播放层按平台分叉（mpvproc Win/Linux、mpvlib macOS）→ Task 3/6；§5.1 mise 工具链 → Task 1；目录结构 §3.2 的 `cmd/unbox`/`internal/shell`/`internal/player`/`frontend` → Task 4/5。§7 验收标准里「`unbox` 主程序编译通过」在本计划以 Linux 编译为准，「三平台安装包产出 / 点击出画面 / 自动切换」属 Plan 3 及之后。

**占位符扫描**：无 TBD/TODO。Task 4 Step 1 的模板名以 `wails3 init --help` 实际输出为准属「命令自查」而非占位。

**类型一致性**：Task 2 定义了 `player.Stream`/`player.Player`/`State`/`Event` 等确切拼写，Task 3/5/6 全部引用同一拼写；`mpvproc.New` 的返回签名在 Task 3 定义、Task 5 消费，一致。

**明确不做（本轮）**：Provider 接口与 live 源、M3U 导入、测速/失败切换、SQLite 持久化、完整前端、mpv 随包分发、macOS libmpv 真实绑定——均属 Plan 3 或之后。
