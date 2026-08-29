# Handoff — 当前状态与待办（2026-08-28）

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
  - mpv 插件：探测 + 一键安装（Linux/macOS 弹命令，Windows 下载 mpv.exe 并校验 SHA）。
  - 丢弃 mpvlib，三平台统一「Web + 外部 mpv」。

- **M5.1 已完成**（分支 `feat/m5.1-js-crawler`，待合入 master）：
  - 内嵌 goja JS 引擎跑 FongMi js0 爬虫（`export default` 模块 + `req`/`pdfh`/`pdfa`/`pd` 原语）。
  - 方法名/签名对齐 FongMi 官方协议（`homeContent`/`categoryContent`/`searchContent`/`detailContent`/`playerContent`），
    同时兼容 dr_py 旧名（`home`/`category`/`search`/`detail`/`play`）。
  - `Spider` Provider 集成 `.js` 站点（classify `js` → Spider）。
  - 实测结论：`csp_` JAR 是编译 dex（非 JS），本地不可行 → M5.2 搁置（见 spec §11）。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（M1）、
`docs/superpowers/specs/2026-08-24-unbox-m2-design.md`（M2）、
`docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`（M4）、
`docs/superpowers/specs/2026-08-29-unbox-m5-js-engine-design.md`（M5）。

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

## 后续待办

- **M5.2 `csp_` JAR**：已实测为编译 dex，本地不可行，搁置（见 M5 spec §11）。若要解锁
  FongMi多线路源完整站点，只两条路：远程爬虫代理 / 接受放弃。
- **M5.3 dr_py 方言**（可选）：`var rule` 重型方言（`muban`/`class_parse`/`lazy`/
  `filter` + `json:`/`js:` 内联规则），多数在 server 源（已被 `tvbox.Drpy` 覆盖），视需求再定。
- **M3 本地媒体库**：未开始。
- **Windows/macOS 实测**：打包已由 GH Actions 自动化，但 mpv 插件下载/安装、Web 播放的
  运行时行为仍需各自宿主机实测。
- 停车项：failover `Events()` fan-out、probe 同步阻塞 `Load`、tvbox 剧集缓存上限、
  点播收藏等。

## 已排出的方向（设计决策，勿重开）

- 丢弃 mpvlib；三平台统一「Web + 外部 mpv」。
- Wails v3 钉死 3.0.0-beta.9（Linux 后端为 GTK4）。
- 播放路由：Web 优先（H.264 HTTP），mpv 兜底（HEVC/RTMP/本地/无 MSE）。
- CMS JSON 协议实测要点（详见 M2 spec §2.1）：分类从 `type_id`/`type_name` 派生；
  `vod_play_from` 列表用 `,`、详情用 `$$$`；剧集 `$$$`/`#`/`$`。
- `csp_` JAR 已实测为编译 dex（非 JS），本地不可行；「解包取 JS」的方案前提不成立（见 M5 spec §3/§11）。
