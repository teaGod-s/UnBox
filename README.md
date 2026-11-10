<p align="center">
  <img src="build/appicon.png" width="128" height="128" alt="UnBox logo" />
</p>

<h1 align="center">UnBox</h1>

<p align="center">
  跨平台 <b>TVBox 兼容</b>桌面播放器 —— 直播 + 点播，一个安装包装好即用。
</p>

<p align="center">
  <a href="https://github.com/teaGod-s/UnBox/releases"><img src="https://img.shields.io/badge/下载-Releases-2ea44f" alt="下载" /></a>
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Wails-v3.0.0-red" alt="Wails" />
  <img src="https://img.shields.io/badge/平台-Windows%20%7C%20macOS%20%7C%20Linux-blue" alt="平台" />
  <img src="https://img.shields.io/badge/License-MIT-green" alt="License" />
</p>

UnBox 是一个面向电视盒子 / IPTV 用户的桌面播放器，支持 TVBox 单线路源、
FongMi 多线路源（CMS JSON、drpy 服务、FongMi js0 爬虫）与 M3U / TXT 直播源。
无需部署服务、无需配置 Go / Node 环境，下载对应平台的安装包即可使用。

## ✨ 特性

- **直播 + 点播**：一套界面同时支持 IPTV 直播与视频点播，分类 / 分组 / 搜索齐全。
- **本地媒体库**：添加本地目录并递归扫描视频，支持海报 / 片名展示、无海报时自动抓取视频首帧、断点续播与统一播放入口。
- **智能播放路由**：H.264 走内置 Web 播放器（hls.js / mpegts.js / 原生 `<video>`），
  HEVC / RTMP / MKV 等本地容器自动切换到外部 mpv；MP4 / M4V / WebM 优先使用内置 Web 播放。
- **断点续播**：自动记录观看进度与历史，首页一键接着看。
- **播放设置**：设置页可开启自动切集、自动换源与下一集预载。自动换源在播放出错、
  持续缓冲 30 秒、或线路在准备阶段就起不来（如指向 B站/腾讯/爱奇艺 平台页面）时，
  按详情页线路顺序尝试其他线路，只匹配同名剧集、每条线路最多一次；
  预载只提前准备资源，失败不影响当前播放。三项默认关闭。
- **全站搜索**：并发搜索所有已导入站点（线程数 1/4/8/16 可调），结果实时刷新、显示所属站点与并发数，可随时中断；
  单站点 12 秒执行上限，吃满超时的站点 10 分钟内（含重启后）直接跳过；搜索结果缓存 TTL 可选 30 分钟 / 4 / 12 / 24 小时。
- **多线路 / 多站点**：FongMi多线路源支持切换线路，自动记忆上次站点。
- **内嵌 JS 爬虫**：纯 Go goja 引擎直接运行 FongMi js0、drpy2（`var rule`）与
  ES Module 爬虫（`.js` 站点），内置 cheerio / crypto-js，无需 Node 环境。
- **源管理**：点播源 / 直播源独立导入，保留历史配置，一键切换、删除、回显。
- **点播详情内嵌播放**：详情页直接选集播放，多源并列 Tab，类目 / 详情面板可折叠，播放器自适应放大。
- **HTML 简介渲染**：DOMPurify 白名单清洗后安全渲染影片简介。
- **日志查看 + 检查更新**：内置日志弹窗（可复制），日志同时落盘到配置目录 `unbox/unbox.log`，重启后仍能回看上一轮；
  检查更新走 Releases 页面重定向（不占 GitHub API 限额），被限流时给出可读提示。

## 📡 支持的源

UnBox 支持两类点播源，导入方式一致（设置页粘贴地址即可）：

| 类型                | 结构                                | 支持的站点                                                          |
|---------------------|-------------------------------------|---------------------------------------------------------------------|
| **TVBox 单线路源**  | 顶层直接含 `sites` 数组             | type=1 CMS JSON、type=3 爬虫（`http` drpy 服务 / `.js` FongMi js0 / drpy2 / ESM）、`csp_AList` / `csp_WebDAV`、`csp_XBPQ`（规则型 JAR） |
| **FongMi 多线路源** | `storeHouse` → `urls[]` → 配置 三层 | 点播页可切换线路；每条线路内部与单线路源同构                        |

> `csp_` JAR 站点按「Go 规则引擎」路线接入：`csp_XBPQ` 已支持（M6.1），
> `csp_XYQHiker` 纳入 M6.2 计划；其余 `csp_` 长尾与 xpath（type=0）暂不支持。
> `.js` 爬虫已支持 FongMi js0、drpy2（加载 `ext` 站点规则）与 ES Module
> （import/export、`assets://`、`__jsEvalReturn`）；drpy2 的 `fyfilter`/筛选与
> `cat.js` 依赖脚本仍有缺口，见「已知限制」。网盘源先支持 AList/WebDAV
> （M8.1/M8.2），云盘分享解析与 Wogg/Wobg 暂不支持。详见「已知限制」。

## 🖥️ 安装

前往 [Releases](https://github.com/teaGod-s/UnBox/releases) 下载对应平台的安装包：

| 平台    | 安装包                                             | 说明                                                                                                                    |
|---------|----------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------|
| Windows | `.exe`（NSIS 安装程序或便携版）· **amd64 / arm64** | 安装程序内嵌 mpv，装好即可播 HEVC / RTMP；**便携版是单文件、不含 mpv**，播这类内容需另行获取（应用内可直接下载） |
| macOS   | `.zip`（内含 `.app`，**arm64 + amd64 通用**）      | 解压拖入「应用程序」；HEVC 需 `brew install mpv`                                                                        |
| Linux   | `.deb`（Ubuntu / Debian）或 `.AppImage`            | `.deb` 双击安装并自动依赖 mpv，AppImage 加执行权限直接运行；AppImage 播放 HEVC / MKV / RTMP 等需 `sudo apt install mpv` |

> ⚠️ **Windows 首次运行提示**：安装包暂未做代码签名，首次双击 `.exe`（安装程序或便携版）时
> SmartScreen 会弹出「Windows 已保护你的电脑 / 发布者未知」。点「**更多信息**」→「**仍要运行**」
> 即可正常安装；每次下载新版本首次运行时可能再次提示，处理方式相同。
> 该提示需应用获得代码签名证书后才会消失，不是病毒拦截。

> 💡 **关于 mpv**：mpv 用于 HEVC / RTMP 以及 MKV、AVI、RMVB、TS 等本地容器，也用于
> WebView 无法解码的视频首帧海报兜底。**Windows 便携版是单个 `.exe`，不包含 mpv**，
> 播上面这些内容需要另行获取：应用内「mpv 插件未安装」提示里点「下载并安装 mpv」即可
> 自动装好，或者改用 NSIS 安装程序（内嵌未修改的便携版 mpv，装好即用）。Linux `.deb`
> 声明发行版 `mpv` 依赖；macOS 和 Linux `.AppImage` 仍需自行安装 mpv。MP4 / M4V /
> WebM 使用内置 Web 播放和首帧抓取。mpv 项目源码见 https://mpv.io/ 。

## 🖥️ 系统要求

| 平台    | 架构                        | 最低版本                                                     | 运行依赖                                                                                                           |
|---------|-----------------------------|--------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| Windows | amd64 / arm64               | Windows 10 1809+                                             | WebView2（安装包内置引导安装）；NSIS 安装包内嵌 mpv                                                                |
| macOS   | amd64 + arm64（通用二进制） | macOS 12 Monterey+                                           | 系统内置 WKWebView；HEVC 需 `brew install mpv`                                                                     |
| Linux   | amd64                       | Ubuntu 24.04+ / Debian 13+（需 GTK4 ≥ 4.14 + WebKitGTK 6.0） | `libgtk-4-1`、`libwebkitgtk-6.0-4`、`gstreamer1.0-libav`、`gstreamer1.0-plugins-bad`、`mpv`（`.deb` 自动声明依赖） |

> WebView2 Runtime 支持 Windows 10 1809+ 的 arm64 构建版本，故 Windows 两架构最低版本一致。

> 运行时**无需**安装 Go / Node 等开发环境 —— 前端资源已编译进二进制。Linux `.deb` 会声明 GTK /
> WebKit / GStreamer 依赖；使用 `.AppImage` 时请按下方说明手动安装。

> Linux 使用 `.AppImage` 时不会自动安装系统依赖。若出现 WebKit 的 `WebVTT encoder` 或 MP4 无法播放提示，请执行：
>
> ```bash
> sudo apt update
> sudo apt install gstreamer1.0-libav gstreamer1.0-plugins-bad
> ```
>
> 安装后重启 UnBox；MKV、HEVC、RTMP 等格式仍需 `mpv`。

> 🐧 **Linux 沙箱（userns）**：WebKitGTK 用 bubblewrap 做 web 进程沙箱，依赖**非特权用户命名空间**（unprivileged userns）。Ubuntu 24.04+ 默认以 AppArmor 限制该能力，若首次运行报 `bwrap: setting up uid map: Permission denied`，请执行：
>
> ```bash
> sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0   # 本次生效
> echo 'kernel.apparmor_restrict_unprivileged_userns=0' | sudo tee /etc/sysctl.d/99-unbox.conf  # 持久化
> ```
>
> 旧内核（Ubuntu 22.04 等）改用 `kernel.unprivileged_userns_clone=1`。应用已内置启动前探测，缺该能力时会直接打印上述指引而非崩溃。

## 🚀 使用

1. 打开 **设置** 页，粘贴导入 **点播源**（TVBox 单线路 / FongMi 多线路 JSON 地址）或 **直播源**（M3U / TXT / 订阅地址）。
2. **点播** 页选择线路与站点，浏览分类或全站搜索，点进详情页选集即可播放。
3. **直播** 页选择分组与频道，点击播放。
4. **媒体库** 页添加本地目录，点击「重新扫描」后即可浏览并播放本地视频；没有匹配海报时，MP4 /
   WebM 等浏览器可解码格式会自动抓取首帧，其他格式在 mpv 可用时走 mpv 兜底。
5. **首页** 查看观看历史，点击任意记录断点续播。

## 🔧 开发者构建

```bash
# 依赖（用 mise 管理，见 mise.toml）：Go 1.27、Node 22、Wails v3.0.0
mise install

# 开发模式（热重载）
mise run dev

# 本机原生构建
wails3 build            # 当前平台
mise run build:linux    # 或 build:win / build:mac
```

发布打包走 [GitHub Actions](.github/workflows/release.yml)：推送 `v*` 标签后，
按 Linux amd64、Windows amd64/arm64、macOS universal 四个目标构建，并自动创建
Release 挂载产物。

## 🧱 技术栈

- **后端**：Go 1.27 + [Wails v3](https://v3.wails.io)（WebView2 / WKWebView / WebKitGTK）
- **前端**：Vue 3 + TypeScript + Vite，hls.js / mpegts.js
- **存储**：SQLite（`modernc.org/sqlite`，纯 Go 免 CGO）

## 🗺️ 路线图

- [ ] Windows / macOS 完整实测与收尾
- [x] M6.1 `csp_XBPQ` 规则引擎（分类/列表/详情/搜索/播放）
- [ ] M6.2 `csp_XYQHiker` 规则引擎
- [ ] M6.3 `csp_` 长尾按收益逐类扩展
- [ ] M6.4 可选远程 spider 桥（有明确需求再做）
- [x] M7.1 修复 drpy2 站点选源（加载 `ext` 规则脚本）
- [x] M7.2 ES Module 加载器（import/export、assets://、`__jsEvalReturn`）
- [x] M7.3 `cat.js` 自有 shim（不内置 GPL 资源）+ `gbk.js` shim + `fyfilter` 展开
- [ ] M7.4 drpy2 / ESM 真实站点验收（首轮报告的 `fyfilter`、`console`、掩码、
  `某影视站` 详情与播放链路均已修复；仍缺 `setInterval` shim 与上游未声明许可证的
  `模板.js`，见 `docs/2026-10-17-m7-acceptance.md` 与 HANDOFF「近期更新」）
- [ ] M8.1 AList 原生 Provider（多盘、公开/私有、搜索）
- [ ] M8.2 WebDAV 原生 Provider（PROPFIND + Basic Auth）
- [ ] M8.3 可选 Samba
- [ ] M8.4 可选云盘分享解析（Quark/UC/Ali/115；按需）

## ⚠️ 已知限制

- FongMi多线路源中基于 `csp_` JAR 的站点：JAR 本身是 Android DEX，不能直接用 Go
  执行；但 `csp_XBPQ` 的 `ext` 是明文规则 JSON，已按 Go 规则引擎接入（M6.1）。
  其余 `csp_` 长尾（含 `csp_XYQHiker`，计划 M6.2）与 xpath（type=0）暂不支持。
  不使用内嵌 JVM/ART，也不执行下载的 JAR。
- `.js` 爬虫：FongMi js0、drpy2（`var rule`，Provider 加载 `ext`）与 ES Module
  已支持，`cat.js` / `gbk.js` 走自有 shim（不内置 GPL 资源），drpy2 的
  `fyfilter`/`filter_url` 与筛选 UI 均已打通。仍有限制：脚本若依赖
  `setInterval` 仍会失败（已注入同步 `setTimeout` 与 no-op `clearTimeout`）；
  上游未声明许可证的 `模板.js` 不随包分发。真实站点验收明细见
  `docs/2026-10-17-m7-acceptance.md`。
- 规则站给出的线路若指向平台播放页（B站 / 腾讯视频 / 爱奇艺 / 优酷 / 芒果 / 搜狐 /
  乐视），这些页面需要平台会员或解析服务，应用会直接说明原因并自动尝试下一条线路，
  不会静默失败。
- 网盘源：先支持 `csp_AList` / `csp_WebDAV`（M8.1/M8.2）。`QuarkShare` /
  `UCShare` / `AliShare` / `P115Share` / `PikPakShare` / `ThunderShare` 依赖
  用户自己的网盘 token 和云盘 API，暂不支持；`Wogg`/`Wobg` 依赖 WebView/反爬，
  不建议在桌面端实现。可以先用 AList 挂载这些网盘作为替代。
- Linux 的 WebKitGTK 无 MSE，HLS / FLV / TS 会自动切到 mpv 播放。

## 💝 捐助

UnBox 是个人「用爱发电」、完全免费的开源项目。如果你觉得它好用，愿意支持后续的开发和维护，欢迎通过 [爱发电](https://afdian.com/a/teaGod) 请我喝杯咖啡 ☕。

应用内「捐助榜单」展示 「爱发电」 支持者（设置页 → 关于 → 捐助榜单）。

或者你也可你通过微信赞赏码捐助。

<p align="center">
  <img src="build/wechat_sponsor_qrcode.jpg" width="512" height="512" alt="UnBox logo" />
</p>

每一份支持我都非常感激。当然，不捐助也完全没有关系——软件会一直免费开源下去，你使用它、反馈问题，就已经是最好的支持了。感谢每一位使用 UnBox 的朋友。🙏

## 📄 License

[MIT](LICENSE)
