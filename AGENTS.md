# AGENTS.md — UnBox 项目指引

本文件供 AI 编码代理（Codex / Claude / 等）接手项目时快速建立上下文。
改动项目前先读这里和 `docs/HANDOFF.md`。

## 项目概述

UnBox 是一个跨平台（Windows / macOS / Linux）桌面媒体播放器，完全兼容 TVBox，
目标「安装即用、无手动依赖」。依赖由 [mise](https://mise.jdx.dev) 管理。

技术栈：Go 1.27.3 + Wails v3（**3.0.0 稳定版**）+ Vue3 + TypeScript。

JAR（`csp_`）兼容的调研与 M6 规划见
`docs/superpowers/specs/2026-10-17-unbox-jar-compat-design.md`；结论是
「不执行 dex，走 Go 规则引擎」。**M6.1 `csp_XBPQ` 与 M6.2 `csp_XYQHiker` 均已实现**
（纯 Go 规则引擎，`internal/provider/tvbox/rule` + `tvbox.JarProvider`）；M6.2 支持
JSoup 链式选择器子集（`&&` 链、`,N` 索引、`:matches`/`:not(:matches)`/`:has`、
`||` 备选、`!` 去前缀）与简单 XPath 子集，字段含 JSONC `//` 注释容错、
`[firstPage=`/`[firstPg=` 翻页语法、`;post`/`;get` 方法后缀、JSON 搜索响应。验收见
`docs/2026-10-17-jar-acceptance.md`（M6.2 三个真实站点端到端：玩偶哥哥 / 河马短剧 /
去看吧动漫）。drpy2 / ES Module 的调研与 M7 规划见
`docs/superpowers/specs/2026-10-17-unbox-js-esm-drpy2-design.md`；结论是
「drpy2 用 `ext`，ESM 用纯 Go 模块加载器」。**M7.1/M7.2 已实现**（选源 + ESM
加载器 + 官方 MIT cheerio / crypto-js bundle），最终 review 又修复了
`export async function`、嵌套 `__jsEvalReturn`、drpy2 引擎库识别与 ext query。
**M7.3 已补齐**：自有 `cat.js` shim（`Crypto`/`_`/`jinja2`）、`gbk.js` shim、
drpy2 `fyfilter`/`filter_url` 展开、`init(ext)`/`init(cfg)` 双形态、UA 常量与
`http`/`req` 相对路径解析。真实站点验收见 `docs/2026-10-17-m7-acceptance.md`；
**筛选 UI 已实现**（2026-10-19）：点播页有筛选项的分类显示「筛选」入口（抽屉/弹层），
drpy2 规则站（`filter_def`/`filter_url`，选择值经 `extend` 与 `{{fl.xxx}}` 透传）与
CMS（class `filter`/`filters`，`videolist` 的 `filter=<URL编码 JSON>`）都支持；
实现见 `internal/provider/{filters.go,tvbox/filters.go}`、`internal/crawler` 的
`VodCategoryExtend`、前端 `vodFilters.ts` + `VodFilterPanel.vue`，spec/plan 见
`docs/superpowers/specs/2026-10-19-unbox-vod-filter-ui-design.md`。剩余缺口是上游未声明
许可证的 `模板.js`。
网盘源的调研与
M8 规划见
`docs/superpowers/specs/2026-10-17-unbox-netdisk-design.md`；结论是
「先做 AList/WebDAV，云盘分享解析与 Wogg/Wobg 暂不做」。

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
- 检查更新：`service.go` 的 `latestReleaseByRedirect` 先抓
  `github.com/teaGod-s/UnBox/releases/latest` 并跟着 302 读 tag（网页入口不占 API 配额），
  失败才退回 `updateURL`（`api.github.com`）。**未登录的 GitHub API 每小时只有 60 次，
  被限流就是 403**，别把主路径改回 API。「关于」页当前版本用免联网的
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
  - `mpvplugin` — 外部 mpv 探测 + 安装兜底（Windows NSIS 安装包优先使用应用目录内嵌 mpv，随后检查用户插件目录和系统 PATH；Linux/macOS 仍弹安装命令）；
  - `failover` — 故障切换包装。
- `internal/playback` — 播放编排：Resolver（share 页解析）→ Controller（Web/mpv 路由）
  → Proxy（本地代理 + HLS 分片重写）。走 mpv 的只有四种情况（`needsMPV`）：**本地媒体库
  文件**、RTMP、HEVC 的 HLS、以及 Web 播放失败后的 `Fallback`。Windows/macOS 其余一律走
  Web，所以「Windows 上总碰到 mpv」通常意味着用户在媒体库看片、或在看 H.265 源。
- `internal/library` — 本地媒体库：递归扫描识别视频 + 片名/海报匹配，带 token 鉴权与防
  目录穿越的本地文件 HTTP 服务 + 进度门面（M3）。
- `internal/shell` — 全部 Wails glue（app / 窗口 / 服务）。全站搜索在
  `service.go` 的 `vodSearchAll`：站点级 `siteSearchTimeout`(12s)、并发度走
  `SearchThreads`（1/4/8/16，**内存权威值**，别改回每次读库）、吃满超时的站点进
  `searchDead` 名单（10 分钟，落盘到 store.kv，重启后仍生效）。
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
  （EOF/Error）须阻塞发送。**`exec.Cmd.Wait` 只允许调用一次**：Close / 命令应答
  超时 / Load 失败清理三处共用 `waiter`（`mpvproc/waiter.go`）收尸，别改回各自 Wait。
- **mpv 起不来时错误必须能定位**：四层配套，拿掉任何一层，用户就会退回
  「连接 mpv IPC 失败: 找不到管道」这种无从下手的报错。
  1. `shell.PickPlayer()`（`internal/shell/pick.go`）是**启动路径**，必须委托给
     `mpvplugin.Manager`：不要自己 `lookPath("mpv")`——那样既会反转「内嵌优先」
     的顺序（PATH 上残留的旧 mpv 会盖掉应用自带的），又会绕过预检。
  2. `mpvplugin.NewPlayer()` 先跑 `mpv --version` 预检（`version.go`）：跑不起来
     （缺 DLL / 被杀软拦截）或低于 `minMPVVersion` 都当场报错。它不在 `Status()` 里，
     因为后者被前端高频轮询。
  3. `mpvproc` 保留子进程 stderr 末尾 4KB（`stderr.go`）。
  4. 等 IPC 期间监视进程是否提前退出（`ipc.go` 的 `waitForIPC`），并把**退出码**
     带进报错（`exitcode.go`）——缺 DLL / 架构不匹配 / 崩溃都是零输出秒退，退出码
     是唯一能区分它们的证据。
- **mpv 在无 GPU 的 Linux（WSLg / 容器）必须退回软件输出**：这些环境里
  `/dev/dri/renderD*` 打不开（WSLg 是 root:render 0660、用户不在 render 组），
  EGL/Zink 起不来，mpv 回退 x11 vo 时二次初始化会 `vo_x11_init: Assertion` 直接 abort
  （表现为「命令应答超时」+ 换源全失败）。`mpvproc.videoOutputArgs()` 会按 render 节点
  可打开性自动带 `--vo=x11 --hwdec=no`；`UNBOX_MPV_SOFT=1/0` 可强制覆盖。
- **mpv 命令应答超时是 15s，且网络打开有 15s 上限**（`--network-timeout=15`）：
  前端不要拿 `Pause()` 这类 IPC 往返去阻塞页面切换（`stopPlayback` 已改成不等应答），
  否则主循环一忙就整页卡住。
- **drpy 规则的两个语义别丢**：列表/搜索的 id 字段写 `a+b` 时按 `"$"` 连接
  （`evalJSONIDExpr`），详情再用 `splitDetailID` 拆成 `fyclass` + 真实 id；
  规则脚本（`js:` 详情、`lazy`）必须在函数作用域执行（`runRuleScript` 的 IIFE 包装），
  否则第二次执行同一条脚本会撞 `Identifier 'x' has already been declared`。
- **平台播放页不等于可播放地址**：B站/腾讯/爱奇艺等页面由 `playback.platformPageName`
  提前识别并报明确原因；新增平台域名时改这里，别指望抓 HTML 能拿到流。
- **TypeScript 停在 6.x，不要装 7.x**：vue-tsc 3.x 仍按 `typescript/lib/tsc` 加载，
  TS 7（原生编译器）不再导出该子路径，装上直接 `ERR_PACKAGE_PATH_NOT_EXPORTED`。
  `tsconfig.json` 的 `moduleResolution` 用 `Bundler`（`Node`/node10 在 TS 6 起已废弃）。
- **`wails3 generate bindings` 默认输出 `.js` + JSDoc（beta.26 起），必须带 `-ts`**：
  漏掉时前端静默丢类型（model 塌成 `any`），`vue-tsc` 报 `TS18047`/`TS18046`，
  看着像代码回归实为命令问题。`build/Taskfile.yml` 的 `generate:bindings` 已带
  `-ts`（和 `-i`），CI 与 `mise run build:*` 安全；手敲这条命令时须自己加。
  `frontend/bindings/` 是 gitignore 的生成物，不入库。
