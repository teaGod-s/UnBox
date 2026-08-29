<p align="center">
  <img src="build/appicon.png" width="128" height="128" alt="UnBox logo" />
</p>

<h1 align="center">UnBox</h1>

<p align="center">
  跨平台 <b>TVBox 兼容</b>桌面播放器 —— 直播 + 点播，一个安装包装好即用。
</p>

<p align="center">
  <a href="https://github.com/teaGod-s/UnBox/releases"><img src="https://img.shields.io/badge/下载-Releases-2ea44f" alt="下载" /></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Wails-v3.0.0--beta.9-red" alt="Wails" />
  <img src="https://img.shields.io/badge/平台-Windows%20%7C%20macOS%20%7C%20Linux-blue" alt="平台" />
  <img src="https://img.shields.io/badge/License-MIT-green" alt="License" />
</p>

UnBox 是一个面向电视盒子 / IPTV 用户的桌面播放器，兼容主流的 TVBox 点播源
（CMS JSON、drpy2 / drpyS 客户端）与 M3U / TXT 直播源。无需部署服务、无需配置
Go / Node 环境，下载对应平台的安装包即可使用。

## ✨ 特性

- **直播 + 点播**：一套界面同时支持 IPTV 直播与视频点播，分类 / 分组 / 搜索齐全。
- **智能播放路由**：H.264 走内置 Web 播放器（hls.js / mpegts.js / 原生 `<video>`），
  HEVC / RTMP / 本地文件自动切换到外部 mpv，兼容性拉满。
- **断点续播**：自动记录观看进度与历史，首页一键接着看。
- **全站搜索**：并发搜索所有已导入站点，结果实时刷新、显示所属站点，可随时中断。
- **多线路 / 多站点**：影视仓多线路源支持切换线路，自动记忆上次站点。
- **源管理**：点播源 / 直播源独立导入，保留历史配置，一键切换、删除、回显。
- **点播详情内嵌播放**：详情页直接选集播放，多源并列 Tab，类目 / 详情面板可折叠，播放器自适应放大。
- **HTML 简介渲染**：DOMPurify 白名单清洗后安全渲染影片简介。
- **日志查看 + 检查更新**：内置日志弹窗（可复制），设置页一键检查新版本。

## 🖥️ 安装

前往 [Releases](https://github.com/teaGod-s/UnBox/releases) 下载对应平台的安装包：

| 平台 | 安装包 | 说明 |
|------|--------|------|
| Windows | `.exe`（NSIS 安装程序或便携版） | 双击安装；首次播放 HEVC / RTMP 会提示自动下载 mpv |
| macOS | `.zip`（内含 `.app`） | 解压拖入「应用程序」；HEVC 需 `brew install mpv` |
| Linux | `.deb`（Ubuntu / Debian）或 `.AppImage` | `.deb` 双击安装，AppImage 加执行权限直接运行；HEVC 需 `sudo apt install mpv` |

> 💡 **关于 mpv**：mpv 是一个可选的外部播放器，仅在播放 HEVC / RTMP / 本地文件时
> 需要。应用内置探测与一键安装引导，未装时也能正常使用 Web 播放器看 H.264 内容。

## 🖥️ 系统要求

| 平台 | 最低版本 | 运行依赖 |
|------|----------|----------|
| Windows | Windows 10 1809+ | WebView2（安装包内置引导安装）；HEVC / RTMP 需 mpv（应用内一键下载） |
| macOS | macOS 12 Monterey+ | 系统内置 WKWebView；HEVC 需 `brew install mpv` |
| Linux | Ubuntu 24.04+ / Debian 13+（需 GTK4 ≥ 4.14 + WebKitGTK 6.0） | `libgtk-4-1`、`libwebkitgtk-6.0-4`（`.deb` 自动声明依赖）；HEVC 需 `sudo apt install mpv` |

> 运行时**无需**安装 Go / Node 等开发环境 —— 前端资源已编译进二进制，除上表外无其他运行时依赖。

## 🚀 使用

1. 打开 **设置** 页，粘贴导入 **点播源**（TVBox 源 JSON 地址）或 **直播源**（M3U / TXT / 订阅地址）。
2. **点播** 页选择线路与站点，浏览分类或全站搜索，点进详情页选集即可播放。
3. **直播** 页选择分组与频道，点击播放。
4. **首页** 查看观看历史，点击任意记录断点续播。

## 🔧 开发者构建

```bash
# 依赖（用 mise 管理，见 .mise.toml）：Go 1.26、Node 22、Wails v3 beta.9
mise install

# 开发模式（热重载）
mise run dev

# 本机原生构建
wails3 build            # 当前平台
mise run build:linux    # 或 build:win / build:mac
```

发布打包走 [GitHub Actions](.github/workflows/release.yml)：推送 `v*` 标签后，
三个平台各自原生编译并自动创建 Release 挂载产物。

## 🧱 技术栈

- **后端**：Go 1.26 + [Wails v3](https://v3.wails.io)（WebView2 / WKWebView / WebKitGTK）
- **前端**：Vue 3 + TypeScript + Vite，hls.js / mpegts.js
- **存储**：SQLite（`modernc.org/sqlite`，纯 Go 免 CGO）

## 🗺️ 路线图

- [ ] **JS 引擎 + JAR 爬虫**（goja）—— 解锁影视仓（某影视仓等）`csp_` JAR 爬虫站点的完整线路与站点
- [ ] **M3 本地媒体库** —— 本地视频扫描、媒体库浏览与播放
- [ ] Windows / macOS 完整实测与收尾

## ⚠️ 已知限制

- 影视仓类源（某影视仓等）中基于 `csp_` JAR 爬虫 / xpath 的站点暂未支持，需待 JS 引擎落地；
  目前仅支持 type=1 CMS 与 type=3 http（drpy）。
- Linux 的 WebKitGTK 无 MSE，HLS / FLV / TS 会自动切到 mpv 播放。

## 💝 捐助

UnBox 是个人「用爱发电」、完全免费的开源项目。如果你觉得它好用，愿意支持后续的开发和维护，欢迎通过[爱发电](https://afdian.com/a/teaGod)请我喝杯咖啡 ☕。

每一份支持我都非常感激。当然，不捐助也完全没有关系——软件会一直免费开源下去，你使用它、反馈问题，就已经是最好的支持了。感谢每一位使用 UnBox 的朋友。🙏

## 📄 License

[MIT](LICENSE)
