# UnBox JAR（`csp_`）兼容设计

日期：2026-10-17
状态：设计定稿，待 M6.1 实施
关联：M5.1 JS 爬虫、M5.3 dr_py、`internal/provider/tvbox`

## 1. 背景

FongMi / TVBox 配置中的 `type=3` 且 `api` 形如 `csp_Xxx` 的站点由 JAR 爬虫提供。
UnBox 此前将其判定为「编译 dex，纯 Go 不可行」并整类排除（`SupportNo`）。
2026-10-17 对一个真实聚合源做了复测与逆向，结论需要修正：

- **可以逆向**：8 个真实 JAR 样本全部是普通 Android DEX（zip + `classes.dex`），
  没有 native 加壳或加密；`jadx 1.4.7` 可还原出可读 Java，`dex2jar 2.4` 也能转换。
- **不能直接执行**：JAR 依赖 Android 运行时、CatVod 宿主 API，部分还依赖
  QuickJS JNI / 腾讯 X5 WebView；Go 只靠 goja 跑不了。
- **但大量站点不需要执行 JAR**：`csp_XBPQ`、`csp_XYQHiker` 是规则驱动型蜘蛛，
  `ext` 是明文 JSON 规则文件，JAR 只是规则解释器。用 Go 重写解释器即可覆盖。

本设计的目标不是「在桌面端跑 dex」，而是**用 Go 原生实现规则型 JAR 爬虫**，
并给长尾 JAR 留一个可选的远程桥接口。

## 2. 调研证据

### 2.1 样本与配置

聚合源（`https://某反代/https://raw.example.com/某规则仓库/tvbox/main/某多线聚合.json`）
展开后 14 个下游配置：7 个是 JSON（5 个可严格解析，2 个带 `//` 注释）、
3 个是 HTML/反爬页、2 个是加密十六进制、1 个 404、1 个空响应。
在 5 个可严格解析的配置中，可统计的 646 个站点里：

- `csp_` JAR 站点 252 个（39%）。
- 跨配置合计约 360 个 `csp_` 站点。
- Top 类：`csp_XBPQ` 114、`csp_XYQHiker` 42、`csp_Bili` 约 15、
  `csp_Wobg/Wogg` 约 10、`csp_AppYsV2` 约 3；其余为长尾。

### 2.2 JAR 身份

| JAR | 大小 | DEX 类数 | 备注 |
|---|---:|---:|---|
| `fongmi_custom_spider.jar` | 142 KB | 247 | 规则/工具型 |
| `xyq_jar.jar` | 393 KB | 672 | XYQHiker/AppYsV2 等 |
| `xyq_custom_spider.jar` | 494 KB | 909 | XYQHiker/XPath 等 |
| `gao_custom_spider.jar` | 267 KB | 528 | 含 QuickJS 引用 |
| `candymuj.jar` | 686 KB | 1356 | 声明的 MD5 与下载内容不符 |
| `pg.jar` | 2.7 MB | 1701 | 依赖腾讯 X5 WebView |
| `hecheng.jar` | 1.1 MB | 2958 | 含 `csp_XBPQ` |
| `xs.jar` | 2.0 MB | 3728 | 含 XBPQ/XYQHiker/PushAgent |

全部为 zip + `classes.dex`，未见 `lib/*.so` 或加壳段。REA 对
`fongmi_custom_spider.jar` 的归档清单证据：`ev_867c14a2…`（摘要视图
`ev_3f65c65d…`），SHA-256 `f19afc0f7d0e8d3c470e18729102af932d38c06e139098aa9b40a7d1b9871e30`。

### 2.3 宿主 ABI（JAR 期望 TVBox 提供的部分）

`jadx` 还原的 `Demo/AList/XYQHiker/XBPQ` 显示标准 CatVod 接口：

```java
init(Context, String ext)
homeContent(boolean) / homeVideoContent()
categoryContent(String tid, String pg, boolean filter, HashMap ext)
detailContent(List<String> ids)
searchContent(String key, boolean quick)
playerContent(String flag, String id, List<String> vipFlags)
liveContent(String), proxy(Map), action(String)
manualVideoCheck(), isVideoFormat(String), destroy()
```

JAR 内**没有** `com.github.catvod.crawler.Spider`，宿主必须提供：

- 字段：`net`（`Net`）、`local`（`Local`）、`siteKey`（String）。
- 方法：`getActivity()`、`getProxyUrl(Map)`。
- 辅助类：`SpiderDebug`、`Net`（约 20 个方法）、`NetSocket`、`Crypto`、
  `Json`、`Local`、`Path`、`UriUtil`。
- 第三方：Gson、org.json、OkHttp（部分 JAR 自带）、`com.whl.quickjs`（JNI）、
  Android `Context/Uri/TextUtils/SharedPreferences/Base64/AlertDialog/WebView`、
  `com.tencent.smtt.*`（`pg.jar`）。

### 2.4 规则型蜘蛛

`csp_XBPQ` 的 `ext` 是一个 JSON 规则文件，例如：

```json
{
  "主页url": "https://www.6080yy1.com/",
  "分类url": "https://www.6080yy1.com/vodshow/{cateId}-{area}-{by}-{class}-{lang}----{catePg}---.html",
  "数组": "class=\"module-item\">&&module-item-text",
  "标题": "title=\"&&\"",
  "链接": "href=\"&&\"",
  "图片": "data-original=\"&&\"",
  "线路数组": "module-tab-item tab-item&&</div>",
  "播放数组": "module-blocklist scroll-box scroll-box-y&&</div>",
  "播放列表": "<a&&</a>",
  "播放标题": "<span>&&</span>",
  "播放链接": "href=\"&&\"",
  "嗅探词": ".m3u8#.mp4#...",
  "筛选": {"1": [{"key": "cateId", "name": "类型", "value": [{"n": "全部", "v": "2"}]}]}
}
```

`csp_XYQHiker` 的 `ext` 同样是明文 JSON，字段为 `搜索链接/搜索列表数组规则/片单标题`
等（XPath/JSoup 风格）。两者都属于「数据驱动」，不依赖 JAR 内的站点专用逻辑。

### 2.5 为什么不做 JVM/ART

实测：

- `dex2jar 2.4` 转换后，`AList` 在 `-noverify` 下可执行并返回合法 CatVod JSON：
  `{"parse":0,"jx":0,"class":[],"filters":{}}`；但默认校验报 `VerifyError`。
- `enjarify` 转换 247 类 0 错误，但 `AList.playerContent` 仍 `VerifyError`；
  可加载 241/246 类（缺 QuickJS 类）。
- 先例：FreeBox（JavaFX 桌面 TVBox）的 `SpiderJarLoader` **明确拒绝含
  `classes.dex` 的 JAR**；TV-Multiplatform 的 `JarLoader` 只做 `URLClassLoader`。
  两者只支持 PC 版 Java `.class` 爬虫，Android JAR 靠局域网 Android 伴侣。

结论：DEX→JVM 转换不可靠，内嵌 JVM/ART 还会带来 40–60 MB 体积、QuickJS JNI、
X5 WebView、GPL 代码与跨平台打包问题，**不作为主线**。

## 3. 设计决策

1. **主线：Go 规则引擎。** 不下载、不执行 JAR；只读取 `ext` 规则 JSON。
2. **首批引擎：`csp_XBPQ`，其次 `csp_XYQHiker`。** 先覆盖 114 + 42 个站点。
3. **规则引擎按类名分派**，不做通用 dex 解释器。未知 `csp_` 类返回「暂不支持」，
   不谎报可用。
4. **JAR 只用于元数据与版本提示**：解析 `spider` 的描述符（`path;md5;hash`），
   保留 MD5 供诊断；M6 不下载 JAR。
5. **长尾走可选远程桥**：如果后续确有需求，再定义 HTTP/JSON 协议，
   由用户自备 Android/Java 服务；不把 JVM 塞进 UnBox。

## 4. 架构

新增包 `internal/provider/tvbox/rule`（不依赖 Wails）：

```
internal/provider/tvbox/
├── jar.go              # Provider 适配器：JarProvider
├── jar_test.go
└── rule/
    ├── engine.go       # Engine 接口 + NewEngine 注册表
    ├── rule.go         # XBPQ 规则模型 + JSON 解析
    ├── extract.go      # "A&&B" 标记提取、数组切分、URL 拼接
    ├── client.go       # 带超时/UA/大小的 GET/POST
    ├── xbpq.go         # XBPQ 引擎
    ├── xbpq_test.go
    ├── extract_test.go
    └── client_test.go
```

配置层：

- `internal/config/spider.go`：`SpiderDescriptor` / `ParseSpider`。
- `internal/config/urlresolve.go`：解析 `cfg.Spider` 与字符串型 `Site.Ext` 的相对路径。
- `internal/config/classify.go`：`csp_` → kind `jar`；已知引擎 `SupportYes`，
  其余 `SupportMaybe`。
- `internal/shell/service.go`：`collectVodSites` 增加 `case "jar"`。

数据流：

```
config.Site{API:csp_XBPQ, Ext:"./XBPQ/x.json"}
  → config.ParseSpider / ResolveRelativeURLs
  → tvbox.NewJar(site)（只校验类名）
  → rule.NewEngine(ctx, class, extURL)（首次动作时懒加载规则 JSON）
  → XBPQEngine.Home/Browse/Search/Detail/Resolve
  → tvbox.JarProvider 转换为 provider.Section/Page/Media/Episode/player.Stream
```

## 5. 里程碑

### M6.0 基础设施（0.5–1 天）

- `SpiderDescriptor` 解析：`;md5;` 后缀、相对路径、MD5 保留。
- `ResolveRelativeURLs` 覆盖 `cfg.Spider` 与字符串型 `Ext`；JSON 对象/数组不误改。
- `Classify`：`csp_` → `jar`，已知引擎 `SupportYes`，未知 `SupportMaybe`。
- `collectVodSites` 接入 `JarProvider`；未知类跳过。
- **验收**：`go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿。

### M6.1 XBPQ 规则引擎（1–2 周）

- 规则模型、标记提取、数组切分、URL 展开。
- `Home`：从 `分类` 生成分组。
- `Browse`：`分类url` + `{cateId}/{catePg}` + `数组/标题/链接/图片`。
- `Search`：`搜索url` + 搜索专用字段；无搜索规则时返回空。
  M6.1 只实现 GET；`搜索模式`/POST 留到 M6.3。
- `Detail`：`影片*` 字段 + `线路数组/播放数组/播放列表` → `vod_play_from/url`。
- `Resolve`：返回播放地址；按 `直接播放` / `嗅探词` 决定是否交给解析器
  （M6.1 先统一返回 `Parse=0`，解析器接入留到 M6.3）。
- **验收**：fixture 单测 + 至少 2 个真实 XBPQ 站点端到端（分类/列表/详情/播放）。

### M6.2 XYQHiker 规则引擎（约 1 周）

> 状态：**已实现（2026-11-07）**。JSoup 链式子集 + 简单 XPath 子集 + JSONC 容错 +
> `firstPage`/`;post` 等边界语法，验收见 `docs/2026-10-17-jar-acceptance.md` §M6.2。

- 解析 XYQHiker 规则 JSON（`搜索链接`、`搜索列表数组规则`、`片单标题`、
  `搜索片单是否Jsoup写法` 等）。
- 实现 JSoup/简单 XPath 子集的提取；复用 `client.go`。
- **验收**：至少 3 个真实 XYQHiker 站点端到端。

### M6.3 长尾扩展（按收益排序）

- `csp_AppYsV2`、`csp_XPath`、`csp_Wogg/Wobg`、`csp_Bili` 等逐类评估。
- 解析器（parse）接入：`parse=1` 时返回给播放编排，不在这里做嗅探。
- **验收**：每类至少 1 个真实站点；未知类明确报「暂不支持」。

### M6.4 可选远程桥（有明确需求再做）

- 定义 `/home /category /detail /search /play` JSON 协议。
- 用户自备 Android/Java 服务；UnBox 只做客户端。
- **验收**：协议文档 + 一个最小服务示例；不作为默认安装依赖。

## 6. 非目标

- 在 UnBox 内嵌 ART/JVM 或执行 `classes.dex`。
- 下载并缓存 JAR 作为运行时（M6 只解析描述符，不下载）。
- 复刻全部 TVBox 解析器/嗅探器/代理逻辑。
- 面面俱到地支持所有 `csp_` 类；长尾明确标注「暂不支持」。

## 7. 风险与对策

| 风险 | 对策 |
|---|---|
| 同一类名在不同 JAR 中语义不同 | 引擎按规则文件字段实现，不按类名猜测；失败时明确报错 |
| 规则字段缺失/方言差异 | 每个字段做可选；为空时回退到通用字段；fixture 覆盖 |
| 站点反爬 / UA / Cookie | `client.go` 统一 UA，支持规则 `请求头`；Cookie 内存保存 |
| 规则文件过大或超时 | 15s 超时 + 4 MB 上限；失败不阻塞其余站点导入 |
| 规则文件指向 `127.0.0.1:9978` | 原样请求；失败时提示用户该源依赖本地服务 |
| 许可证 | 不复制 FongMi/TV（GPL-3.0）宿主代码；按观察到的规则格式独立实现 |
| 安全 | 不执行 JAR；只解析 JSON；不把规则 URL 当作可执行内容 |
| MD5 漂移 | 保留声明 MD5 用于诊断；M6 不校验（因为不下载 JAR） |

## 8. 证据附录

- 聚合源配置：`某多线聚合.json`（14 个下游 URL）。
- 8 个 JAR 的 SHA-256：
  `9cd4cea8…`（candymuj）、`f19afc0f…`（fongmi）、`646c5449…`（gao）、
  `2faba4b3…`（hecheng）、`0e622ce7…`（pg）、`c81eedda…`（xs）、
  `cf4a2528…`（xyq_custom）、`ace8cb39…`（xyq_jar）。
- MD5 实测：`pg`、`gao custom_spider`、`xyq_custom_spider`、`XYQ` 与配置声明一致；
  `candymuj` 不一致（服务端内容漂移）。
- JVM 实验：`dex2jar 2.4` + `-noverify` 可执行 `AList`；`enjarify` 仍有
  `VerifyError`；不纳入主线。
- 先例：FreeBox `SpiderJarLoader.isJarAvailable()` 拒绝 `.dex`；
  TV-Multiplatform `JarLoader.kt` 仅 `URLClassLoader`。
