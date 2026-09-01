# Unbox M3 设计文档：本地媒体库

日期：2026-09-01
状态：待评审
里程碑：M3（本地媒体库）

> 本稿复用既有播放路由 / SQLite store / ShellService 绑定，新增一个独立的
> 「媒体库」子系统（新 tab）。元数据仅文件名 + 本地 NFO，**不联网刮削**。

## 1. 目标

本地视频扫描、媒体库浏览与播放：用户添加若干本地目录，递归扫描出视频文件，
在独立的「媒体库」tab 里按目录分组浏览，点击复用现有 Web/mpv 路由播放，
并记录本地文件的观看进度（断点续播 + 进首页历史）。

## 2. 背景与复用（已核实）

- `internal/player` 的 `KindForURL` 已把 `file:` scheme 判为 `StreamLocal`；
  `playback.Controller.Prepare` 把 `StreamRTMP/StreamLocal` 直接路由到 mpv，
  `StreamMP4` 走 Web 原生 `<video>`。→ 播放层有现成入口，无需新播放器。
- 点播历史已有 `store.vod_history`（按 `(site, vod_id)` 去重）+ `ShellService` 的
  `RecordVodHistory` / `UpdateVodProgress` / `ListVodHistory`。→ 本地进度可复用。
- 前端 tab 在 `App.vue` 的 `<nav class="tabs">`（首页/点播/直播/设置），`mode`
  状态机驱动，新增 `library` 一档即可；`ShellService` 公开方法自动生成
  `frontend/bindings`。→ 新增方法即自动暴露给前端。

## 3. 范围

**本期（M3）**：
- 多目录管理：增删媒体目录，持久化（`library_dirs`）。
- 递归扫描视频文件（扩展名白名单），结果持久化（`library_items`）。
- 手动重扫 + 启动时增量重扫；识别新增 / 失效文件。
- 媒体库浏览：按目录分组、按文件名 / 最近加入排序。
- 复用播放路由播放本地文件（Web 优先、mpv 兜底，见 §5 D1）。
- 本地观看进度 + 首页历史（断点续播）。

**不在本期**：
- 在线刮削（豆瓣 / TMDB）海报 / 简介 / 演员。
- 字幕下载 / 内置字幕管理（仅回传已存在的同名 `.srt`/`.ass`）。
- 目录实时监控（inotify）—— 用「手动 / 启动时重扫」替代。
- 视频转码 / 缩略图预览。

## 4. 架构

新增包 `internal/library`（纯 Go，不依赖 Wails）：
- `scan.go` — 目录扫描 + 视频识别 + 失效检测（增量 diff）。
- `model.go` — `Dir` / `Item` 类型。
- `serve.go` — 本地文件 HTTP 服务（Web 播放 + 海报图共用，见 §5 D1）。

`internal/store` 新增表 + 方法（§6）。

`internal/shell/service.go` 新增方法（Wails 绑定面，§7）。

`frontend`：`App.vue` 新增「媒体库」tab + 目录管理 / 浏览 / 播放 UI（§8）。

## 5. 关键决策

### D1 本地文件播放路由（已定案：B）

- **方案 A（已否决）**：所有本地文件 `file://` + `StreamLocal` → 全走 mpv。
  否决理由：MP4/H264 也强制 mpv，未装 mpv 就播不了本地 MP4；且 webview 安全限制下
  `file://` 海报图渲染不可靠。
- **方案 B（采纳）**：新增本地文件 HTTP 服务 `internal/library/serve.go`
  （只监听 `127.0.0.1`，带 token 鉴权，不暴露到局域网）。按扩展名分流：
  - MP4 / M4V / WebM → `http://127.0.0.1:PORT/...` + `StreamMP4` → **Web**；
  - 其余（MKV / AVI / RMVB / TS / FLV / HEVC…）→ `file://` + `StreamLocal` → **mpv**。
  与既有「Web 优先、mpv 兜底」一致：未装 mpv 也能播 H264 MP4；海报图也走同一
  服务，天然解决 `file://` 图片渲染问题。

### D2 元数据

仅文件名 + 本地 NFO：片名 = 去扩展名、去常见季集后缀的 basename；同目录同名
`poster.jpg` / `folder.jpg` / 目录名 `.jpg` 作为海报；目录名作为分组名。不做任何联网。

### D3 存储模型

见 §6。本地观看进度**复用 `vod_history`**（`site="local"`、`vod_id=绝对路径`），
首页历史零改动；`vod_logo` 存海报的服务 URL。

### D4 目录选择

Wails v3 原生目录选择对话框（beta.9 待核实 API）；兜底：设置页文本框输入绝对路径
+ 校验目录存在。MVP 先文本框，对话框列为收尾项。

### D5 视频识别

扩展名白名单：`mp4 mkv m4v mov avi flv ts m2ts wmv rmvb rm webm mpg mpeg 3gp vob`。
按扩展名判容器；**不探测编码**（编码探测由 controller 在播放时对 HLS 做，本地文件
按扩展名分流即可）。

## 6. 数据模型

### store 新表

```sql
CREATE TABLE IF NOT EXISTS library_dirs (
  path TEXT PRIMARY KEY,
  added_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS library_items (
  path TEXT PRIMARY KEY,        -- 绝对路径，天然唯一
  name TEXT NOT NULL,           -- 去扩展名片名
  dir  TEXT NOT NULL,           -- 所属目录（分组键）
  ext  TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  mtime INTEGER NOT NULL DEFAULT 0,
  poster TEXT,                  -- 海报服务 URL，可空
  scanned_at INTEGER NOT NULL   -- 本次扫描时间，用于失效检测
);
```

进度复用 `vod_history`（`site="local"`、`vod_id=path`、`vod_title=name`、
`vod_logo=poster`、`source="local"`）。

### Go 类型（`internal/library/model.go`）

```go
type Dir struct { Path string; AddedAt int64 }
type Item struct {
    Path, Name, Dir, Ext string
    Size, MTime          int64
    Poster               string
}
```

### store 新增方法（`internal/store`）

```go
AddLibraryDir(path string) error
RemoveLibraryDir(path string) error
ListLibraryDirs() ([]LibraryDir, error)
ReplaceLibraryItems(dir string, items []LibraryItem) error // 一次性覆盖该目录的扫描结果；LibraryItem 为 store 内定义
ListLibraryItems() ([]library.Item, error)                   // 按 dir, name 排序
ListLibraryItemsByDir(dir string) ([]library.Item, error)
```

> 注：`store` 不 import `library`，`ReplaceLibraryItems` 的入参用 `store` 内定义的
> `LibraryItem` 结构（与 `library.Item` 字段一致），由 service 层做映射；保持
> 现有「store 不依赖业务包」的边界。

## 7. 接口（ShellService 新增）

```go
// 目录管理
AddLibraryDir(path string) error
RemoveLibraryDir(path string) error
ListLibraryDirs() ([]LibraryDirInfo, error)

// 扫描
ScanLibrary() (ScanLibraryResult, error)   // 全量重扫，返回新增/失效计数；后台执行+进度事件
RescanLibrary() (ScanLibraryResult, error) // 别名，语义同 ScanLibrary（供前端按钮）

// 浏览
ListLibrary() ([]LibraryItemInfo, error)

// 播放
PrepareLibrary(path string) (playback.Plan, error) // 构建 Stream → s.playback.Prepare
PlayLibrary(path string) error                     // 便捷：Prepare + 开始播放

// 进度
RecordLibraryProgress(path string, progress, duration float64) error
ListLibraryHistory() ([]VodHistoryInfo, error)     // 已由 ListVodHistory 覆盖，site="local"
```

播放路由：`PrepareLibrary` 按扩展名分流（§5 D1），构造
`player.Stream{URL, Kind}` 后调 `s.playback.Prepare`，返回既有 `playback.Plan`
（前端复用现有 mpv/Web 播放逻辑，零改动）。

## 8. 前端

- `App.vue` nav 新增「媒体库」按钮（`mode='library'`）。
- 目录管理：列表展示已加目录 + 「添加目录」（文本框输入路径 / 原生对话框）+
  「移除」+ 「重新扫描」。
- 浏览：按目录分组的条目列表（海报 + 片名 + 进度条）。
- 播放：点击条目调 `PrepareLibrary`，复用现有 `plan.Backend` 分 Web/mpv 播放；
  进度回传复用现有 10s 轮询 → `RecordLibraryProgress`。
- 首页：`ListVodHistory` 已含 `site="local"` 记录，点击本地条目 → 切媒体库并续播。

## 9. 边界与错误处理

- 目录被删 / 移动 / 盘掉线 → 扫描标记失效；UI 灰显 + 提示「目录不可用」，不崩。
- 权限不足 / 隐藏目录 → 跳过并计数，扫描结果里 `skipped` 汇总。
- 超大目录 / 海量文件 → 单次扫描文件数上限（如 50000），分页浏览（复用前端分页）。
- 同名文件不同目录 → 绝对路径为主键，天然不冲突。
- 慢盘 / 网络盘 → 扫描后台执行 + `library:progress` 事件，可中断（复用 search 进度模式）。
- 本地文件服务仅监听 `127.0.0.1` + 随机端口 + token，不暴露局域网。

## 10. 测试

- `scan`：白名单扩展名、递归、失效检测（增删文件 diff）、空目录、权限拒绝跳过。
- `model`：片名清洗（去扩展名 / 季集后缀）、海报名匹配（poster.jpg / folder.jpg / 目录名）。
- `store`：dirs 增删、items 覆盖替换、progress upsert/update。
- `serve`：token 鉴权拒绝无 token 请求、范围限制在媒体目录内（防目录穿越）。
- `service`：`PrepareLibrary` 分流（mp4→web、mkv→mpv）。

## 11. 验收标准

1. 添加目录 → 扫描出白名单内视频，按目录分组正确展示。
2. 删除目录 → 其条目消失，其他目录条目保留。
3. 重新扫描 → 新增文件出现、已删文件消失（失效检测正确）。
4. MP4/H264 本地文件走 Web 播放（未装 mpv 也能播）；MKV/HEVC 走 mpv。
5. 本地文件播放进度落库，首页历史出现该条目，点击断点续播。
6. `go test ./...` / `go vet ./...` / `gofmt` / `CGO_ENABLED=1 go build ./...` 全绿；
   `vue-tsc --noEmit` + `vitest` 通过。

## 12. 后续（非本期）

在线刮削（豆瓣/TMDB）、字幕下载、inotify 实时监控、缩略图预览、定时自动重扫。
