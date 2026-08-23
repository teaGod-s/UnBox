# AGENTS.md — Unbox 项目指引

本文件供 AI 编码代理（Codex / Claude / 等）接手项目时快速建立上下文。
改动项目前先读这里和 `docs/HANDOFF.md`。

## 项目概述

Unbox 是一个跨平台（Windows / macOS / Linux）桌面媒体播放器，完全兼容 TVBox，
目标「安装即用、无手动依赖」。依赖由 [mise](https://mise.jdx.dev) 管理。

技术栈：Go 1.26.3 + Wails v3（**3.0.0-beta.9 钉死**）+ Vue3 + TypeScript。

## 常用命令

工具版本统一在 `mise.toml`，所有任务经 `mise run`：

```bash
mise run test          # go test ./...
mise run build:linux   # wails3 build GOOS=linux GOARCH=amd64
mise run build:win     # wails3 build GOOS=windows GOARCH=amd64
mise run build:mac     # wails3 build GOOS=darwin GOARCH=arm64
mise run dev           # wails3 dev（前端热重载）
mise run scan          # go run ./cmd/unbox-scan
```

**交叉编译不可行**（cgo：Wails + libmpv）。三个平台必须各自**原生**编译：
Windows 构建只能在 Windows 宿主机做，不能在 WSL 交叉编译。

前端产物 `frontend/dist` 是 gitignore 的构建产物；production 构建由
`assets.go` 的 `//go:embed all:frontend/dist` 嵌入二进制，`assets_dev.go`
在非 production 构建提供空容器（dev 走 Vite）。

## 架构（internal/ 包）

- `internal/config` — 配置解析。
- `internal/provider` — 内容来源抽象；`internal/provider/live` — M3U/TXT 解析。
- `internal/probe` — URL 探测 / 测速排序。
- `internal/player` — 播放器接口（`Player` 9 方法、`Embedder`）；
  - `mpvproc` — mpv 子进程 + JSON IPC（Linux/Windows，`--wid` 嵌入）；
  - `mpvlib` — macOS libmpv（CAMetalLayer 分层渲染）；
  - `failover` — 单事件循环的故障切换包装。
- `internal/shell` — 全部 Wails glue（app / 窗口 / 服务 / 原生句柄提取）。
- `internal/store` — SQLite 持久化（`modernc.org/sqlite`，纯 Go 无 cgo）。
- `cmd/unbox` — 主程序入口；`cmd/unbox-scan` — 扫描 CLI。
- `frontend` — Vue3 + TS UI（组件 `App.vue`，绑定经 `frontend/bindings` 生成）。

## 约束与约定

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；
  `internal/player/` 等业务层不得 import Wails。
- 公开错误信息 / 注释用中文。
- TDD：改代码先写失败测试。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；
  Linux 额外 `CGO_ENABLED=1 go build ./...`。

## 关键坑（已踩过，勿重蹈）

- **Linux/WSLg 必须强制 `GDK_BACKEND=x11`**：GTK4 默认优先 Wayland，WSLg 下
  Wayland 会让 WebKit 窗口不渲染、`--wid` 拿不到 XID。已在
  `internal/shell/app.go` 的 `forceLinuxX11Backend()` 处理（GTK 初始化前）。
- **WSLg GPU 直通**：`/dev/dri` 必须存在，否则 WebKitGTK 无 GPU 时渲染全黑。
  这是环境问题而非代码问题，详见 `docs/HANDOFF.md`。
- **GTK 非线程安全**：`internal/shell/embed_linux.go` 取 XID 必须经 Wails 的
  `application.InvokeSync` 派发到主线程；`g_main_context_invoke(NULL,...)` 在
  主循环启动前会就地执行回调（跨线程调 GTK），导致 `g_application_run` 段错误。
- **mpv JSON IPC**：`set` 命令拒绝 bool/数字（用 `set_property`）；终端事件
  （EOF/Error）须阻塞发送。
