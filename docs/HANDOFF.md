# Handoff — 当前状态与待办（2026-08-24）

接手时先读 `AGENTS.md` 建立上下文，再读本文件了解进度与卡点。

## 总体进度

- **M1 已完成**（Plans 1–4）：配置解析、M3U 导入、直播浏览/播放、测速/故障切换、
  收藏、`--wid` 窗口嵌入。
- **M2 已完成（CMS JSON 点播）**：`internal/provider/tvbox`（每站点一个 Provider），
  壳层「直播 + 多站点」改造，前端「直播/点播」切换 + 点播浏览/详情/剧集播放。
  实测用户订阅中 4 个 CMS 站点（线路 id=3：量子资源/非凡资源/无水印采集/360资源），
  真实站点全链路（分类派生→列表→详情→剧集→Resolve+Referer）已验证通过。
- **导入优化**：直播源改**按需加载**（`ShellService.LoadLive`），导入只解析配置
  （~15s 而非 2-3 分钟）；config resolver 并发化 + 直播拉取并发/去重/短超时；
  进度经 Wails 事件 `import:progress` 推给前端。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（M1）、
`docs/superpowers/specs/2026-08-24-unbox-m2-design.md`（M2）；
实现计划：`docs/superpowers/plans/` 下 plan1–plan4（M1）、m2-plan1-cms-vod（M2）。

## 当前 git 状态

- 分支 `master`，HEAD `0086cbb`（M2 完成）。
- M2 的提交（按序）：spec → 修正实测结论+fixture → plan → 各 Task 实现。
- 关键实现：`internal/provider/tvbox/`（episodes.go / cms.go / tvbox.go 及测试）、
  `internal/shell/`（多源改造 + Vod* 方法）、`frontend/src/App.vue`（点播面板）。

## 卡点（已解决）：WSLg 渲染 —— 窗口已正常显示

`./bin/unbox` 能启动、窗口映射、`--wid` 嵌入正常，窗口正常渲染（软件渲染回退）。
`/dev/dri` 仍缺失、DRI3 仍告警 → 无 GPU 加速，仅影响性能不影响显示。
（详情与历史教训见本文件历史版本；当前已非阻塞。）

## 后续待办

- **M2.5：JS 爬虫站点**（type=3 http 客户端模式）—— 已实现（`tvbox.Drpy`，
  `dbdd428`），对 drpy2/drpyS 服务调 `/api/*`，`vod_*` 复用 CMS 解析。**待真实
  drpyS 实例实测钉死端点/信封**；`playerContent` 懒加载/解析留待后续。
- **M4：内置 Web 播放 + 外部 mpv 插件** —— 已在 `feat/m4-web-playback` 实现：HLS/FLV/TS/MP4
  默认在 WebView 播放，RTMP/本地/HEVC 交给外部 mpv；share 页面由 Go 解析，本地代理负责
  headers 与 HLS 分片重写。Linux/macOS 展示安装命令，Windows 下载固定版本并校验 SHA-256。
- **M3：本地媒体库** —— 未开始。
- **Windows 播放**：mpv 插件下载/探测已支持，仍需 Windows 宿主机实测安装器与播放。
- **macOS 播放**：统一走 Web + 外部 mpv，仍需 macOS 宿主机实测 Homebrew 命令与播放。
- **停车项（来自 Plan 3/4）**：failover `Events()` fan-out、probe 同步阻塞
  `Load`、`ListFavorites` 的 Logo 字段、tvbox 剧集缓存上限、点播收藏/观看进度。

## 已排出的方向（设计决策，勿重开）

- 排除 JAR/Python 爬虫源；JS 爬虫留 M2.5。
- 播放路由：WebView 原生视频/HLS.js/MPEG-TS.js 优先；RTMP、本地文件和 HEVC 使用外部
  mpvproc，Web 致命错误可按会话降级到 mpv。
- Wails v3 钉死 3.0.0-beta.9（Linux 后端为 GTK4，非 GTK3）。
- CMS JSON 协议实测要点（详见 M2 spec §2.1）：分类从列表 `type_id`/`type_name`
  派生（无独立分类端点）；`vod_play_from` 列表用 `,`、详情用 `$$$`；剧集 `$$$`/`#`/`$`。
