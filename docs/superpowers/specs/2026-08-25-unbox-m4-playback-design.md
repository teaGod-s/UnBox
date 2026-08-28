# Unbox M4 — 播放架构重设计（内置 Web 播放 + mpv 插件）

> 状态：设计稿，待评审。实现计划另立 `docs/superpowers/plans/` 下。

## 目标

1. **内置 Web 播放**：`<video>` + hls.js/mpegts.js + Go 本地代理，覆盖 H.264 的
   HLS/FLV/TS/MP4，**安装即用、零外部依赖**，视频直接在 WebView 里渲染（嵌入
   UI 免费解决）。
2. **mpv 降级为插件**：运行时探测 mpv；未安装时 UI 提供**一键安装**；装了才用于
   HEVC/RTMP/本地文件等 Web 播不了的内容。
3. **播放路由**：按流格式 + 编码自动选后端，前端无感。

## 背景（spike 实测，2026-08-25）

- VOD（CMS 采集站，非凡资源/分享量子）主流 **H.264 HLS**，`access-control-allow-origin: *`，
  **无需 Referer**（裸 curl 可拉）。
- 「share」线路返回**网页播放器 HTML**（内嵌 `const url="...index.m3u8?sign=..."`），
  站点自己就用 hls.js + artplayer——**mpv 播不了 HTML**，需 URL 解析。
- 直播源大量已死（源腐烂，运维问题非编解码问题）。
- HEVC 未在样本中采到（4K 源多为 `csp_` JAR），占比待补；HEVC 是 mpv 插件存在的
  主理由。

## 架构

```
前端「播放」按钮
   → ShellService.Play*(id)
        → 解析 + 路由（Go）──────────────────────────────┐
             │ 判定：格式(HLS/FLV/TS/MP4) + 编码(H264/HEVC) + mpv 是否可用
             ├─ Web 后端（内置）                          │
             │     Go：share 页解析 → 真实流 URL           │
             │     Go：登记 token → 返回代理 URL           │
             │     前端：<video> + hls.js/mpegts.js 渲染   │
             └─ mpv 后端（插件，探测到才启用）              │
                   Go：mpvproc 子进程（现有）               │
```

## 组件

### 1. Web 播放后端（内置）

- **前端渲染**：新增 `<video>` 视频区 + 控制条（播放/暂停/音量/进度）。按流格式
  选 hls.js（m3u8）/ mpegts.js（flv/ts）/ 原生 `<video>`（mp4）。
- **Go 侧**：不 spawn 进程，只负责「解析 → 登记 → 返回代理 URL」。前端拿到代理
  URL 后自己渲染。
- 这是与 mpv 后端最大的差异：**Web 播放是 Go+前端协作**，mpv 是纯 Go 侧子进程。

### 2. Go 本地代理

- `GET /proxy/<token>` → 查 token 表 → 拼 Referer/UA/Cookie → 转发真实源 → 流回。
- 响应带 `Access-Control-Allow-Origin: *`（WebView 跨域到 127.0.0.1 需要）。
- **HLS 关键点**：代理 m3u8 时需**重写分片 URL** 回代理（`segment.ts` →
  `/proxy/<token>?seg=...`），否则分片直连丢 header。
- token 表内存态（`map[string]streamMeta`），带过期清理。

### 3. 播放路由（Go）

- 输入：resolved `Stream{URL, Kind, Headers}`。
- 判定顺序：
  1. `Kind == RTMP / Local` → mpv（Web 播不了）。
  2. 编码探测：HEVC/未知编码 → mpv。
  3. mpv 未安装 → 强制 Web（HEVC 会失败，返回明确错误「需安装 mpv 插件」）。
  4. 其余（H.264 HLS/FLV/MP4）→ Web。
- **编码探测**是路由的关键：对 m3u8 抓 `CODECS=`（主播放列表）或用 ffprobe 类
  嗅探；首版可先按格式路由 + 对 HEVC 失败时降级 mpv（跑起来再优化）。

### 4. mpv 插件（探测 + 一键安装）

- **探测**：复用现有 `PickPlayer()`（`exec.LookPath("mpv")`）。存在→启用 mpv 后端，
  否则前端「播放器就绪」显示「Web 模式」。
- **一键安装**（用户已定）：前端「安装 mpv 插件」按钮 → `ShellService.InstallMpvPlugin()`
  → 平台安装器 + 进度事件。分平台：
  - **Linux**：`pkexec apt-get install -y mpv`（polkit 弹密码框，GUI 友好；无 polkit
    回退 `sudo` 但需终端）。⚠️ 环境敏感，需实测。
  - **Windows**：下载预编译 mpv.exe（固定来源）→ 解压到
    `%APPDATA%/unbox/plugins/mpv/` → 用绝对路径探测。
  - **macOS**：`brew install mpv`（有 brew）或下载预编译 mpv。
- 安装成功后重新 `LookPath` / 用插件目录绝对路径刷新 mpv 后端。

### 5. share 线路 URL 解析（Go，Resolve 层）

- 现状 bug：`share/xxx` 直接喂 mpv → HTML → 失败。
- 修复：Resolve 时若 URL 返回 HTML（`content-type: text/html` 或含 `const url=`），
  正则抽出真实 `index.m3u8?sign=...`（相对路径用站点 origin 拼全）。
- 此解析对 **Web 和 mpv 都有效**（mpv 也能因此播 share 线路）。

### 6. 前端视频区

- 现有 `.player` 是纯文本/控制侧栏。M4 加真正的 `<video>` 视频区（嵌入区域），
  替代当前「独立窗口」的观感。
- 控制条（播放/暂停/音量/进度）在 Web 后端走 JS；mpv 后端仍走 Go IPC。

## 数据流（Web 路径）

```
点播详情 → 点「第01集」
  → ShellService.PlayVod(site, epID)
  → tvbox.Resolve → Stream{URL: share/xxx, ...}
  → share 解析 → Stream{URL: ...index.m3u8?sign=..., Kind: HLS}
  → 路由 → Web
  → proxy.Register(stream) → "http://127.0.0.1:PORT/proxy/<token>"
  → 返回 {Backend:"web", URL: proxyURL} 给前端
  → 前端 new Hls() → attach <video>
```

## 测试

- Go：proxy 的 header 注入 + m3u8 重写；share URL 解析；路由（H264→web、HEVC→mpv、
  RTMP→mpv、mpv 缺失降级）。
- 前端：hls.js/mpegts.js 接入（用本地 fixture 流）。
- 手工：真实流的 Web 播放 + mpv 插件安装 + HEVC 兜底。

## 风险 / 开放问题

1. **Linux 一键安装的 sudo/polkit**：GUI 应用弹密码框，需实测 polkit 是否可用。
2. **HEVC 编码探测成本**：路由要判断编码，首版可能先「按格式路由 + HEVC 失败降级」。
3. **WebKitGTK 的 MSE 成熟度**：HLS 边界情况（不连续/切轨）实测。
4. **macOS mpvlib 去留**：现有 libmpv/CAMetalLayer 是否被「Web + 外部 mpv」取代，
   待定。
5. **代理的 HLS 分片重写**：相对/绝对分片、`?sign` 透传等边界。

## 已排除

- Go `plugin` 包（.so 动态加载）：仅 Linux、版本耦合，跨平台否掉。
- 完全抛弃 mpv：HEVC/RTMP 无解。
- sidecar 独立二进制插件：偏重，mpvproc 已是子进程模式。
