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

- **M2.5：JS 爬虫站点**（type=3 drpy2/hipy）—— 需先定 JS 运行时（goja 纯 Go vs
  打包 node），工作量大，未开始。
- **M4：全平台 mpvlib** —— 已定为**独立里程碑**（不在 M2/M3），把 Windows/Linux
  从 mpvproc 子进程切换为 libmpv + 分层渲染，复用 macOS 已验证逻辑。
  - **Linux mpv 嵌入延后到 M4（2026-08-24 已定）**：mpv 0.37 在 WSLg（DISPLAY+
    WAYLAND_DISPLAY 同存）选 Wayland/DRM 后端，`--wid`（X11 专有）被静默忽略 →
    mpv 独立窗口。`--vo=x11` 可强制嵌入，但前端无视频区、会铺满主窗口且无返回
    按钮。保留独立窗口至 M4 mpvlib，勿再对 mpvproc 做半成品嵌入。
- **M3：本地媒体库** —— 未开始。
- **Windows 播放**：已支持（命名管道 IPC + 隐藏控制台 + 独立开窗，见 `c06b37a`）；
  `--wid` 嵌入延后到 M4（WebView2 会覆盖 mpv 画面）。仍需 Windows 宿主机实测。
- **macOS 嵌入**：libmpv + CAMetalLayer，需 macOS 机器。
- **停车项（来自 Plan 3/4）**：failover `Events()` fan-out、probe 同步阻塞
  `Load`、`ListFavorites` 的 Logo 字段、tvbox 剧集缓存上限、点播收藏/观看进度。

## 已排出的方向（设计决策，勿重开）

- 排除 JAR/Python 爬虫源；JS 爬虫留 M2.5。
- 嵌入播放：macOS → libmpv/CAMetalLayer，Windows/Linux → mpv 子进程 + `--wid`。
  （全平台统一到 mpvlib 是独立里程碑 M4。）
- Wails v3 钉死 3.0.0-beta.9（Linux 后端为 GTK4，非 GTK3）。
- CMS JSON 协议实测要点（详见 M2 spec §2.1）：分类从列表 `type_id`/`type_name`
  派生（无独立分类端点）；`vod_play_from` 列表用 `,`、详情用 `$$$`；剧集 `$$$`/`#`/`$`。
