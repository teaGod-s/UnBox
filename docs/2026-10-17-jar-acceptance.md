# M6.1 XBPQ 规则引擎验收（2026-11-03）

## 范围

M6.1 仅支持 `csp_XBPQ`：读取站点 `ext` 中的 XBPQ 规则 JSON，用纯 Go
解释分类、列表、搜索、详情与播放数据；不下载、不执行 JAR，不内嵌 JVM/ART。

## 自动化验收

覆盖包：`internal/provider/tvbox/rule`、`internal/provider/tvbox`。

```bash
go test ./internal/provider/tvbox/rule -count=1
go test ./internal/provider/tvbox -count=1
```

已验证：

- `ParseXBPQ`：中文规则键、`筛选` 的 `n/v` 选项解析；空规则报错。
- `Client`：固定 `okhttp/3.12.11` UA、`请求头` 透传、Cookie 内存保持、4 MiB 上限。
- `Cut` / `CutList` / `Clean`：`A&&Box` 标记提取、重复片段切分、HTML 标签与实体清理。
- `XBPQEngine.Home`：`电影$1#剧集$2` 转为两个 Section。
- `XBPQEngine.Browse`：分类地址占位符展开；相对链接按 `主页url` 转绝对地址。
- `XBPQEngine.Search`：搜索地址 `{wd}` 占位符编码。
- `XBPQEngine.Detail`：线路/播放列表两段切分；`$$$`、`#`、`$` 格式与现有
  `parseEpisodes` 对齐。
- `XBPQEngine.Resolve`：返回播放地址、UA 与站点 Referer 头。
- `JarProvider`：`Home → Browse → Detail → Resolve` 端到端；未知 `csp_` 类在
  `NewJar` 阶段明确拒绝。
- `config.Classify`：`csp_XBPQ` → `jar` / 可用；其他 `csp_` → `jar` / 待定。
- `unbox-scan`：未知 JAR 类计入「待定」，不再计入「不支持」。

## 真实规则验收

规则：`https://raw.githubusercontent.com/yw88075/tvbox/main/XBPQ/6080.json`
（README 中给出的 XBPQ 示例，2026-11-03 抓取）。

```text
rule ok: https://www.6080yy1.com/ classes: 电影$1#剧集$2#综艺$3#动漫$4#
home sections: 4
first: 电影 1
browse: Get "https://www.6080yy1.com/vodshow/1--------1---.html": EOF
```

结论：

- 真实规则 JSON 可解析；
- `Home` 能还原 4 个分类，分类 id 与规则一致；
- 目标站点对当前网络环境返回连接 EOF，未能完成真实列表/详情/播放验收。

站点可达时可用同一验收程序复测；该失败不影响 fixture 覆盖的规则引擎行为。

## 真实聚合源验收（2026-11-03）

源：`https://gh-proxy.com/https://raw.githubusercontent.com/noimank/tvbox/main/tvboxmuti.json`

展开结果：7 个终端配置、743 个站点；其中 `csp_XBPQ` 共 **114 个**，全部是
M6.1 新增支持范围：

| 线路 | XBPQ 站点数 |
|---|---:|
| 🚀1-心魔在线 | 94 |
| 🚀5-Clun在线 | 20 |

其余 `csp_` 类（如 `csp_XYQHiker`）仍按「待定」处理，不在 M6.1 范围。

### 逐站验收结果

对 114 个站点逐个执行「加载规则 → Home → Browse」：

| 阶段 | 数量 | 说明 |
|---|---:|---|
| 列表解析成功 | 1 | 冷曦影视（心魔在线），126 条列表 |
| 页面无匹配条目 | 34 | 域名停靠 / 反爬页 / JS Cookie 挑战，HTTP 200 但没有影片节点 |
| 请求失败 | 78 | 连接失败、超时、401/403/404/5xx 等站点侧问题 |
| 规则无可展示分类 | 1 | 布卡影视（Clun在线） |
| 规则解析失败 | 0 | 修复后全部规则可解析 |

### 完整链路（冷曦影视 / 心魔在线）

```text
sections=5 first=电影(movie)
items=126
item title="生化危机：爆发夜2026" id=https://www.lenxitv.cc/v/754281.html
media title="生化危机：爆发夜2026" year="2026" area="美国" type="恐怖片"
sources=[红牛云 极速云 速播云 精英云] episodes=4
episode source="红牛云" name="正片" url=https://www.lenxitv.cc/p/754281-1-1.html
stream kind=hls url=https://hn.bfvvs.com/play/dBB849Yd/index.m3u8
```

列表、详情、线路、剧集与真实播放地址均通过。冷曦影视使用 fed 播放器，播放页
把真实 m3u8 以 `data-play="SXn"+base64` 的形式放在 iframe 上；M6.1 会解码该属性
并返回真实 m3u8（实测 `HTTP 200`、`application/vnd.apple.mpegurl`）。

### 本机新增站点清单

心魔在线（94）：FREEOK、大米星球、看一看、朴樱影视、人人筷子、小鸭看看、6080、
CHNNETFILX、兰花影视、TVB云播、野荷塘、锐行加速、无心影视、极速影院、看剧吧、
花花、番茄影视、大豆高清、饭团、干饭影视、电影蜜蜂、追剧TU、熊猫影视、ok影院、
起飞影视、皮皮影视、电影先生、美奇影视、139影视、子子影视、土豪影视、101影视、
思古影视1、无极影院、明帝影视、磁力熊1、蓝光电影网、哆咪、猫影视、素白白、
大师兄影视、网飞TV、COKEMV、人人影视p、快看影视、胖虎影视、达达龟、影视大全、
歪片星球、爱看影院、秋霞影视、西瓜影视、假低端、鸭奈非4K、NIKETV、批哩㕷哩、
映迷影院、看了么、酷茶影视、野菊花电影网、首播影视、libvio、voflix、兄弟影视、
落攻阁、莫扎兔、蓝光影院、影渣渣、白嫖影视、圣城影视、冷曦影视、奇优影视、
音范丝、app影院、全能影视、北川影视、小狗影院、我爱跟剧、四个圈影视、光棍影视、
金曼TV、6v电影、91美剧、八戒影院、在线之家、奈落影院、555、万博剧院、久久美剧网、
人人美剧、米爱影视、美剧天堂、美剧窝、ppxdm动漫。

Clun在线（20）：一帆影视、人人影视、全网看、农民影视、喝茶影视、大白鲨、奇迹影视、
小镇影视、布卡影视、星辰影院、来看点播、欧乐影视、毒舌影视、熊猫影视、狐狸君、
瓜籽影院、秀儿影院、策弛影视、茶杯狐影视、飞快影视。

### 本轮修复的真实规则兼容点

在真实源上发现并修复：

1. `请求头` 既可能是对象，也可能是 `User-Agent$MOBILE_UA` 字符串；
2. 规则 JSON 中的非标准转义（如 `\&`）不再让整份规则失败；
3. `筛选` 可能是对象或 JSON 字符串，无法识别时降级为无筛选；
4. 详情字段同时支持 `影片类型/影片年代/影片地区/影片状态` 与
   `类型/年代/地区/状态`；
5. 详情标题优先取 `<h1>`，避免列表 `标题` 规则先匹配到导航「首页」；
6. 播放列表/标题/链接为空时使用锚点默认规则；
7. 过滤 `javascript:;` 排序按钮，并把多余的线路名截断到实际播放段数；
8. `Resolve` 识别播放页并从 `data-play`（`SXn`+base64）解出真实 m3u8/mp4，
   页面内直接出现媒体地址时也会兜底提取。
9. 详情页补回海报：优先用规则 `图片` 字段，缺失时读 `og:image`，相对地址转绝对。

## 已知限制

- `csp_XYQHiker`、其余 `csp_` 长尾未实现；
- XBPQ「筛选」目前只解析为规则模型，尚未接入分类筛选参数展开；
- `SearchMode` 只保留在规则模型中，M6.1 的搜索按 GET 处理；
- 详情页依赖规则字段；缺少 `播放数组` 时返回空剧集，而不是猜测网页结构；
- 播放页若通过更复杂的 JS 动态注入视频地址（非 `data-play`、页面也无明文媒体
  地址），M6.1 仍只返回播放页 URL，需依赖后续解码/嗅探；
- 站点反爬、Cookie 之外的挑战（如 JavaScript 计算、WebView 输入）不在 M6.1 范围。
