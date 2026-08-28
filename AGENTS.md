# AGENTS.md — UnBox 项目指引

本文件供 AI 编码代理（Codex / Claude / 等）接手项目时快速建立上下文。
改动项目前先读这里和 `docs/HANDOFF.md`。

## 项目概述

UnBox 是一个跨平台（Windows / macOS / Linux）桌面媒体播放器，完全兼容 TVBox，
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

**交叉编译不可行**（cgo：Wails 的 Linux WebKitGTK / macOS 后端）。三个平台必须
各自**原生**编译：Windows 构建只能在 Windows 宿主机做，不能在 WSL 交叉编译。

前端产物 `frontend/dist` 是 gitignore 的构建产物；production 构建由
`assets.go` 的 `//go:embed all:frontend/dist` 嵌入二进制，`assets_dev.go`
在非 production 构建提供空容器（dev 走 Vite）。

## 发布出包与版本

- 三平台原生出包走 GitHub Actions：`.github/workflows/release.yml`，push `v*` 标签
  自动原生编译 + 打包 + 创建 Release；手动 `workflow_dispatch` 只构建不上传 Release。
  goreleaser 已弃用（Wails v3 cgo 无法可靠交叉编译，必须各平台各自原生编译）。
- 版本注入：`internal/shell` 的 `appVersion` 是 `var`，发布构建经
  `-ldflags -X github.com/unbox/unbox/internal/shell.appVersion=<version>` 注入；根
  `Taskfile.yml` 的 `VERSION` 变量读 `VERSION` 环境变量（默认 0.0.1），CI 在 build 前
  把 tag 剥掉 `v` 前缀写入。nfpm 的 `version` 走 `${VERSION}`。
- 检查更新：`service.go` 的 `updateURL` 指向
  `api.github.com/repos/teaGod-s/UnBox/releases/latest`；「关于」页当前版本用免联网的
  `CurrentVersion()` 即时回显（不要用 `CheckUpdate()` 取当前版本，那会联网）。
- Logo 源文件是 `build/appicon.svg`（三面三色 + 眯眯眼笑脸），改动后重渲染
  `appicon.png` 并 `wails3 generate icons` 重生成 ico/icns；图标经 SVG 的
  `matrix()` 变换贴到顶面菱形，勿手改坐标。
- 打包元数据：产品名用 `UnBox`，公司名 / 厂商统一用 `RejectCode`（nfpm `vendor`、
  NSIS `INFO_COMPANYNAME`、`info.json` 的 `CompanyName`/`LegalCopyright`）。
- 捐助入口：前端 `App.vue` 顶部 `DONATE_URL` 是爱发电链接（`afdian.com/a/teaGod`），
  README 捐助栏目里也有一处，保持一致。

## 架构（internal/ 包）

- `internal/config` — 配置解析 + 多仓展开（`Resolver`，追踪线路名 `SourceName`）。
- `internal/provider` — 内容来源抽象；`internal/provider/live` — M3U/TXT；`internal/provider/tvbox` — CMS/Drpy。
- `internal/probe` — URL 探测 / 测速排序。
- `internal/player` — 播放器接口（`Player`）；
  - `mpvproc` — mpv 子进程 + JSON IPC（三平台统一，独立窗口）；
  - `mpvplugin` — 外部 mpv 探测 + 一键安装（Linux/macOS 弹命令，Windows 下载）；
  - `failover` — 故障切换包装。
- `internal/playback` — 播放编排：Resolver（share 页解析）→ Controller（Web/mpv 路由）
  → Proxy（本地代理 + HLS 分片重写）。
- `internal/shell` — 全部 Wails glue（app / 窗口 / 服务）。
- `internal/store` — SQLite 持久化（`modernc.org/sqlite`，纯 Go 无 cgo）。
- `cmd/unbox` — 主程序入口；`cmd/unbox-scan` — 扫描 CLI。
- `frontend` — Vue3 + TS UI（组件 `App.vue` / `PlaybackView.vue`，绑定经 `frontend/bindings` 生成）。

## 约束与约定

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；
  `internal/player/` 等业务层不得 import Wails。
- 公开错误信息 / 注释用中文。
- 主窗口最小尺寸 720×480，设在 `internal/shell/app.go` 的 `OpenWindow()`（`MinWidth`/`MinHeight`）。
- TDD：改代码先写失败测试。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；
  Linux 额外 `CGO_ENABLED=1 go build ./...`。

## 关键坑（已踩过，勿重蹈）

- **Linux/WSLg 必须强制 `GDK_BACKEND=x11`**：GTK4 默认优先 Wayland，WSLg 下
  Wayland 会让 WebKit 窗口不渲染。已在 `internal/shell/app.go` 的
  `forceLinuxX11Backend()` 处理（GTK 初始化前）。
- **WSLg 环境坑**（详见 `docs/HANDOFF.md`）：
  - `/mnt/shared_memory` 未挂载 → 窗口渲染空白（标题带 `[WARN:COPY MODE]`），挂载 tmpfs + `wsl --shutdown` 解决。
  - 裸 Ubuntu 无 emoji 字体 → 站点名里的 emoji 显示成方块（`sudo apt install fonts-noto-color-emoji`）。
  - 中文输入法无效（Windows IME 组合事件无法经 RDP→Weston→XWayland 转发到 WebKitGTK，microsoft/wslg 已知限制）。
- **WebKitGTK 无 MSE**：Linux 上 hls.js/mpegts.js 不可用，HLS/FLV/TS 只能走 mpv；
  路由逻辑在 `internal/playback/controller.go`（`SetWebMSE(false)`）。
- **mpv JSON IPC**：`set` 命令拒绝 bool/数字（用 `set_property`）；终端事件
  （EOF/Error）须阻塞发送。
