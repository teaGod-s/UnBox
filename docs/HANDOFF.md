# Handoff — 当前状态与待办（2026-09-07）

接手时先读 `AGENTS.md` 建立上下文，再读本文件了解进度与卡点。

## 总体进度

- **M1 已完成**：配置解析、M3U 导入、直播浏览/播放、测速/故障切换、收藏。
- **M2 已完成**：CMS JSON 点播（`internal/provider/tvbox` + 壳层多源 + 前端点播面板）。
- **M2.5 已完成**：JS 爬虫客户端模式（`tvbox.Drpy`，对 drpy2/drpyS 调 `/api/*`）。
- **M4 已完成**（已合入 master）：
  - 内置 Web 播放（hls.js / mpegts.js / 原生 `<video>` + Go 本地代理）+ 外部 mpv 插件。
  - 播放路由：H.264 HTTP → Web；HEVC / RTMP / 本地文件 / 无 MSE → mpv。
  - Linux WebKitGTK 无 MSE → HLS/FLV/TS 走 mpv，MP4 走原生 `<video>`。
  - share 页 URL 解析 + Go 代理（HMAC 签名 + HLS 分片重写）。
  - mpv 播放器：探测优先级为应用目录内嵌 mpv → 用户插件目录 → 系统 PATH；Linux/macOS 保留安装命令兜底。
  - 丢弃 mpvlib，三平台统一「Web + 外部 mpv」。

- **M5.1 已完成**（已合入 master）：
  - 内嵌 goja JS 引擎跑 FongMi js0 爬虫（`export default` 模块 + `req`/`pdfh`/`pdfa`/`pd` 原语）。
  - 方法名/签名对齐 FongMi 官方协议（`homeContent`/`categoryContent`/`searchContent`/`detailContent`/`playerContent`），
    同时兼容 dr_py 旧名（`home`/`category`/`search`/`detail`/`play`）。
  - `Spider` Provider 集成 `.js` 站点（classify `js` → Spider）。
  - 实测结论：`csp_` JAR 是编译 dex（非 JS），本地不可行 → M5.2 搁置（见 spec §11）。

- **M5.3 dr_py 方言适配已完成**（已合入 master，merge commit `4aa0367a`）：
  - 支持 `var rule` 的 `class_parse`、`url`/`searchUrl` 占位、`muban` 覆盖、`json:`/`js:` 内联规则、`lazy` 和 GBK 解码。
  - 保留 M5.1 FongMi `export default` 动作分发路径，并补齐真实 dr_py 常用的 `fetch`/`request`/`fetch_params`、`buildUrl`、`urlDeal`、`print` 语义。
  - 公开 `hjdhnx/dr_py` 的 `360影视.js` 真实验收通过：分类 4 个、一级列表 35 条、搜索“重器” 8 条、详情与播放地址解析成功。
  - 代码提交：`c54c23cd`、`30b3cd27`、`69119f3e`、`83bf134d`、`641a0df6`、`94922abd`、`386a7774`、`1be06d6b`、`24c6315b`；真实源校准为 `6274940e`、`77c5d8f3`、`832903e0`。
  - 最终验证：`go test ./... -count=1`、`go vet ./...`、`CGO_ENABLED=1 go build ./...`、`gofmt` 全部通过。

- **M3 本地媒体库已完成**（基础实现 merge `adcc8f3e`，首帧海报与布局修复已在 `f2b7031e`，
  2026-09-07）：
  `internal/library`（递归扫描 + 片名/海报匹配 + 带 token 鉴权与防穿越的本地 HTTP
  服务 + 进度门面）、`internal/store` 目录/条目表、`internal/shell` 绑定方法、前端
  媒体库 tab（扫描 → 浏览 → 播放，切页停止，断点续播）。无海报条目现在按 `(path, mtime)`
  缓存首帧：WebView canvas 优先，mpv 图片输出兜底；生成的本地 URL 只在运行时回填首页，
  不写入数据库。详见下方「近期更新」。

- **点播播放与导航修复已完成**（已合入 master，merge `dfb8777a`，2026-09-04）：
  - 直播/点播播放器计划与页面归属隔离，切页不会显示另一页面的画面或写入点播进度。
  - 详情页折叠简介后保留原生播放控件；集数每页 36 集，分页支持两侧箭头滚动和等宽网格。
  - 类目、线路、站点选择器仅在点播列表页显示；详情返回支持首页、搜索结果和原类目列表，搜索结果缓存 5 分钟。
  - 提交：`0e4102fa`、`e1e5c9ae`、`fd33add5`、`18d9e79a`、`8f017481`、`3967f3a8`、`cd2a594a`、`a1842bcd`、`2ebb077e`。
  - 后续并发修复：播放准备与 fallback 使用前后端双向 token，后端串行化 `Load+Play` 并拒绝旧请求；停止播放会使在途请求失效，避免旧请求暂停或覆盖新播放。全站搜索事件携带 `ID/Query`，取消、重复搜索和返回操作会丢弃旧结果。
  - 验证：前端 13 项测试、生产构建、`go test ./... -count=1`、`go vet ./...`、`CGO_ENABLED=1 go build ./...` 通过。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（M1）、
`docs/superpowers/specs/2026-08-24-unbox-m2-design.md`（M2）、
`docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`（M4）、
`docs/superpowers/specs/2026-08-29-unbox-m5-js-engine-design.md`（M5）。

## 近期更新（2026-09-01 之后）

> 09-01 快照之后合入 master 的更新。下方「M4 之后新增的功能（本次会话）」为当时的
> 冻结快照，保留作历史记录，不再更新。

- **Web 播放器轨道控制**（`6c8c8d9b`，2026-09-10/11）：HLS Web 播放路径新增轨道设置菜单，
  可选择清晰度、音轨和 HLS 内置字幕轨；菜单仅在 hls.js 可用时显示，mpv/FLV/原生 MP4
  路径不显示。外挂 SRT/VTT 加载入口当前隐藏，字幕转换工具保留在前端供后续复用。
- **mpv 独立窗口控件**（`2026-09-11`）：mpv 仍以独立窗口播放，但默认开启 mpv OSC，
  鼠标移入 mpv 窗口即可显示播放/暂停、进度、音量等原生控件；UnBox 主窗口不再重复
  展示暂停、继续和音量控制，保留选集、换源、收藏和播放记录等业务操作。

- **播放设置与点播自动化**（`2026-09-12`，分支 `feat/playback-settings`，尚未合入 master）：
  设置页新增「播放设置」分类，三个开关默认关闭，KV 键为 `playback.autoNext`、
  `playback.autoSwitchSource`、`playback.preloadNext`（缺失/非法值按关闭处理）。
  - **自动切集**：只取当前线路剧集数组里的下一集，没有下一集就停下，不跨线路找。
  - **自动换源**：明确错误立即触发，未起播/持续缓冲 30 秒触发；按详情页线路顺序尝试，
    只接受 `Name.trim()` 完全同名的剧集，找不到就跳过该线路，每条其他线路最多一次；
    切换后沿用当前进度续播。手动切换线路与自动换源走同一条流程。
  - **预载下一集**：Web 用独立隐藏 `<video>` + 独立代理会话（不占用当前播放会话、
    不参与降级）；mpv 只做可取消的 `Range: bytes=0-1` 后台预热，不创建第二个 mpv，
    也不调用共享播放器的 `Load/Play`。预载失败/取消不影响当前播放。
  - mpv 事件经 `playback:event` 带会话 token 桥接前端；failover 事件改为扇出，终端事件
    阻塞送达、其余事件在下游跟不上时丢弃，避免前端消费速度反压到 mpv 读循环。
- **修复 `@(event)` 事件接线**（`2026-09-12`）：Vue 会把 `@(fallback)` 编译成字面 prop
  `on(fallback)`，`emit('fallback')` 永远匹配不上，web→mpv 降级回调此前完全失效；
  已全部改为 `@fallback` / `@playback`，并加了禁止该写法的回归断言。

- **vod-ux / vod-nav 已合入**：点播 UX 改进（`b311b21c`，09-03）与播放/导航修复
  （`dfb8777a`，09-04）均已合入（09-01 快照记为「待合入」，现完成）。
- **播放原地重试 + mpv 断点续播**（`47b1e957`，09-05，含于 v0.4.5）：hls.js 的
  `MEDIA_ERROR`/`NETWORK_ERROR` 先原地重试（各自设上限），耗尽再 fallback；mpegts.js
  网络错误 `unload()+load()` 重试。fallback 携带 `currentTime`，mpv 从断点 `Seek`
  续播（seek 失败非致命）。Linux 无 MSE，不受影响。
- **Linux arm64 出包：尝试后放弃**（`f48d976f` → `bb0a2892`，09-05，含于 v0.4.5）：
  试图让 stock build.sh 直接编 arm64，但它构建仓库根 `.`（`cmd/unbox` 才是 main），
  `go build -o X .` 退出码 0 却只产几十 KB ar 归档，deb 无可执行文件。遂移除 Linux
  arm64；现矩阵四目标：Linux amd64、Windows amd64/arm64、macOS universal。恢复需先改
  `build/docker/build.sh` / `Dockerfile.cross`，暂不值得。
- **M3 本地媒体库**（基础 merge `adcc8f3e`，首帧海报后续提交合入 `master`，09-06/07，
  v0.4.5 之后，尚未发版）：
  自底向上：`store` 媒体库目录/条目表（`f8d94716`）→ `library` 递归扫描 + 片名清洗/
  海报匹配（`0c7b2b0b`/`9992ad3d`/`5242cb39`）→ 带 token 鉴权 + 防目录穿越的本地文件
  HTTP 服务（`6c968264`）→ 目录/条目/进度门面（`e68167b7`）→ Wails 绑定
  （`8163eaba`）→ 前端媒体库 tab（`48147692`）→ 安全加固（限制路径 `03ccb38b`、
  阻止符号链接越界 `fc76b5f1`、切页停止播放 `947d3b49`）→ Linux GStreamer 依赖声明
  （`a761c04b`）→ README 媒体库说明（`802352e9`）。
- **本地媒体库首帧海报**（已完成，`c50380bc`、`0744bdb3`）：无海报条目先尝试 WebView
  canvas 抓帧，失败后调用 mpv `--vo=image` 输出目录抓帧；缓存按 `(path, mtime)` 失效，
  首页运行时回显海报和播放进度，mpv 进度上报不会覆盖已有时长。
- **媒体库片单布局**（已完成，`f221be04`、`2e7dc1de`、`75ccd108`、`0812c1cf`、`9ff00922`）：
  播放器与片单列间距为 `6px`，片单右侧保留 `12px`，滚动条宽度为 `6px`。

## M4 之后新增的功能（本次会话）

- **设置独立页**：分别导入点播源/直播源（互不覆盖），源历史（点击切换 / 删除 / 回显当前源）。
- **首页**：点播观看历史（片名/所属站点/集数/进度），点击断点续播（mpv `Seek` / web `seekTo`）。
- **点播线路选择**：多线路源（`urls`/`storeHouse`）显示线路下拉（切换线路自动选第一个站点），
  单线路源自动隐藏线路下拉。
- **全站搜索**：并发搜所有站点（结果带所属站点、点击切到对应站详情），
  进度经 `search:progress` 事件显示，线程数可配置（1/4/8/16，默认 1）。
- **日志查看**：内存环形缓冲（64KB），设置页弹窗查看 + 复制按钮。
- **简介 HTML 渲染**：`v-html` + DOMPurify 白名单清洗。
- **点播详情内嵌播放器**：选集直接在详情页（海报左侧）播放，无需切回直播页。
- **站点记忆**：最后站点持久化，重启后若源未变自动恢复。
- **mpv 进度回传**：前端每 10s 轮询 `Position()`，mpv 后端也能写入观看进度。
- **详情页面板折叠**：点播详情的类目面板 / 详情面板可折叠，折叠后播放器自适应放大。
- **当前版本即时回显**：新增 `CurrentVersion()` 免联网接口，「关于」页不再显示占位符。
- **版本注入**：发布构建经 `-ldflags -X` 注入 `shell.appVersion`（`Taskfile.yml` 的 `VERSION` 变量读环境变量，默认 0.0.1）。
- **GitHub Actions 分平台出包**：`.github/workflows/release.yml`，push `v*` 标签三平台原生编译并创建 Release（弃 goreleaser）。
- **README + MIT LICENSE**：补仓库 README（特性/安装/构建/路线图）与许可证。
- **Logo 迭代**：三面三色柔和配色 + 顶面眯眯眼笑脸（源文件 `build/appicon.svg`）。
- **窗口最小尺寸**：`OpenWindow` 设 `MinWidth=720`/`MinHeight=480`，防止窗口缩到只剩标题栏淹没播放器。
- **点播详情海报右对齐**：海报 `margin-left:auto` 靠右贴合边框。
- **源选择/集数贴近播放器**：源 tab 与集数列表移到播放器正下方（左侧栏，集数列表 `max-height` 可滚动），
  避免被右侧简介栏隔开。
- **设置页关于扩展**：新增「关于我们」（当前版本 / 内部版本 / logo / 简介）、「免责条款」、「开源库」、
  「源码」（跳 GitHub）、「捐助」（爱发电）入口。
- **日志增强**：每行日志带内部版本前缀（`debug.ReadBuildInfo().Main.Version`，本地通常为 `(devel)`）；
  前端 RuntimeError 经 `LogError` 接口写入日志缓冲，可在「查看日志」里看到。

## 已知限制 / 环境坑

- **FongMi多线路源的 `csp_` 站点**：实测 `csp_` JAR 是编译后的 Android dex（APK，
  `classes.dex`），不是 JS——需安卓 ART/DexClassLoader 才能跑，纯 Go 本地不可行。
  故 FongMi多线路源的完整站点/线路仍未解锁；M5.1 只解锁了独立 `.js`（FongMi js0）站点。
- **WSLg 中文输入法**：Windows IME 组合事件无法经 RDP→Weston→XWayland 转发到
  WebKitGTK（microsoft/wslg 已知限制），WSLg 里打不了中文；Windows 版（WebView2）正常。
- **WSLg emoji 字体**：裸 Ubuntu 无 emoji 字体，源站点名里的 emoji 显示成方块，
  `sudo apt install fonts-noto-color-emoji` 解决。
- **WSLg COPY MODE**：`/mnt/shared_memory` 未挂载会导致窗口渲染空白（标题带
  `[WARN:COPY MODE]`）；挂载 tmpfs + `wsl --shutdown` 解决。
- **WebKitGTK 无 MSE**：Linux 上 hls.js/mpegts.js 不可用，HLS 只能走 mpv。
- **WSLg 鼠标光标不可见**：WSLg 的 XWayland 路径有光标渲染 bug；本 app 因 mpv `--wid`
  嵌入强制 `GDK_BACKEND=x11`（XWayland），撞上该 bug（光标消失、hover 仍高亮）。Windows/macOS 正常。
  缓解：Windows PowerShell 里 `wsl --shutdown` 重开（重置 WSLg 图形栈）/ `wsl --update` 更新；
  跑 app 时试 `XCURSOR_THEME=Adwaita`。切 Wayland 可修光标，但会破坏窗口显示与 mpv 嵌入，不做。
- **Linux 沙箱（userns）崩溃**：WebKitGTK 的 web 进程沙箱依赖 bwrap（bubblewrap）+ 非特权
  userns。Ubuntu 24.04+ 默认 `kernel.apparmor_restrict_unprivileged_userns=1` 拦掉 bwrap 建 userns，
  表现为 `bwrap: setting up uid map: Permission denied` → `dbus-proxy` 失败 → webview 在 cgo 里
  SIGTRAP 裸崩。已在 `cmd/unbox/main.go` 启动最早处加 `CheckLinuxPrerequisites()`：探测
  `unshare -U true`，失败打印可操作指引（sysctl 放开 userns）并干净退出，而非裸崩。
  探针依赖 `unshare`（util-linux，几乎总在）；缺失则放行不误拦。用户侧修复见 README「系统要求」。

## 后续待办

- **M5.2 `csp_` JAR**：已实测为编译 dex，本地不可行，搁置（见 M5 spec §11）。若要解锁
  FongMi多线路源完整站点，只两条路：远程爬虫代理 / 接受放弃。
- **M5.3 dr_py 方言**：核心适配已完成。后续仅保留非本次范围的 `filter`/`filter_url`/`filter_def`、crypto-js
  和 `muban` 全量模板对齐。
- **M3 本地媒体库**：✅ 已完成（基础 merge `adcc8f3e`，首帧海报与布局修复已合入 master，
  2026-09-07），详见上方「近期更新」。
- **Windows/macOS 实测**：打包已由 GH Actions 自动化；Windows NSIS 内嵌 mpv 的下载、解压、安装包执行和无系统 mpv 播放仍需 Windows 宿主机实测，macOS 仍需验证外部 mpv 安装与播放。
- 停车项：failover `Events()` fan-out、probe 同步阻塞 `Load`、tvbox 剧集缓存上限、
  点播收藏等。

## 已排出的方向（设计决策，勿重开）

- 丢弃 mpvlib；三平台统一「Web + 外部 mpv」。
- Wails v3 钉死 3.0.0-beta.9（Linux 后端为 GTK4）。
- 播放路由：Web 优先（H.264 HTTP），mpv 兜底（HEVC/RTMP/本地/无 MSE）。
- CMS JSON 协议实测要点（详见 M2 spec §2.1）：分类从 `type_id`/`type_name` 派生；
  `vod_play_from` 列表用 `,`、详情用 `$$$`；剧集 `$$$`/`#`/`$`。
- `csp_` JAR 已实测为编译 dex（非 JS），本地不可行；「解包取 JS」的方案前提不成立（见 M5 spec §3/§11）。
