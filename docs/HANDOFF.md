# Handoff — 当前状态与待办（2026-08-23）

接手时先读 `AGENTS.md` 建立上下文，再读本文件了解进度与卡点。

## 总体进度

- **M1 已完成**（Plans 1–4 全部合并到 master）：
  配置解析、M3U 导入、直播浏览/播放、测速/故障切换、收藏，以及 `--wid` 窗口嵌入。
- **M2 未开始**：TVBox 点播（VOD）。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`；
实现计划：`docs/superpowers/plans/` 下的 plan1–plan4。

## 当前 git 状态

- 分支 `master`，HEAD `2b6f115`。
- 最近两个修复（均在本会话完成、已提交）：
  - `af74872` fix(shell): XID 提取改走 Wails InvokeSync，修复启动段错误。
  - `2b6f115` fix(shell): Linux 强制 GDK_BACKEND=x11，修复窗口不显示。

## 卡点：WSLg 环境问题（非代码问题，待用户在 Windows 侧解决）

**现象**：`./bin/unbox` 能启动、窗口能映射、`--wid` 嵌入拿到正确 XID，但
WebView 渲染**全黑**（Vue 未 mount，连静态标题都没有）。

**根因**：WSLg 的 GPU 直通坏了 —— `/dev/dri` 不存在（`ls /dev/dri` 为空），
dmesg 里 `dxgkrnl` 有 GPU 通信警告（`dxgvmb_send_wait_sync_object_gpu`）。
WebKitGTK 2.52.3 无 GPU 时软渲染也失效：`LIBGL_ALWAYS_SOFTWARE=1`、
`WEBKIT_DISABLE_DMABUF_RENDERER=1`、`WEBKIT_DISABLE_COMPOSITING_MODE=1`
及其组合均无效。

**已确认非代码问题**：把 Plan 4 之前的代码（`b4dba38`）单独编译，在当前
环境同样渲染全黑（YAVG=16）。

**修复方向（Windows 侧，需用户操作）**：
1. `wsl --shutdown` 重启 WSLg，然后 `ls /dev/dri` 应出现 `renderD128`。
2. 若仍为空：`wsl --update`，并检查/重装 Windows 显卡驱动
   （本会话用户装 Windows 工具链时可能误动了驱动）。

验证恢复：`ls /dev/dri` 有输出后，`./bin/unbox` 应能显示频道列表。

## 后续待办

- **M2**：TVBox VOD —— `internal/provider` 增加点播源，前端加点播页。
- **Windows `--wid` 嵌入**：`internal/shell/embed_windows.go`（HWND 直取）已写好，
  需在 Windows 宿主机原生编译 + 运行验证。
- **macOS 嵌入**：libmpv + CAMetalLayer，需 macOS 机器。
- **停车项（来自 Plan 3/4）**：failover `Events()` fan-out、probe 同步阻塞
  `Load`、`ListFavorites` 的 Logo 字段等。

## 已排出的方向（设计决策，勿重开）

- 排除 JAR/Python 爬虫源。
- 嵌入播放：macOS → libmpv/CAMetalLayer，Windows/Linux → mpv 子进程 + `--wid`。
- Wails v3 钉死 3.0.0-beta.9（Linux 后端为 GTK4，非 GTK3）。
