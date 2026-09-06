# 本地媒体库首帧兜底海报设计

- 日期：2026-09-06
- 状态：已批准设计，待写实现计划
- 范围：`internal/library`、`internal/shell`、`frontend/src/App.vue`
- 不引入：ffmpeg、新原生依赖、mpv 强依赖

## 1. 背景与目标

M3 本地媒体库扫描本地目录、按片名/海报文件匹配，无海报的条目回退为纯文字卡片。
用户希望：**当本地视频没有匹配海报时，用视频本身的某一帧作为封面海报**（类比 Windows
资源管理器的视频缩略图）。

约束（来自既有决策）：
- mpv 是可选外部播放器，**默认不安装**（Windows 仅在首次播 HEVC/RTMP 时自动下载，
  macOS/Linux 需 `brew/apt install`）。不能让兜底功能依赖一个常缺席的组件。
- ffmpeg 刻意不引入（`internal/player/mpvproc/mpvproc_test.go:27` 注释「M1 不引入 ffmpeg」）。
- go.mod 无任何媒体解码库。

因此帧源必须是**始终存在**的东西：应用自带的 WebView（WebView2 / WKWebView /
WebKitGTK）里的 `<video>` 元素。现有 Web 播放路径已把本地视频 URL 喂给 `<video>` 在播，
证明 `<video>` 能加载该 URL；抓帧只多一步 `drawImage` → `toBlob`。

## 2. 现状（已核实代码事实）

- **海报匹配**：`internal/library/model.go` `findPoster(dir, stem)` 命中返回海报路径，未命中
  返回 `""`。
- **扫描**：`internal/library/library.go` `Scan()`（约 `:79`）仅在 `Poster != ""` 时
  （`:111-113`）注册 URL；无海报条目 `Poster` 留空字符串入库。
- **存储**：`internal/store/store.go` `LibraryItem.Poster`（`:78-86`），空串入库
  （`:137,459`），`ReplaceLibraryItems`（`:448`）。
- **HTTP 服务**：`internal/library/serve.go`
  - `server` 只监听 `127.0.0.1` 随机端口（`:45`），16 字节 hex token（`:24-30`）。
  - 路由 `GET /v/<seqID>?t=<token>`（`:57`），token 鉴权（`:62`），防目录穿越
    （id 不含 `/`，`:66-73`），`http.ServeFile`（`:74`）。
  - `server.register(path) string`（`:33`）登记路径并返回带 token 的完整 URL。
  - **handler 不设任何 CORS 头**（`http.ServeFile` 默认无 `Access-Control-Allow-Origin`）。
- **播放路由**：`internal/shell/service.go` `PrepareLibrary`（`:272`）→
  `library.StreamFor(path)`（`library.go:141-158`）→ `playback.Prepare` 产出 `Plan`，
  `Backend` 为 `web` 或 `mpv`。Web 后端即前端 `<video>` 直连注册 URL。
- **mpv 探测**：`internal/player/mpvplugin/manager.go` `Manager.Status().Path`
  （`:63-79,151-157`）；插件目录 `<UserConfigDir>/unbox/plugins/mpv/mpv[.exe]`。
- **缓存目录约定**：`os.UserConfigDir()/unbox/`（db `unbox.db`、mpv 插件子目录）。
- **前端**：`frontend/src/App.vue`
  - `interface LibraryItem { ... Poster: string }`（`:18`）。
  - `<img v-if="item.Poster" :src="item.Poster" class="thumb" loading="lazy" alt="" @error="imgError" />`（`:1214`）。
  - `imgError`（`:1025-1028`）仅 `display:none` 隐藏坏图。
  - 媒体库列表 `playLibraryItem(item.Path)`（`:1213`、`:411`）。

## 3. 方案概览

逐条懒生成。对每个无海报条目，按顺序尝试两条帧源，首成即止：

1. **主路径 web-canvas（始终可用）**：前端隐藏 `<video crossorigin="anonymous">`
   加载文件注册 URL → `loadeddata` 取 `duration` → seek `min(duration*0.1, 60)` 秒 →
   `seeked` → `canvas.drawImage` → `toBlob('image/jpeg', 0.8)` → 回写后端缓存。
   覆盖浏览器可解码容器：MP4 / M4V / WebM（H.264 / VP8 / VP9）。
2. **兜底 mpv one-shot（仅当主路径 `<video>` 报 `error` 且 mpv 已安装）**：后端起
   `mpv --no-config --vo=image --frames=1 --start=10% <file>` 写 JPEG 到缓存。覆盖
   MKV / AVI / RMVB / TS / FLV / HEVC 等 `<video>` 解不了的容器。
3. **两者都不成** → 纯文字卡片（= 今天行为，无回退、不崩溃）。

mpv 不在场时，非 web 可解码格式仍纯文字；主路径（MP4 等）不受影响。

## 4. 详细设计

### 4.1 后端使能：serve.go 加 CORS 头 + register 去重

`<video>` 加载的是 `http://127.0.0.1:<port>/v/...`，与 Wails 前端 origin
（`wails.localhost` 之类，三平台各异）**不同源**。跨源 video 画进 canvas 会被「污染」，
`toDataURL`/`toBlob` 抛 `SecurityError`。

修法：`serve.go` `handler()` 在响应前加
`w.Header().Set("Access-Control-Allow-Origin", "*")`；前端抓帧用的隐藏 `<video>` 设
`crossorigin="anonymous"`（无凭证、无自定义头 → 简单请求，免预检）。

安全分析：服务只绑 `127.0.0.1`、token 在 URL query 是唯一秘密、本地任意进程本就能
请求（知 token 即可）。加 ACAO 只放行 canvas 像素读取，不改变威胁模型。对既有播放
路径无影响（`<video>` 播放不依赖 CORS，仅抓帧需要）。

另：`server.register` 当前每次分配新 id、不去重；同一视频在「播放」与「抓帧」各注册一次会致 `ids` map 冗余增长。改为按绝对路径去重（同路径返回同 URL）。

### 4.2 新包 `internal/library/thumb`

仅兜底路径使用。

```go
package thumb

var ErrNoMpv = errors.New("mpv 未安装")

// Generator 抽象帧生成，便于注入 fake 测试。
type Generator interface {
    Generate(videoPath, cachePath string) error
}

// MpvGenerator 用一次性 mpv 子进程抓帧。
type MpvGenerator struct {
    mpvPath string // 来自 mpvplugin.Manager.Status().Path
    timeout time.Duration
}

func (g *MpvGenerator) Generate(videoPath, cachePath string) error
```

`Generate` 实现：
- `mpvPath == ""` → 返回 `ErrNoMpv`。
- 起子进程：`mpv --no-config --vo=image --frames=1 --start=10%
  --o=<cachePath> <videoPath>`（具体 `--vo=image` 输出参数以实测为准；备选
  `screenshot-to-file`）。超时 `timeout`（默认 15s）杀进程。
- 成功后校验 `cachePath` 存在且非空，否则返错。
- `--start=10%`：mpv 原生 seek 高效，不封顶 60s（与主路径的已知小差异，见 4.6）。

测试用注入 `Generator`（fake = 一段写 jpg 的脚本），断言命令拼接 + 缓存写入 + `ErrNoMpv`。
真实 mpv 抓帧不在 CI 跑（runner 无 mpv）。

### 4.3 ShellService 新绑定（`internal/shell/service.go`）

```go
// EnsureThumb 注册文件 URL 与缓存 URL，返回是否已缓存。
// 前端渲染无海报条目时调用。
func (s *ShellService) EnsureThumb(path string, mtime int64) (videoURL, posterURL string, cached bool, err error)

// SaveThumb 写入前端 web-canvas 抓到的 JPEG 字节到缓存，注册并返回 posterURL。
func (s *ShellService) SaveThumb(path string, mtime int64, jpeg []byte) (posterURL string, err error)

// GenerateThumbMpv 兜底路径：委托 thumb.Generator 抓帧写缓存。
// mpv 缺席返 thumb.ErrNoMpv，前端据此放弃、留纯文字。
func (s *ShellService) GenerateThumbMpv(path string, mtime int64) (posterURL string, err error)
```

实现要点：
- key = `sha1(fmt.Sprintf("%s\x00%d", path, mtime))`（`\x00` 分隔符防「a1|23」与「a|123」碰撞）；cachePath =
  `<UserConfigDir>/unbox/posters/<key>.jpg`；首用创建 `posters/` 目录。
- `videoURL` = `server.register(videoAbsPath)`；`posterURL` = `server.register(cachePath)`
  （文件未生成时请求该 URL 会 404，前端在 `SaveThumb`/`GenerateThumbMpv` 成功后才挂到
  `item.Poster`，故不会提前请求空文件）。
- `cached = fileExists(cachePath)`。
- `SaveThumb` 原子写：写临时文件 → `os.Rename` 覆盖；并发同 key 用 `singleflight` 去重。
- `GenerateThumbMpv` 复用注入的 `thumb.Generator`（生产用 `MpvGenerator`，mpvPath 取
  `mpvplugin.Manager.Status().Path`）。

### 4.4 前端管线（`frontend/src/App.vue`）

渲染媒体库列表时，对 `Poster==""` 的条目异步跑管线，分两段限流：**cached 检查段**（`EnsureThumb` 命中 `cached=true`，仅便宜 IPC）放开并发；**生成段**（web-canvas 主路径 + mpv 兜底）用简单信号量限并发 1–2。
`mtime` 取自 `item.MTime`（`LibraryItem` 已有该字段）：

1. 调 `ShellService.EnsureThumb(path, mtime)` → `{videoURL, posterURL, cached}`。
2. `cached` → 设 `item.Poster = posterURL`，秒出，结束。
3. 否则试主路径 web-canvas：
   - 创建隐藏 `<video crossorigin="anonymous" src=videoURL>`，`preload="auto"`。
   - `loadeddata` → `seek = min(video.duration * 0.1, 60)` → 设 `currentTime`。
   - `seeked` → 离屏 `<canvas>` 同视频尺寸 `drawImage` → `toBlob('image/jpeg', 0.8)`。
   - 取 `ArrayBuffer` → 调 `ShellService.SaveThumb(path, mtime, bytes)` → 设
     `item.Poster = posterURL`，结束。
   - `<video>` `error` 事件 → 主路径失败，转步骤 4。
4. 兜底：调 `ShellService.GenerateThumbMpv(path, mtime)`：
   - 成功 → 设 `item.Poster = posterURL`，结束。
   - 返 `ErrNoMpv` 或其他错 → 标记该条 `thumbAttempted=true`，留纯文字，结束。
5. 任一步超时（主路径 ~10s、兜底由后端 15s 管）→ 视同失败，走下一步/标记。

维护每条 `thumbAttempted` 标志（响应式 Map，`item.Path` 为键），防滚动重渲染时无限重试。
`imgError` 现有逻辑（隐藏坏图）保留兜底。

### 4.5 缓存与失效

- 目录 `<UserConfigDir>/unbox/posters/`，文件 `<sha1(path|mtime)>.jpg`。
- 视频重编码/替换 → mtime 变 → 新 key → 首次请求重生成；旧文件成孤儿。
- 孤儿 GC：YAGNI，v1 不做（个人媒体库量级下磁盘占用可忽略；后续可加扫描时清理
  无对应 DB 条目的缓存）。
- Scan/DB **不改**：`Poster` 留 `""` 入库，前端运行时装饰。relaunch 时
  `EnsureThumb(cached=true)` 对已生成条目秒回填、不重抓。
- 注：`posterURL` 含随机端口 + token，每次启动变化，**不可持久化到 DB**；relaunch 的
  `EnsureThumb` 往返是固有且廉价的（sub-ms 级），无需为省它做持久化。

### 4.6 帧位置与已知小差异

用户选定「代表性帧·10%（封顶 60s）」。
- 主路径 web-canvas：`seek = min(duration * 0.1, 60)`（duration 由 `loadeddata` 免费拿；
  浏览器 seek 成本随深度涨，故封顶 60s）。
- 兜底 mpv：`--start=10%`（原生 seek 高效，不封顶）。
- 差异场景仅出现在「长片 + 非 web 可解码容器 + 已装 mpv」的窄例；严格 60s 封顶需
  先探一次时长，YAGNI 不做。接受此差异。

## 5. 不做（YAGNI）

- 不持久化生成 URL 到 DB（URL 含随机 token/端口，持久化无意义，见 §4.5）。
- 不做孤儿缓存 GC。
- 不做 CSS 占位/骨架（生成中 1–2s 空着；v2 再议）。
- 不为兜底路径做 mpv 60s 封顶的时长探测。
- 不改 Scan/DB schema。
- 不改既有播放路径。

## 6. 测试计划（TDD，先写失败测试）

- `internal/library/thumb`：
  - 注入 fake mpv（脚本写一张 jpg）→ 断言命令行参数正确、缓存文件存在且非空。
  - `mpvPath==""` → 返回 `ErrNoMpv`。
  - 超时 / 非零退出 → 返错且不留下半截文件。
- `internal/library/serve`（`serve_test.go`）：
  - 响应头含 `Access-Control-Allow-Origin: *`。
  - 既有 token 鉴权 / 防穿越行为不回归。
- `internal/shell/service.go` 绑定（注入 fake `thumb.Generator` 与 fake `server`）：
  - `EnsureThumb`：缓存文件存在→`cached=true`；不存在→`cached=false`，两 URL 非空。
  - `SaveThumb`：写文件 + 返回 posterURL；原子写（并发同 key 不撕坏）。
  - `GenerateThumbMpv`：委托 fake；fake 返 `ErrNoMpv` 时透传。
- 前端编排（mock `ShellService` + fake `<video>`/`<canvas>`，测纯逻辑）：
  - cached → 直接出图，不建 video。
  - 主路径成功 → SaveThumb 被调、Poster 被设。
  - 主路径 error → 转 GenerateThumbMpv。
  - 兜底 ErrNoMpv → 标记 attempted、Poster 仍空。
  - 并发不超上限；重渲染不重试已 attempted 条目。
- 真实抓帧不在 CI 跑（runner 无 mpv、无显示），靠手动验收。

## 7. 手动验收

本地 WSL/Linux 或目标平台：
1. 一个无海报的 MP4 → 主路径出图，`posters/` 下出现 `<key>.jpg`。
2. 一个无海报的 MKV，已装 mpv → 兜底出图。
3. 同一 MKV，卸 mpv → 纯文字，不崩、不卡 UI。
4. 重启应用 → 已缓存条目秒出图，不重抓。
5. 重编码某 MP4（改 mtime）→ 新 key → 重新生成。
6. 滚动大量条目 → 限流生效，不批量起 video。

## 8. 风险与未决

- **Canvas 污染**：依赖 ACAO + `crossorigin=anonymous` 生效。已在 HTML 规范层面确认
  跨源 video 经此组合后 canvas 不被污染；实现时需在三平台 webview 各验一次
  （WebView2 / WKWebView / WebKitGTK origin 各异）。若某平台仍污染，回退为该平台
  仅走 mpv 兜底（但 mpv 可能不在 → 纯文字）。
- **非 faststart MP4**：浏览器为 seek 到 10% 可能需下载较多数据；本地 loopback 尚可，
  大文件首帧略慢（1–2s 量级）。可接受。
- **`--vo=image` 输出参数**：具体 mpv 版本对 `--vo=image --o=<path>` 的支持以实测为准，
  `--start=10%` 语法稳定。备选 `screenshot-to-file` IPC（需常驻 mpv，不取）。
