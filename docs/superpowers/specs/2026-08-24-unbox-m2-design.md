# Unbox M2 设计文档：TVBox 点播（CMS JSON）

日期：2026-08-24
状态：待评审
里程碑：M2（M1 已交付）

---

## 1. 项目定位

M2 在 M1（直播 + 壳 + mpv 播放）之上，新增 TVBox 点播（VOD）能力。首期只做
**CMS JSON（type=1）站点**，打通「站点 → 分类 → 影片列表 → 详情（剧集）→ 播放」
全链路；JS 爬虫（type=3 drpy2/hipy）作为 M2 第二阶段单独评估，不在本文档范围。

**明确不做（继承 M1 决策）**

- JAR 爬虫（`csp_*`，ARM native 加壳，桌面端不可行）
- Python 爬虫（需额外运行时，冲击"安装即用"）
- JS 爬虫（本文档留作 M2.5，需先解决 JS 运行时选型：goja 纯 Go vs 打包 node）

---

## 2. 调研结论

### 2.1 CMS JSON 协议（苹果CMS v10 `provide/vod`，已实测核实）

TVBox `type=1`（CMS）站点指向苹果CMS v10 的 JSON API，`site.api` 为完整基地址，
形如 `http://域名/api.php/provide/vod/`。动作经 `ac` 参数区分。**已对用户订阅中的
真实站点（非凡资源 / 量子资源）实测抓取并确认如下事实。**

| 动作 | 请求 | 返回关键字段 |
|---|---|---|
| 列表（最新） | `?ac=videolist&pg=<页>` | `list[]`：`vod_id` / `vod_name` / `vod_pic` / `type_id` / `type_name` / `vod_remarks` / `vod_year`；`pagecount` / `total` |
| 列表（按分类） | `?ac=videolist&t=<type_id>&pg=<页>` | 同上，仅含该分类 |
| 搜索 | `?ac=videolist&wd=<关键词>&pg=<页>` | 同上 |
| 详情 | `?ac=detail&ids=<vod_id>` | `list[0]`：`vod_content` / `vod_play_from` / `vod_play_url` / `vod_area` 等 |

**实测要点（与常见文档不一致处，以实测为准）**

1. **没有独立分类端点**：`?ac=list` / `?ac=class` 实测返回的都是**视频列表**（最新），
   不是 `class[]`。分类须从列表项的 `type_id` / `type_name` **去重派生**。
2. **列表项可能内嵌完整详情**：部分站点（非凡资源）的列表项已含 `vod_play_url` /
   `vod_content`（单页 370KB）；另一部分（量子资源）列表轻量、需单独 `ac=detail`。
   两种都要兼容：`Browse` 只用轻量字段，`Detail` 始终走 `ac=detail`。
3. **`vod_play_from` 分隔符不一致**：列表项里用 `,`（仅展示用），详情里用 `$$$`。
   解析器对两种都要处理。
4. **`limit` 参数被忽略**：实测站点固定每页 20 条，忽略 `limit`。

剧集结构由 `vod_play_from`（线路名）与 `vod_play_url`（集 + 地址）共同表达，
**实测确认**的分隔符：

- 线路分隔：`$$$`（如 `feifan$$$ffm3u8` = 2 条线路）。
- 集分隔：`#`；每集为 `剧集名$播放地址`（`$` 取第一个）。
- 即 `vod_play_url` = `第01集$url#第02集$url$$$第01集$url#第02集$url`。

**结论**：纯 HTTP + JSON，零运行时依赖，符合"安装即用"。分隔符、分类派生等
细节均以 `testdata/cms/` 真实 fixture 锁定（非凡资源：`list.json` / `detail.json` /
`search.json`）。

### 2.2 站点存量

用户订阅（某Tvbox多线路）实测 8 个 CMS 站点（type=1）。M2 用其中站点作真实测试夹具；
源派系（JAR 占 84%）不影响本里程碑，CMS 站点的协议是标准的。

---

## 3. 架构

### 3.1 核心抽象（对 M1 的增量扩展）

M1 的两个接口（`Provider` / `Player`）不变，仅 `provider.Media` 增补点播字段，
并新增 `Episode` 类型。`Resolve(ctx, id)` 签名保持不变 —— 直播传频道 id，
点播传剧集 id。这样 `Player`、`failover`、shell 播放链路零改动复用。

```go
// provider 包新增
type Episode struct {
    ID     string  // 稳定标识，供 Resolve 定位
    Source string  // 线路名（来自 vod_play_from）
    Name   string  // 第N集
    URL    string  // 播放地址
}

// Media 增补（M1 的 ID/Title/Logo/Group 不动）
type Media struct {
    ID          string
    Title       string
    Logo        string
    Group       string
    Description string     // vod_content 简介
    Year        string     // vod_year
    Area        string     // vod_area
    Type        string     // type_name
    Remarks     string     // vod_remarks
    Sources     []string   // 线路名列表
    Episodes    []Episode  // 剧集列表
}
```

### 3.2 目录结构（新增 `internal/provider/tvbox`）

```
internal/provider/tvbox/
├── tvbox.go        Provider 实现（每站点一个实例）
├── cms.go          CMS JSON 客户端：list / videolist / detail / search
├── episodes.go     vod_play_from / vod_play_url 拆分（纯函数）
└── *_test.go       对应测试
```

### 3.3 数据流

```
导入订阅 → shell 收集 cfg.Sites（type==1）→ 每个站点 tvbox.New(site)
   │
前端「点播」tab
   ├─ Sources()            → [直播, 站点1, 站点2, ...]
   ├─ Categories(siteKey)  → tvbox.Home()   → ac=videolist → 去重 type_id/type_name 派生分类
   ├─ Browse(siteKey,cat,page) → tvbox.Browse() → ac=videolist&t=<cat> → 影片列表
   ├─ Search(siteKey,q)    → tvbox.Search() → ac=videolist&wd=q
   ├─ Detail(siteKey,id)   → tvbox.Detail() → ac=detail → 详情+剧集
   └─ Play(siteKey,epID)   → tvbox.Resolve(epID) → Stream{URL, Referer} → mpv
```

### 3.4 tvbox Provider（每站点一个实例）

- `New(site config.Site) *Provider`：持有 `site.API`、站点名、key，内部一个
  最近 detail 缓存（上限 64，`vod_id → 已拆分剧集`）。
- `ID()` 返回站点 key（唯一）。
- `Home()` → `ac=videolist` 取最新列表，去重 `type_id`/`type_name` 派生分类。
- `Browse(cat, page)` → `ac=videolist&t=<cat>&pg=<page>`，映射到 `provider.Page`。
- `Search(q)` → `ac=videolist&wd=<q>`。
- `Detail(id)` → `ac=detail&ids=<id>`，拆分剧集并缓存，返回扩展 `Media`。
- `Resolve(epID)` → 从缓存反查该剧集的 URL，返回
  `player.Stream{URL, Headers: {Referer: 站点 origin}, Kind: 按扩展名}`。
  缓存未命中则按 epID 里的 `vod_id` 重取 detail 再查。

**epID 编码**：`<vod_id>/<sourceIdx>/<epIdx>`（三整段，`/` 分隔）。每个 tvbox
Provider 只处理自己站点的 epID，故无需在 id 内嵌站点 key。

### 3.5 壳层（多源）

`ShellService` 从「单 provider」改为「直播 + 多站点」：

```go
type ShellService struct {
    player player.Player
    store  *store.Store
    live   provider.Provider             // 直播（M1 不变）
    vods   map[string]provider.Provider  // 站点 key → tvbox.Provider
    mu     sync.RWMutex
}
```

`ImportSubscription` 在收集 `lives` 之外，同时收集 `cfg.Sites` 中 `type==1` 的
站点，逐个 `tvbox.New`。新增服务方法（显式带 source 参数，直播旧方法保留兼容）：

- `Sources() []SourceInfo` —— 顶层来源：直播 + 各站点（id/name/kind）。
- `Categories(sourceID) []Section` —— 直播=分组，点播=分类。
- `Browse(sourceID, cat, page) []Item`
- `Search(sourceID, q) []Item`
- `Detail(sourceID, id) Media` —— 点播返回含剧集的详情。
- `Play(sourceID, id) error` —— resolve + load + play（复用现有播放逻辑）。

### 3.6 前端

顶层加「直播 / 点播」切换。点播面板：站点下拉 → 分类侧栏 → 影片列表 → 详情
（简介 + 线路选择 + 剧集按钮）→ 播放。复用现有播放控制与错误提示。

---

## 4. 功能范围

**包含**

1. 导入订阅时收集 `type==1` 站点并构建 `tvbox.Provider`。
2. 点播浏览：站点列表 / 分类 / 影片列表（分页）/ 搜索。
3. 影片详情：简介 + 线路 + 剧集。
4. 剧集播放：resolve URL + Referer + mpv 播放。

**不包含（顺延）**

- JS 爬虫站点（M2.5）。
- 嗅探 / VIP 解析 / 广告分片过滤。
- 点播收藏 / 观看进度（现有 store 只面向直播，点播适配留后续）。
- 首页推荐（`?ac=videolist` 无分类的"最新"可作为分类之一，不单独做推荐位）。

---

## 5. 测试策略

沿用 M1「真实样本定正确性」：

- **Task 1 先抓 fixture**：重新拉取某Tvbox多线路订阅、解出 type=1 站点，抓其
  `ac=list` / `ac=videolist` / `ac=detail` 真实响应存入 `testdata/cms/`，并记录
  该站点的 `api` 基地址。
- `episodes.go`（分隔符拆分）为纯函数，用真实 fixture + 构造样本覆盖
  `$$$`/`,` 两种线路分隔、`#` 集分隔、`$` 名址分隔、缺 URL 等边界。
- `cms.go` 的 HTTP 客户端用 `httptest.Server` 打桩（不进真网），按协议返回
  构造 JSON 验证字段映射与错误传播。
- 壳层新增方法用内存 Provider 桩测路由（source 分发、nil/未知 source 报错）。
- 全量 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 绿。

---

## 6. 验收标准

- [ ] `go test ./... -count=1` 全绿；`go vet` / `gofmt` 干净。
- [ ] `testdata/cms/` 至少一个真实站点的 3 类响应入库，解析测试通过。
- [ ] 导入某Tvbox多线路订阅后，前端「点播」tab 能看到 8 个 CMS 站点。
- [ ] 进入某站点 → 出分类 → 出影片列表 → 点开详情见剧集 → 点剧集出画面。
- [ ] 直播功能（M1）不受影响（回归测试全绿）。

---

## 7. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 剧集分隔符生态差异（`$$$` vs `,`） | 解析错位 | 真实 fixture 锁定 + 双兼容 + 纯函数测试 |
| CMS 站点防盗链（需 Referer） | 播放失败 | Resolve 时带站点 origin 作 Referer；`config.Fetcher` 已有 UA 伪装 |
| 站点可用率低（用户源仅 8 个 CMS 站） | 演示价值有限 | 协议标准，源派系问题非技术问题；留 JS 爬虫（M2.5）补生态 |
| 站点 API 不稳定 / 失效 | 点播时好时坏 | 错误显式回传前端；不阻断直播 |
