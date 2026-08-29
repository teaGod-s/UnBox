# Unbox M1 设计文档：桌面壳 + mpv 播放 + IPTV

日期：2026-08-17
状态：待评审
里程碑：M1（共 M1–M3）

---

## 1. 项目定位

Unbox 是一个跨平台（Windows / macOS / Linux）桌面媒体聚合播放器，用 Go +
Wails v3 实现。它把多种来源统一到一个 `Provider` 接口之后，用同一套 UI 浏览、
同一个播放内核播放。

**支持的来源（分里程碑交付）**

| 里程碑 | 来源 | 状态 |
|---|---|---|
| M1 | TVBox 配置中的直播源（`lives`）、独立 M3U/TXT | 本文档 |
| M2 | TVBox 点播站点（`sites`）：CMS + JS 爬虫 | 后续 spec |
| M3 | 本地媒体库 | 后续 spec |
| ~~M4~~ | ~~Jellyfin/Emby~~ | 已舍弃 |

**明确不做**

- JAR 爬虫（`csp_*`，Android dex + ARM native 加壳，桌面端不可行）
- Python 爬虫（hipy/drpy-py，需额外运行时，冲击"安装即用"）
- 上述两类统一通过 type 4 外挂进程协议留出扩展位，不内置

---

## 2. 调研结论（决定本设计的实测数据）

### 2.1 源生态实测

对用户提供的订阅链接 `https://example.com/apia.php?id=1` 实际抓取分析。
该链接为三层嵌套结构：

```
apia.php?id=1              → storeHouse[] 多仓库列表（含 clan:// 协议）
  └─ oua.php?b=某Tvbox多线路        → urls[] 聚合 55 条线路
       └─ apib.php?id=N    → 实际配置（`jhSPAyzn**<base64>` 混淆）
```

成功解析 5 个线路配置，共 **309 个点播站点**，类型分布：

| 类型 | 数量 | 占比 | M1+M2 可用 |
|---|---|---|---|
| T3 — JAR 爬虫（`csp_*`） | 261 | 84.5% | 否 |
| T3 — JS 爬虫 | 17 | 5.5% | 是（M2） |
| T3 — http 形式 | 10 | 3.2% | 可能（M2） |
| T4 — 外挂 remote | 8 | 2.6% | 需外挂进程 |
| T1 — CMS JSON | 8 | 2.6% | 是（M2） |
| T3 — Python 爬虫 | 5 | 1.6% | 否 |
| T0 — XPath | 0 | 0% | — |

**结论一**：该订阅源为 JAR 派，点播可用率约 8%（乐观 14%）。这是源的派系问题，
不是技术路线问题——JS 爬虫生态（drpy2/hipy）有自己的订阅源，占比会相反。

**结论二**：T0（XPath）实测占比为 0，该格式在当前生态中已基本消失。**不列入内置
实现**（早期设计草案中曾计划内置，据实测数据移除）。

**结论三**：`lives` 直播源不经过任何爬虫，纯 m3u/txt + HTTP，**100% 可用**。
5 个配置共含 32 组直播源。这是 M1 的价值基础。

### 2.2 spider 加壳实测

配置中 `spider` 字段指向的 JAR（伪装为 `.jpg`，MD5 与配置声明一致）解包后：

```
classes.dex               36 KB    仅为壳
assets/ftyguard_v7.so     81 KB    ARM32 原生库
assets/ftyguard_v8.so    102 KB    ARM64 原生库
assets/ftyshinidie.guard 993 KB    加密的真实逻辑
```

真实爬虫逻辑经 ARM native 库解密加载。x86/x64 桌面平台指令集不兼容，
dex2jar 仅能得到空壳。**JAR 路线在桌面端不可行**，此为排除依据。

### 2.3 配置格式实测

7 个真实样本中：

- **2 个**（line-2, line-12）标准与宽松 JSON 解析器**均无法解析**
  （line-12 含 `//数据接口` 行注释；line-2 存在其他语法问题）
- **4 个**必须禁用严格模式（含非法控制字符）才能解析
- **1 个**为多仓 `storeHouse` 结构
- 加密形式：`<随机前缀>**<base64>`

**结论**：Go 标准库 `encoding/json` 无法处理任何一个真实样本，必须自研容错解析层。

### 2.4 Wails v3 能力核验（源码级，v3.0.0-beta.9）

| 能力 | 源码位置 | 结论 |
|---|---|---|
| `NativeWindow() unsafe.Pointer` | `webview_window.go:1649` | 存在 |
| `nativeWindow()` 为 `windowImpl` 接口方法 | `webview_window.go:81` | 三平台必实现 |
| `BackgroundTypeTransparent` | `webview_window_options.go:286` | 存在 |
| `MacBackdropTransparent` | `webview_window_options.go:508` | macOS 透明可行 |
| Windows `Frameless` / `Mica` / `Acrylic` | `:371`, `:295-299` | 存在 |

**版本状态**：v3.0.0-beta.0 发布于 2026-08-02，至 2026-08-16 已迭代至 beta.9
（两周 10 个 beta）。API 处于快速变动期，视为项目主要外部风险。

---

## 3. 架构

### 3.1 核心抽象

系统建立在两个接口上。所有来源实现 `Provider`，所有播放实现 `Player`，
UI 层只依赖接口，不依赖具体实现。

```go
// 所有来源的统一接口
type Provider interface {
    ID() string
    Home(ctx context.Context) ([]Section, error)
    Browse(ctx context.Context, cat string, page int) (Page, error)
    Search(ctx context.Context, q string) ([]Item, error)
    Detail(ctx context.Context, id string) (Media, error)
    Resolve(ctx context.Context, epID string) (Stream, error)
}

// 播放所需的一切信息
type Stream struct {
    URL      string
    Headers  map[string]string  // Referer / UA / Cookie
    Kind     StreamKind         // HLS / MP4 / FLV / RTMP / Local
    Subtitle []SubtitleTrack
    Backups  []string           // 同频道备用流，供测速切换使用
}

// 所有播放实现的统一接口
type Player interface {
    Load(ctx context.Context, s Stream) error
    Play() error
    Pause() error
    Seek(sec float64) error
    SetVolume(v int) error
    SelectTrack(kind TrackKind, id int) error
    State() State
    Events() <-chan Event   // 位置 / 缓冲 / 错误 / EOF
    Close() error
}
```

新增来源 = 新增一个 `Provider` 实现，不触动其他代码。
更换播放实现 = 新增一个 `Player` 实现，UI 与 Provider 零改动。

### 3.2 目录结构

```
unbox/
├── cmd/
│   ├── unbox/              主程序（Wails v3）
│   └── unbox-scan/         源体检 CLI（独立可执行）
├── internal/
│   ├── config/             TVBox 配置解析层（M1 核心）
│   │   ├── fetch.go          拉取：UA 伪装、重定向、clan://、本地文件
│   │   ├── decode.go         解码：探测式剥离加密/压缩
│   │   ├── lenient.go        容错 JSON → 标准 JSON
│   │   ├── resolve.go        多仓递归展开（深度上限 + 环检测）
│   │   └── model.go          统一数据模型
│   ├── provider/
│   │   ├── provider.go       Provider 接口
│   │   ├── live/             IPTV Provider（M1）
│   │   ├── tvbox/            sites Provider（M2）
│   │   └── local/            本地媒体库（M3）
│   ├── player/
│   │   ├── player.go         Player 接口
│   │   ├── mpvproc/          mpv 子进程 + JSON IPC（Win/Linux，M1）
│   │   └── mpvlib/           libmpv 分层渲染（macOS M1；Win/Linux 后期）
│   ├── shell/                Wails v3 相关代码，全部收敛于此
│   ├── proxy/                本地代理：注入 header、HLS 重写、广告分片过滤
│   ├── probe/                测速与健康检查
│   └── store/                SQLite（modernc.org/sqlite，纯 Go 无 cgo）
├── frontend/                 Vue 3 + TypeScript + Tailwind
├── testdata/configs/         真实配置样本（含失败用例）
├── mise.toml
└── docs/superpowers/specs/
```

### 3.3 配置解析管线

五段式，除 Fetcher 外均为无 IO 的纯函数，逐段可测：

```
订阅 URL / 本地文件
   │
   ├─ Fetcher   HTTP(UA=okhttp/3.12.11、跟随重定向) / file:// / clan://
   ├─ Decoder   探测式：<前缀>**base64 / 裸 base64 / gzip / AES / 明文 / BOM
   ├─ Lenient   去 // 与 /* */ 注释、尾随逗号、非法控制字符、单引号
   ├─ Resolver  storeHouse → urls[] → 配置，递归展开（深度 ≤ 3，环检测）
   │
   └─→ Config（统一模型）
```

**Decoder 采用探测式而非配置声明式**：真实样本中加密方式无任何标识字段，
只能按特征逐一尝试。

**Resolver 必须有深度上限与环检测**：多仓可以互相引用，实测已见到三层嵌套。

### 3.4 播放层：按平台分叉

`--wid` 窗口嵌入在 macOS 上不被 mpv 支持（mpv 侧限制，非 Wails 限制）。
为避免 macOS 出现独立窗口的降级体验，M1 即按平台采用不同实现：

| 平台 | M1 实现 | 机制 |
|---|---|---|
| Windows | `mpvproc` | mpv 子进程 + `--wid=<HWND>` 嵌入 |
| Linux | `mpvproc` | mpv 子进程 + `--wid=<X11 Window>` 嵌入 |
| **macOS** | **`mpvlib`** | **libmpv + CAMetalLayer，WebView 透明分层** |

M1 之后 Windows / Linux 亦切换至 `mpvlib`，届时复用 macOS 已验证的渲染逻辑。
此安排使高风险的 A 方案在 M1 即得到验证，而非推迟到后期。

两种实现的窗口结构不同，M1 阶段三平台并非完全一致：

- **`mpvproc`（Win/Linux）**：WebView 不透明，mpv 作为子窗口占据播放区域，
  播放控件需绘制在 WebView 的非播放区域，或以独立浮层窗口实现。
- **`mpvlib`（macOS）**：WebView 透明浮于视频层之上，播放控件可直接覆盖在画面上。

前端需将播放区域布局抽象为两种模式，由后端上报当前平台的实现类型决定。
M1 之后三平台统一为 `mpvlib` 模式时，`mpvproc` 分支连同该布局模式一并移除。

**失败自动切换**逻辑位于 `Player` 接口之上：监听 `Events()` 的 EOF/错误事件，
按 `Stream.Backups` 的测速排序切换下一条流。两种 Player 实现共用该逻辑。

### 3.5 mpv 分发策略

用户无需自行安装任何依赖。mpv 随安装包分发：

| 平台 | 产物 | 内含 |
|---|---|---|
| Windows | NSIS 安装包 | `mpv.exe` + 依赖 dll |
| macOS | `.app` + DMG | libmpv 签名进 bundle |
| Linux | AppImage | mpv 及依赖库 |

预计包体 60–90 MB。运行时查找顺序：内置路径 → 系统 PATH。

---

## 4. M1 功能范围

**包含**

1. TVBox 配置解析层（多仓 / `urls[]` 聚合 / 加密 / 容错 JSON / `clan://`）
2. 独立 M3U / TXT 导入（URL 或本地文件）
3. 直播频道浏览与播放
4. 多源测速 + 失败自动切换
5. 收藏 / 最近观看 / 自定义分组（SQLite 持久化）
6. `unbox-scan` CLI：输出订阅源兼容性报告

### 4.1 unbox-scan 输出定义

```
$ unbox-scan https://example.com/api.php?id=1

订阅源: https://example.com/api.php?id=1
结构:   storeHouse(4) → urls(55) → 配置

配置解析
  成功 5 / 7      失败 2（line-2 语法错误、line-12 含行注释）

点播站点  309 个
  可用    25  (8.1%)    CMS 8、JS 17
  待定    18  (5.8%)    T3-http 10、T4-remote 8
  不支持 266 (86.1%)    JAR 261、Python 5

直播源    32 组 / 1284 频道   全部可用

结论: 该源以 JAR 爬虫为主，点播可用率低；直播完全可用。
```

`--json` 输出机器可读格式，供 UI 导入页复用。

**不包含（顺延至 M2 之后）**

- EPG 节目单（XMLTV 解析 + 频道名模糊匹配 + 时区处理，工作量不小）
- 点播站点（M2）

---

## 5. 工具链与构建

### 5.1 mise 配置

```toml
[tools]
go = "1.26.3"
node = "22"
"go:github.com/wailsapp/wails/v3/cmd/wails3" = "v3.0.0-beta.9"

[tasks.dev]
run = "wails3 dev"

[tasks.test]
run = "go test ./..."

[tasks.scan]
run = "go run ./cmd/unbox-scan"

[tasks."build:win"]
run = "wails3 build -platform windows/amd64"

[tasks."build:mac"]
run = "wails3 build -platform darwin/universal"

[tasks."build:linux"]
run = "wails3 build -platform linux/amd64"
```

**Wails 版本必须钉死**：两周内发布 10 个 beta，使用 `@latest` 会导致构建随时
失效。版本升级作为独立的显式提交进行。

### 5.2 交叉编译限制

libmpv 与 Wails 均引入 cgo，**交叉编译不可行**。三平台各自构建，
CI 使用 GitHub Actions 三个 runner，由 mise 保证各机器工具链版本一致。

---

## 6. 测试策略

### 6.1 真实样本作为测试夹具

调研阶段抓取的真实配置已保存至 `testdata/configs/`：

| 文件 | 特征 | 用途 |
|---|---|---|
| `01-storehouse.json` | `storeHouse` 多仓 + `clan://` | Resolver 正例 |
| `02-urls-aggregate.json` | `urls[]` 聚合 55 条 | Resolver 正例 |
| `line-1.raw` | `jhSPAyzn**<base64>` 混淆 | Decoder 正例 |
| `line-3/4/6/9.raw` | 解码后含非法控制字符 | Lenient 正例 |
| `line-12.raw` | 含 `//数据接口` 行注释 | **当前失败用例** |
| `line-2.raw` | 其他语法问题 | **当前失败用例** |

### 6.2 TDD 顺序

按 test-driven-development skill 执行。**第一批测试为配置解析层**：
先让 7 个真实样本全部解析通过（含 2 个当前失败用例），再编写其他代码。

解析层的正确性由真实数据定义，而非由设计假设定义。

---

## 7. M1 验收标准

- [ ] `unbox` 主程序在 Windows / macOS / Linux **编译通过**
- [ ] `unbox-scan` CLI 在三平台**编译通过**
- [ ] `testdata/configs/` 全部 7 个真实样本解析通过
- [ ] 订阅链接粘贴导入 → 频道列表展示 → 点击出画面
- [ ] 主流失效时自动切换至备用流
- [ ] 收藏 / 最近观看 / 分组正确持久化
- [ ] 三平台画面均嵌入主窗口内（macOS 经 `mpvlib` 达成，无独立窗口）
- [ ] 三平台安装包产出，用户无需预装任何依赖
- [ ] `unbox-scan <订阅链接>` 输出兼容性报告

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| Wails v3 beta API 破坏性变更 | 高 | 版本钉死；Wails 代码收敛于 `internal/shell/`，不外泄至业务层 |
| macOS libmpv + CAMetalLayer 集成复杂 | 高 | M1 即验证；失败则临时退回独立窗口，不阻塞其他平台交付 |
| cgo 导致三平台各自构建 | 中 | CI 三 runner；mise 统一工具链版本 |
| macOS 未签名公证被 Gatekeeper 拦截 | 中 | 需 Apple 开发者账号（99 USD/年），否则文档说明右键打开 |
| Linux 发行版 webkit2gtk 版本差异 | 中 | AppImage 打包；必要时限定支持的发行版范围 |
| 用户现有订阅点播可用率仅 8% | 中 | M1 交付直播价值；M2 前向用户说明源派系差异 |

---

## 9. 后续里程碑

- **M2**：TVBox 点播 Provider（CMS + JS 爬虫 + 嗅探 + VIP 解析 + 广告分片过滤）。
  M1 完成后立即启动，届时编写独立 spec。
- **M3**：本地媒体库（目录扫描、NFO 刮削、海报缓存、观看进度）。
