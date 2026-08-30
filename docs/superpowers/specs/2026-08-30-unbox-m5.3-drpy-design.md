# UnBox M5.3 设计：dr_py 方言适配（`var rule`）

日期：2026-08-30
状态：待评审
里程碑：M5.3（可选，核心子集）

## 1. 目标与范围

在 M5.1（FongMi js0）基础上，适配 dr_py 的 `var rule` 声明式爬虫方言，
让独立的 dr_py `.js` 站点也能本地运行（浏览 / 搜索 / 详情 / 播放）。

**MVP 范围（已定）**：`class_parse` + `searchUrl`（`**` 占位）+ `url`
（`fyclass`/`fypage` 占位）+ `muban` 模板覆盖 + `json:`/`js:` 内联规则 +
`lazy` 懒播 + gbk 解码。
**后置**：`filter` 筛选、crypto 加密。

## 2. 现状（实测）

dr_py（`hjdhnx/dr_py`，188 个 `.js`）是 `var rule` 声明式，比 FongMi js0 重一个量级。
采样 36/188：100% `rule`、86% `filter`、83% `lazy`、61% `class_parse`、33% `muban`、
0% `export default`。核心字段语义：

| 字段 | 语义 | 示例 |
|---|---|---|
| `title` / `host` | 站名 / 基准域名 | `host: 'https://www.139ys.com'` |
| `url` | 首页/分类 URL 模板，`fyclass`=分类 id、`fypage`=页码 | `/vodshow/fyclass--------fypage---.html` |
| `searchUrl` | 搜索 URL，`**`=关键词、`fypage`=页码 | `/search.php?searchword=**&page=fypage` |
| `class_parse` | 分类提取（`选择器;name;id;id正则`，用 `&&` 链） | `.nav-list&&li:lt(5);a&&Text;a&&href;/(\w+).html` |
| `muban.<模板>.<层级>.<字段>` | 模板选择器覆盖 | `muban.首图2.二级.desc = '.data:eq(0)&&Text'` |
| `一级`/`二级`/`搜索`/`推荐` | 内联提取规则（`json:` / `js:`） | `一级:'json:data.movies;title;cover;id;description'` |
| `lazy` | 播放地址解析 JS 片段 | `lazy:'js:input=input.split("?")[0]'` |
| `headers` / `timeout` | 请求头 / 超时 | `headers:{'User-Agent':'MOBILE_UA'}` |

## 3. 架构

复用 M5.1 已就位的 `req`（HTTP）、`rule`（`pdfh`/`pdfa`/`pd` 与 `&&` 链）、
`helpers`（base64/md5/log）。新增 Go 侧 dr_py 规则解释器 + goja 跑 `js:`/`lazy` 片段。
**不捆绑 `drpy.min.js`**——那要 cheerio/crypto/pako/ESM，goja 跑不了（M5 已证）。

| 文件 | 职责 |
|---|---|
| `drpy.go` | dr_py `rule` 解释：字段读取、URL 模板、动作分发（复用 M5.1 的 `VodHome` 等入口） |
| `muban.go` | `muban` 动态对象（自动建中间对象）+ Load 后读回覆盖值 |
| `classparse.go` | `class_parse` 四段解析（分类） |
| `inline.go` | `json:` / `js:` 内联规则解释（列表/详情/搜索） |
| `lazy.go` | `lazy` 播放解析 |
| `types.go`（改） | `Rule` 增补 dr_py 字段 |
| `helpers.go`（改） | 注入 `fetch`/`urljoin2`/`buildUrl`/`urlDeal`；gbk 解码 |

依赖：`golang.org/x/text`（GBK，已是间接依赖，转直接）。

## 4. 关键设计点

### 4.1 `muban` 动态对象

dr_py 爬虫写 `muban.首图2.二级.desc = '...'`，要求 `muban.首图2.二级` 已存在。
做法：`New()` 时用 `vm.NewDynamicObject` 装一个自动建中间对象的 handler，让任意
深层赋值不报错；`Load` 后 `Export` 读回覆盖值，与「内置默认模板」合并。
内置默认只给最小集（列表卡 `.item, .module-item, .stui-vodlist__box` 等 M5.1 已用的兜底），
覆盖优先。

### 4.2 `class_parse`

`选择器;name;id;id正则` 四段：`选择器` 定位条目（`&&` 链），`name`/`id` 各自用
`&&` 链从条目里取，`id正则` 对 id 值做一次正则提取。复用 `rule.go` 的 `evalRule`。

### 4.3 URL 模板

- `url`：`fyclass`→分类 id、`fypage`→页码。
- `searchUrl`：`**`→关键词（URL 编码）、`fypage`→页码。
- `detailUrl`/`playUrl`：`fyid`/`{id}`→影片 id。

### 4.4 `json:` 规则

`json:<路径>;<字段1>;<字段2>;…`：`<路径>` 用 `.` 逐级导航到数组；字段支持
`a||b`（前者优先）、`a+b`（拼接）。字段名 → Vod 字段映射：`title`→VodName、
`cover`→VodPic、`id`→VodID、`description`→VodContent、`cat_name`/`type_name`→TypeName 等。

### 4.5 `js:` 规则与 `lazy`

`js:`/`lazy` 是任意 JS 片段，在 goja 里跑，注入白名单全局：`input`、`fetch`/`request`
（=req）、`VOD`、`urljoin2`/`buildUrl`/`urlDeal`、`log`/`print`。`js:` 详情片段通过
`VOD` 输出结果；`lazy` 片段吃 `input` 产出 `{url, parse, …}`。

### 4.6 gbk 解码

dr_py 站大量 gbk 编码。`req` 响应用 `x/text` 的 `simplifiedchinese.GBK` 解码（按
Content-Type 或首字节探测），保证中文标题/简介不乱码。

## 5. 集成

`template.go` 现有 `VodHome`/`VodCategory`/`VodSearch`/`VodDetail`/`VodPlay` 的
「声明式 rule 降级路径」目前读的是简化字段（`class_name`/`class_url`/`vod_selector`）。
M5.3 把它**替换**为真实的 dr_py 解释（`class_parse` + `url`/`searchUrl` 模板 +
`一级`/`二级`/`搜索` + `lazy`），FongMi js0 路径不动。

## 6. 风险与待验证

| 风险 | 应对 |
|---|---|
| `muban` 内置模板全集未知（只知覆盖点） | 最小内置集 + 覆盖优先；用真实爬虫校准 |
| `json:` 路径/字段映射细节未钉死 | 验收即用真实爬虫校准（360影视.js 等） |
| `js:` 片段依赖 Node 专有 API | 遇不兼容报错降级；白名单注入 |
| gbk 探测误判 | 按 Content-Type 优先、首字节兜底 |
