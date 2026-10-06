# Handoff — 当前状态与待办（2026-09-07）

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
  - mpv 播放器：探测优先级为应用目录内嵌 mpv → 用户插件目录 → 系统 PATH；Linux/macOS 保留安装命令兜底。
  - 丢弃 mpvlib，三平台统一「Web + 外部 mpv」。

- **M5.1 已完成**（已合入 master）：
  - 内嵌 goja JS 引擎跑 FongMi js0 爬虫（`export default` 模块 + `req`/`pdfh`/`pdfa`/`pd` 原语）。
  - 方法名/签名对齐 FongMi 官方协议（`homeContent`/`categoryContent`/`searchContent`/`detailContent`/`playerContent`），
    同时兼容 dr_py 旧名（`home`/`category`/`search`/`detail`/`play`）。
  - `Spider` Provider 集成 `.js` 站点（classify `js` → Spider）。
  - 实测结论：`csp_` JAR 是编译 dex（非 JS），本地不可行 → M5.2 搁置（见 spec §11）。

- **M5.3 dr_py 方言适配已完成**（已合入 master，merge commit `4aa0367a`）：
  - 支持 `var rule` 的 `class_parse`、`url`/`searchUrl` 占位、`muban` 覆盖、`json:`/`js:` 内联规则、`lazy` 和 GBK 解码。
  - 保留 M5.1 FongMi `export default` 动作分发路径，并补齐真实 dr_py 常用的 `fetch`/`request`/`fetch_params`、`buildUrl`、`urlDeal`、`print` 语义。
  - 公开 `hjdhnx/dr_py` 的 `360影视.js` 真实验收通过：分类 4 个、一级列表 35 条、搜索“重器” 8 条、详情与播放地址解析成功。
  - 代码提交：`c54c23cd`、`30b3cd27`、`69119f3e`、`83bf134d`、`641a0df6`、`94922abd`、`386a7774`、`1be06d6b`、`24c6315b`；真实源校准为 `6274940e`、`77c5d8f3`、`832903e0`。
  - 最终验证：`go test ./... -count=1`、`go vet ./...`、`CGO_ENABLED=1 go build ./...`、`gofmt` 全部通过。

- **M3 本地媒体库已完成**（基础实现 merge `adcc8f3e`，首帧海报与布局修复已在 `f2b7031e`，
  2026-09-07）：
  `internal/library`（递归扫描 + 片名/海报匹配 + 带 token 鉴权与防穿越的本地 HTTP
  服务 + 进度门面）、`internal/store` 目录/条目表、`internal/shell` 绑定方法、前端
  媒体库 tab（扫描 → 浏览 → 播放，切页停止，断点续播）。无海报条目现在按 `(path, mtime)`
  缓存首帧：WebView canvas 优先，mpv 图片输出兜底；生成的本地 URL 只在运行时回填首页，
  不写入数据库。详见下方「近期更新」。

- **点播播放与导航修复已完成**（已合入 master，merge `dfb8777a`，2026-09-04）：
  - 直播/点播播放器计划与页面归属隔离，切页不会显示另一页面的画面或写入点播进度。
  - 详情页折叠简介后保留原生播放控件；集数每页 36 集，分页支持两侧箭头滚动和等宽网格。
  - 类目、线路、站点选择器仅在点播列表页显示；详情返回支持首页、搜索结果和原类目列表，搜索结果缓存 5 分钟。
  - 提交：`0e4102fa`、`e1e5c9ae`、`fd33add5`、`18d9e79a`、`8f017481`、`3967f3a8`、`cd2a594a`、`a1842bcd`、`2ebb077e`。
  - 后续并发修复：播放准备与 fallback 使用前后端双向 token，后端串行化 `Load+Play` 并拒绝旧请求；停止播放会使在途请求失效，避免旧请求暂停或覆盖新播放。全站搜索事件携带 `ID/Query`，取消、重复搜索和返回操作会丢弃旧结果。
  - 验证：前端 13 项测试、生产构建、`go test ./... -count=1`、`go vet ./...`、`CGO_ENABLED=1 go build ./...` 通过。

设计文档：`docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（M1）、
`docs/superpowers/specs/2026-08-24-unbox-m2-design.md`（M2）、
`docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`（M4）、
`docs/superpowers/specs/2026-08-29-unbox-m5-js-engine-design.md`（M5）。

## 近期更新（2026-09-01 之后）

> 09-01 快照之后合入 master 的更新。下方「M4 之后新增的功能（本次会话）」为当时的
> 冻结快照，保留作历史记录，不再更新。

- **mpv 诊断补漏**（2026-10-06）：v0.8.1 发出后用户回报「mpv 启动后立即退出」，
  且**没有 stderr 内容**——正是上一轮在 `HANDOFF` 里预判的分支。排查发现上一轮的
  修复本身有三处缺陷：
  - **预检没跑在主路径上**：启动走 `shell.PickPlayer()`（`cmd/unbox/main.go:26`），
    它直接 `mpvproc.New()`；而预检写在 `mpvplugin.NewPlayer()`，只有设置页
    「重新检测」(`RefreshMPV`) 会调。等于预检对绝大多数用户是死代码。
  - **探测顺序被反转**：`PickPlayer` 先 `lookPath("mpv")` 再回退 `pluginStatus()`，
    与 `mpvplugin.Status()` 文档化的「内嵌 → 插件目录 → PATH」相反。Windows 装了
    NSIS 包但 PATH 上另有 mpv 时，坏的那个赢。**已改为全部委托 manager**（内嵌优先
    是产品决定）。
  - **静默退出时没有判据**：`waitForIPC` 丢掉了 `cmd.Wait()` 的错误，也就是退出码。
    缺 DLL (0xC0000135) 与崩溃 (0xC0000005) 都是零输出秒退，退出码是唯一能区分
    它们的证据。**已带进报错**（`mpvproc/exitcode.go`，含 Windows 常见 NTSTATUS 中文翻译）。
  - 顺带：`PickPlayer` 选中哪个 mpv 会写进日志缓冲；播放失败原因改走
    `setVodPlaybackError` 一并写入「查看日志」（此前 `vodPlaybackError` 是直接赋值，
    错误只活在界面上，用户没法把日志发给我们，只能截图）。
  - **仍未定位 mpv 本身的根因**——只补齐了证据链。需回访该用户拿新的报错：退出码
    出来即可分辨缺 DLL / 架构不匹配 / 崩溃。用户反馈该错误出现在**播放点播**时
    （即 HEVC 的 HLS 或 Web 失败后的降级，非本地文件）。
- **Wails beta.9 → beta.26 升级**（`6d083c15`，2026-10-01）：Wails v3 落后 17 个 beta
  后跟上。版本是**四处锁**，必须同步：`go.mod`、`mise.toml`、`.github/workflows/release.yml`
  的 `wails3@`、`frontend/package.json` 的 `@wailsio/runtime`（npm 与 Go 版本严格一一对应），
  另加 `App.vue` 应用内「开源库」回显与三处文档。顺带跟上 `x/sync` v0.23.0、
  `x/text` v0.42.0、`modernc.org/sqlite` v1.60.1。
  - **beta.26 移除了 `jchv/go-winloader`**（原来在 `internal/webview2/webviewloader`
    里真实 import），`go mod tidy` 会把它从 `go.mod` 清掉，属预期；CI 独立
    `go install wails3@beta.26` 自带依赖，`go list -deps` 验过 windows amd64/arm64 与
    darwin arm64 均可解析。
  - **`generate bindings` 默认输出变了**：beta.26 起默认 `.js` + JSDoc，`-ts` 成为必需参数。
    仓库 `build/Taskfile.yml` 的 `generate:bindings` 已带 `-ts`，CI 与 `mise run build:*`
    不受影响；**手动执行该命令须自己加 `-ts`**，否则前端静默丢类型、`vue-tsc` 报
    `TS18047`/`TS18046`，看着像代码回归。详见 `AGENTS.md`「关键坑」。
  - 验证：全量 `go test`/`go vet`/`gofmt`/`CGO_ENABLED=1 go build`、前端 169 用例与
    生产构建、`wails3 task linux:build` 全链路（重生成 bindings → vue-tsc → vite →
    `go build -tags production`）；前端产物 1,046 kB / gzip 310 kB，与 beta.9 一致无回退。
- **mpv 故障诊断三层加固**（2026-10-02）：用户反馈 Windows 上反复出现
  `连接 mpv IPC 失败: open \.\pipe\unbox-mpv-N: The system cannot find the file specified`。
  排查结论：**与「没装 mpv」无关**——没装会走 `未找到 mpv 可执行文件`；该报错发生在
  `cmd.Start()` 成功**之后**，即 mpv 被找到并拉起了，但进程立刻退出、IPC 管道始终没出现
  （`ERROR_FILE_NOT_FOUND` 而非 `ERROR_PIPE_BUSY`）。真正的盲点是 mpv 的 stderr 被
  `io.Discard` 丢掉了，故障只能靠猜。三层修复：
  - **预检**（`mpvplugin/version.go`）：`NewPlayer()` 先跑 `mpv --version`，跑不起来
    （缺 DLL / 被杀软拦截）或低于 `minMPVVersion`（当前 0.28.0，一行可调）当场报错；
    版本号认不出来时不拦截（fail-open，避免误伤分支版本）。
  - **保留 stderr**（`mpvproc/stderr.go`）：环形保留子进程 stderr 末尾 4KB。
  - **识别提前退出**（`mpvproc/waiter.go` + `ipc.go` 的 `waitForIPC`）：重试循环从两个
    平台文件提到 `ipc.go`，并新增只调用一次 `exec.Cmd.Wait` 的 `waiter`——`exec.Cmd`
    不允许 Wait 两次，而 Close / 命令超时 / 失败清理三处都要收尸。等管道期间发现进程
    已退出就立即返回，不再空等满 5 秒。
  - 效果：报错由 `连接 mpv IPC 失败: … 找不到文件` 变为
    `mpv 启动后立即退出（mpv 输出: Failed to load libmpv-2.dll）`。
  - 顺带明确路由：Windows/macOS 只有**本地媒体库文件、RTMP、HEVC 的 HLS、Web 失败后的
    Fallback** 四类走 mpv，所以该报错反复出现通常意味着用户在媒体库看片 / 看 H.265 源，
    且开着自动换源时每次失败都会再触发一轮，表现为连环报错。
- **捐助榜单加载态**（2026-10-02）：弹窗一打开就渲染，而榜单初值是空榜，模板按
  `Donors.length` 二选一，于是整个拉取窗口都误显示「还没有捐助记录，感谢每一份支持」，
  页脚还跟着显示「数据更新于 未知」。新增 `donation.ts` 的 `donationView(status, donors)`
  纯函数收敛分支：有数据优先展示列表（重开弹窗继续显示旧榜单、后台静默刷新），无数据时
  才看状态（加载中 / 加载失败 / 空态）。同时去掉 `catch` 里的清空——它会在任何一次拉取
  失败时抹掉已经看到的榜单。
- **Web 播放器轨道控制**（`6c8c8d9b`，2026-09-10/11）：HLS Web 播放路径新增轨道设置菜单，
  可选择清晰度、音轨和 HLS 内置字幕轨；菜单仅在 hls.js 可用时显示，mpv/FLV/原生 MP4
  路径不显示。外挂 SRT/VTT 加载入口当前隐藏，字幕转换工具保留在前端供后续复用。
- **mpv 独立窗口控件**（`2026-09-11`）：mpv 仍以独立窗口播放，但默认开启 mpv OSC，
  鼠标移入 mpv 窗口即可显示播放/暂停、进度、音量等原生控件；UnBox 主窗口不再重复
  展示暂停、继续和音量控制，保留选集、换源、收藏和播放记录等业务操作。

- **播放设置与点播自动化**（`2026-09-12`，分支 `feat/playback-settings`，尚未合入 master）：
  设置页新增「播放设置」分类，与「个性化」同款的一排按钮，每项一个按钮直接切换并
  回显「开/关」（开启时用主题强调色，悬停显示说明），不弹窗、不拼接长文案；
  三个开关默认关闭，KV 键为 `playback.autoNext`、
  `playback.autoSwitchSource`、`playback.preloadNext`（缺失/非法值按关闭处理）。
  - **自动切集**：只取当前线路剧集数组里的下一集，没有下一集就停下，不跨线路找。
  - **自动换源**：明确错误立即触发，未起播/持续缓冲 30 秒触发；按详情页线路顺序尝试，
    只接受 `Name.trim()` 完全同名的剧集，找不到就跳过该线路，每条其他线路最多一次；
    切换后沿用当前进度续播。手动切换线路与自动换源走同一条流程。
  - **预载下一集**：Web 用独立隐藏 `<video>` + 独立代理会话（不占用当前播放会话、
    不参与降级）；mpv 只做可取消的 `Range: bytes=0-1` 后台预热，不创建第二个 mpv，
    也不调用共享播放器的 `Load/Play`。预载失败/取消不影响当前播放。
  - mpv 事件经 `playback:event` 带会话 token 桥接前端；failover 事件改为扇出，终端事件
    阻塞送达、其余事件在下游跟不上时丢弃，避免前端消费速度反压到 mpv 读循环。
- **修复 `@(event)` 事件接线**（`2026-09-12`）：Vue 会把 `@(fallback)` 编译成字面 prop
  `on(fallback)`，`emit('fallback')` 永远匹配不上，web→mpv 降级回调此前完全失效；
  已全部改为 `@fallback` / `@playback`，并加了禁止该写法的回归断言。

- **Windows 安装包版本号同步**（`2026-09-20`）：控制面板「程序和功能」显示的版本取自
  `build/windows/nsis/project.nsi` 里的 `INFO_PRODUCTVERSION`（由 `wails_tools.nsh` 写入
  `Uninstall\DisplayVersion`），该值是仓库内硬编码的；`build/windows/info.json` 的
  `file_version` / `ProductVersion` 同理。现在 `build/windows/Taskfile.yml` 的 `package`
  任务在打包前用 `VERSION`（发布构建为 tag 去 v 前缀）覆盖这几处，CI 与本地打 Windows 包
  都会带上真实版本。注意只对**新构建**生效，已安装的旧版本需重新安装才会更新。

- **点播加载反馈 + 跳过片头片尾**（`2026-09-20`）：
  - 起播加载反馈：`vodPlayerLoading` 从发起播放保持到播放器上报 `playing`（画面真正出现），
    期间 `PlaybackView` 显示遮罩 + 转圈 + 「正在加载剧集…」。此前提示在「后端播放计划就绪」
    时就消失，而那时才开始拉清单/分片，等于真正的等待窗口没有反馈。
  - 跳过标记：`VodSkipMarks{IntroEnd, OutroStart}`（秒，0 为未标记）存在 `store.kv` 的
    `vod.skip.<site>.<vodID>`，按「站点+影片」共享，同剧各集通用；读取失败/非法 JSON 回退零值。
  - 判定逻辑收敛为纯函数 `frontend/src/vodSkip.ts` 的 `resolveSkipAction`：片头优先、
    时长未知不跳片尾、每个动作每集只跳一次。Web 由 `progress` 驱动，mpv 由 `playback:event`
    的位置事件驱动；跳片尾 = 跳到结尾触发既有 `ended` → 自动切集。
  - **mpv 调研结论**：mpv 无内置跳片头片尾能力，可用原语是 `--start`/`--end`（`end` 亦为
    运行时可设属性）、`edl://` 协议、Lua 脚本。UnBox 已有 IPC 的 `time-pos` 观察与 `seek`，
    因此在应用层实现最省事，Web/mpv 共用同一套规则，无需注入脚本或构造 EDL。
  - 暂未覆盖：mpv 路径目前只做片头跳过（片尾需要总时长，PlaybackEvent 尚未带 duration）。

- **Web 播放器自绘控件 / 旋转 / 全屏**（`2026-09-17`）：
  - 播放控件改为自绘（`.player-controls` 覆盖层：播放/暂停、进度、时间、倍速、静音、
    音量、画中画、全屏），`<video>` 不再带 `controls`。原生控件会跟着旋转后的画面一起
    倾斜，也没法和右上角工具按钮共存，所以必须自绘。
  - 观感：控件条与按钮**全透明**（不铺底、不做背景模糊），靠图标的 drop-shadow 保证
    亮画面下也看得清；除倍速外全部用内联 SVG 图标，既避免文案变化挤压进度条，也不依赖
    emoji 字体；进度/音量条去掉原生不透明轨道，改半透明轨道 + 白色滑块。
  - 倍速是自绘按钮 + 弹出菜单（原生 select 无法与其它按钮统一观感）。
  - 画中画按钮只在 WebView 支持时出现（Chromium 标准 API / WebKit 的
    webkitSetPresentationMode）。
  - **坑**：`.player-controls` 是 `pointer-events: none` 覆盖层，必须把每个交互元素
    单独放行；先后漏掉过 `select` 与倍速菜单 `.rate-menu`，都表现为点击穿透到
    `<video>`、点控件却触发了播放/暂停。放行清单目前是 `button` / `input` /
    `.rate-menu`，`layoutContracts.test.ts` 有回归断言——**新增可点元素时必须同步加**。
  - 控件默认隐藏，鼠标移入淡入；播放中静置 3 秒自动隐藏，暂停中或轨道菜单打开时保持可见。
    单击画面切换播放/暂停（延迟 250ms 判双击），双击画面切全屏。
  - 右上角旋转按钮按 0°→90°→180°→270° 循环；90/270 时按容器与视频实际比例等比缩放，
    避免旋转后溢出被裁。旋转只作用于 `<video>`，控件与工具按钮保持水平。
  - 全屏由自绘按钮请求整个 `.playback-view` 容器，控件与工具按钮留在画面内；另支持双击
    画面与快捷键（空格/K 播放暂停、←/→ 前后 5 秒、F 全屏）。
  - 流就绪（canplay）后自动起播，不再需要手动点一次播放；自动切集/换源后画面能接着播。
  - 切集/换源时不再预先清空 `vodPlaybackPlan`：清空会卸载 `<video>`，导致全屏退出、
    画面黑屏，且新流不自动播放。新计划就绪后直接替换，失败时由 catch 清空。

- **vod-ux / vod-nav 已合入**：点播 UX 改进（`b311b21c`，09-03）与播放/导航修复
  （`dfb8777a`，09-04）均已合入（09-01 快照记为「待合入」，现完成）。
- **播放原地重试 + mpv 断点续播**（`47b1e957`，09-05，含于 v0.4.5）：hls.js 的
  `MEDIA_ERROR`/`NETWORK_ERROR` 先原地重试（各自设上限），耗尽再 fallback；mpegts.js
  网络错误 `unload()+load()` 重试。fallback 携带 `currentTime`，mpv 从断点 `Seek`
  续播（seek 失败非致命）。Linux 无 MSE，不受影响。
- **Linux arm64 出包：尝试后放弃**（`f48d976f` → `bb0a2892`，09-05，含于 v0.4.5）：
  试图让 stock build.sh 直接编 arm64，但它构建仓库根 `.`（`cmd/unbox` 才是 main），
  `go build -o X .` 退出码 0 却只产几十 KB ar 归档，deb 无可执行文件。遂移除 Linux
  arm64；现矩阵四目标：Linux amd64、Windows amd64/arm64、macOS universal。恢复需先改
  `build/docker/build.sh` / `Dockerfile.cross`，暂不值得。
- **M3 本地媒体库**（基础 merge `adcc8f3e`，首帧海报后续提交合入 `master`，09-06/07，
  v0.4.5 之后，尚未发版）：
  自底向上：`store` 媒体库目录/条目表（`f8d94716`）→ `library` 递归扫描 + 片名清洗/
  海报匹配（`0c7b2b0b`/`9992ad3d`/`5242cb39`）→ 带 token 鉴权 + 防目录穿越的本地文件
  HTTP 服务（`6c968264`）→ 目录/条目/进度门面（`e68167b7`）→ Wails 绑定
  （`8163eaba`）→ 前端媒体库 tab（`48147692`）→ 安全加固（限制路径 `03ccb38b`、
  阻止符号链接越界 `fc76b5f1`、切页停止播放 `947d3b49`）→ Linux GStreamer 依赖声明
  （`a761c04b`）→ README 媒体库说明（`802352e9`）。
- **本地媒体库首帧海报**（已完成，`c50380bc`、`0744bdb3`）：无海报条目先尝试 WebView
  canvas 抓帧，失败后调用 mpv `--vo=image` 输出目录抓帧；缓存按 `(path, mtime)` 失效，
  首页运行时回显海报和播放进度，mpv 进度上报不会覆盖已有时长。
- **媒体库片单布局**（已完成，`f221be04`、`2e7dc1de`、`75ccd108`、`0812c1cf`、`9ff00922`）：
  播放器与片单列间距为 `6px`，片单右侧保留 `12px`，滚动条宽度为 `6px`。

### 捐助榜单自动更新（2026-09-20）

- `docs/donors.json` 由 `.github/workflows/donors.yml` 每天 UTC 18:00 自动刷新；
  失败时不会用空文件覆盖仓库中的榜单。
- 导出脚本读取 `AFDIAN_USER_ID` / `AFDIAN_TOKEN` 环境变量，也可本地直接执行：
  `AFDIAN_USER_ID=xxx AFDIAN_TOKEN=yyy go run ./cmd/unbox-donors > docs/donors.json`，
  然后提交更新后的 `docs/donors.json`。
- 凭据存放在仓库 Settings → Secrets and variables → Actions 的 **Repository secrets**，
  Secret 名为 `AFDIAN_USER_ID` 和 `AFDIAN_TOKEN`；首次使用需先添加这两项。不要使用
  Environment secrets：定时任务无人在场审批，可能拿不到凭据。
- 手动更新：进入 Actions → donors → Run workflow。轮换 token 时，在爱发电生成新 token
  后更新 Repository secret `AFDIAN_TOKEN`（若 `user_id` 同时变化则一并更新
  `AFDIAN_USER_ID`），再手动触发 workflow。轮换后必须同步更新 Secret，否则定时任务会开始失败。
- 应用按三级链路取得榜单：远端 raw
  `https://raw.githubusercontent.com/teaGod-s/UnBox/master/docs/donors.json` → `store.kv`
  缓存键 `donations.cache`（TTL 6 小时）→ 内置快照
  `internal/shell/donors_snapshot.json`。网络、HTTP 或 JSON 解析等任何失败都静默降级；
  `donors.yml` 每次刷新和 `release.yml` 打包前都会从 `docs/donors.json` 同步快照。
  不弹错误。
- 隐私约定：界面不展示金额，金额仅用于排序；`anonymous` 条目在导出时就把昵称替换为
  「热心网友」并清空 ID/头像。`docs/donors.json` 是公开文件，脱敏必须在导出侧完成。
- 更新榜单只需替换 `docs/donors.json`，无需发版；应用会在缓存过期后自动拉到新数据。
- 已知不确定项：爱发电响应字段名可能变化，导出脚本已兼容 `sponsor` / `user` 嵌套及
  `name` / `user_name` / `nickname` 等昵称别名；字段缺失时会向 stderr 打印只含实际键名的
  警告，便于排障且不输出字段值。

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
- **WSLg 鼠标光标不可见**：WSLg 的 XWayland 路径有光标渲染 bug；本 app 因 mpv `--wid`
  嵌入强制 `GDK_BACKEND=x11`（XWayland），撞上该 bug（光标消失、hover 仍高亮）。Windows/macOS 正常。
  缓解：Windows PowerShell 里 `wsl --shutdown` 重开（重置 WSLg 图形栈）/ `wsl --update` 更新；
  跑 app 时试 `XCURSOR_THEME=Adwaita`。切 Wayland 可修光标，但会破坏窗口显示与 mpv 嵌入，不做。
- **Linux 沙箱（userns）崩溃**：WebKitGTK 的 web 进程沙箱依赖 bwrap（bubblewrap）+ 非特权
  userns。Ubuntu 24.04+ 默认 `kernel.apparmor_restrict_unprivileged_userns=1` 拦掉 bwrap 建 userns，
  表现为 `bwrap: setting up uid map: Permission denied` → `dbus-proxy` 失败 → webview 在 cgo 里
  SIGTRAP 裸崩。已在 `cmd/unbox/main.go` 启动最早处加 `CheckLinuxPrerequisites()`：探测
  `unshare -U true`，失败打印可操作指引（sysctl 放开 userns）并干净退出，而非裸崩。
  探针依赖 `unshare`（util-linux，几乎总在）；缺失则放行不误拦。用户侧修复见 README「系统要求」。

## 后续待办

- **M5.2 `csp_` JAR**：已实测为编译 dex，本地不可行，搁置（见 M5 spec §11）。若要解锁
  FongMi多线路源完整站点，只两条路：远程爬虫代理 / 接受放弃。
- **M5.3 dr_py 方言**：核心适配已完成。后续仅保留非本次范围的 `filter`/`filter_url`/`filter_def`、crypto-js
  和 `muban` 全量模板对齐。
- **M3 本地媒体库**：✅ 已完成（基础 merge `adcc8f3e`，首帧海报与布局修复已合入 master，
  2026-09-07），详见上方「近期更新」。
- **Windows/macOS 实测**：打包已由 GH Actions 自动化；Windows NSIS 内嵌 mpv 的下载、解压、安装包执行和无系统 mpv 播放仍需 Windows 宿主机实测，macOS 仍需验证外部 mpv 安装与播放。
- **回访报 mpv 报错的用户（第一轮已回，见「近期更新」）**：拿到的新报错是「启动后立即
  退出」且无 stderr，正是预判分支。第二轮已补上退出码与路径日志。**再出包后请该用户复现**，
  新的报错会形如 `mpv 启动后立即退出（退出码 3221225781 (0xC0000135)：找不到依赖的 DLL）`，
  照退出码即可定位到缺 DLL / 架构不匹配 / 崩溃。同时请他提供「查看日志」内容（现在播放
  错误会进日志了）与安装方式（NSIS 包 or 便携 exe）。
- 停车项：failover `Events()` fan-out、probe 同步阻塞 `Load`、tvbox 剧集缓存上限、
  点播收藏等。

## 已排出的方向（设计决策，勿重开）

- 丢弃 mpvlib；三平台统一「Web + 外部 mpv」。
- Wails v3 钉死 3.0.0-beta.26（Linux 后端为 GTK4）；升版必须同步四处：go.mod、mise.toml、.github/workflows/release.yml 的 `wails3@`、frontend 的 `@wailsio/runtime`（四者版本必须一致，否则桥接不匹配）。另注意 `generate bindings` 自 beta.26 起默认输出 `.js`，须带 `-ts`。
- 播放路由：Web 优先（H.264 HTTP），mpv 兜底（HEVC/RTMP/本地/无 MSE）。
- **mpv 预检放在 `NewPlayer()`，不要挪进 `Status()`**：`Status()` 被前端高频轮询（`MPVStatus`）且被首帧抓取调用，里面起子进程会拖慢这些路径；预检只在真正要创建播放器时做一次。
- CMS JSON 协议实测要点（详见 M2 spec §2.1）：分类从 `type_id`/`type_name` 派生；
  `vod_play_from` 列表用 `,`、详情用 `$$$`；剧集 `$$$`/`#`/`$`。
- `csp_` JAR 已实测为编译 dex（非 JS），本地不可行；「解包取 JS」的方案前提不成立（见 M5 spec §3/§11）。
