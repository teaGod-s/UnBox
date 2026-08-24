# Handoff — 当前状态与待办（2026-08-23）

接手时先读 `AGENTS.md` 建立上下文，再读本文件了解进度与卡点。

## 总体进度

- **M1 已完成**（Plans 1–4 全部合并到 master）：
  配置解析、M3U 导入、直播浏览/播放、测速/故障切换、收藏，以及 `--wid` 窗口嵌入。
- **M2 未开始**：TVBox 点播（VOD）。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`；
实现计划：`docs/superpowers/plans/` 下的 plan1–plan4。

## 当前 git 状态

- 分支 `master`，HEAD `a5a8016`（docs: 新增 AGENTS.md 与 handoff）。
- 本会话的修复（均已提交）：
  - `af74872` fix(shell): XID 提取改走 Wails InvokeSync，修复启动段错误。
  - `2b6f115` fix(shell): Linux 强制 GDK_BACKEND=x11，修复窗口不显示。
  - `a5a8016` docs: 新增 AGENTS.md 与 handoff，供 Codex 接手。

## 卡点（已解决）：WSLg 渲染 —— 2026-08-24 窗口已正常显示

**现状**：`./bin/unbox` 能启动、窗口映射、`--wid` 嵌入拿到正确 XID，**窗口已
正常渲染**（用户目视确认；抓帧 YMAX=235 有亮色内容，App 背景本就是近黑
`RGB(6,7,15)`，故整屏 YAVG≈16 属正常）。WebKitGTK 走软件渲染回退，无 GPU 也能显示。

**仍存在的残留（非阻塞）**：
- `/dev/dri` 仍不存在（`ls /dev/dri` 为空），`libEGL` 报
  `DRI3 error: Could not get DRI3 device`，dmesg 里 `dxgkio_query_adapter_info`
  Ioctl failed -2 —— 即 **无 GPU 硬件加速**，纯软件渲染。仅影响性能，不影响显示。

**历史教训（勿重蹈）**：早期会话曾误判「窗口全黑 = /dev/dri 缺失导致、环境阻塞」。
该结论不成立（或已失效）——当前 `/dev/dri` 仍缺失，窗口却正常显示。若再遇「窗口
全黑」，优先排查：① `./bin/unbox` 是否旧二进制（未含 GDK_BACKEND=x11 修复）；②
抓帧几何是否错误（多显示器/虚拟桌面下窗口可能在屏幕外）；最后才考虑 GPU。

**可选优化（非必需）**：想让 WebKit 用上 GPU 加速，可在 Windows 侧
`wsl --shutdown` 重启后 `ls /dev/dri` 看是否出现 `renderD128`；没有则
`wsl --update` / 重装显卡驱动。不做也不影响正常使用。

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
