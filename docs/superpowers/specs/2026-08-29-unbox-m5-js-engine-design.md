# UnBox M5 设计：JS 引擎 + 爬虫运行时（goja）

日期：2026-08-29
状态：待评审
里程碑：M5（拆 M5.1 JS 爬虫 / M5.2 JAR 爬虫 / M5.3 可选 XPath）

## 1. 目标与范围

在桌面端用纯 Go JS 引擎（goja）直接运行 TVBox 爬虫 JS，解锁 `csp_` JAR 与
`.js` 爬虫站点，让 Tvbox 多线路源能浏览、搜索、详情、播放。

- **M5.1（本设计主目标）**：JS 爬虫 —— goja 运行时 + 核心爬虫 API + 规则引擎 +
  drpy 模板 + `Spider` Provider。验收：跑通一个真实 drpy2 爬虫（首页/分类/详情/播放）。
- **M5.2**：JAR（`csp_`）—— JAR 解包取 JS，复用 M5.1 运行时。
- **M5.3（可选）**：XPath（type=0），视需求再定。

## 2. 现状与差距

`internal/config/classify.go` 现状：

| site.type | api 形态 | 判定 | 实现 |
|---|---|---|---|
| 1 CMS | 任意 | 支持 | ✅ `tvbox.Provider` |
| 3 spider | `http` 前缀 | 支持 | ✅ `tvbox.Drpy`（drpy 服务客户端） |
| 3 spider | `.js` 后缀 | 支持 | ❌ 无实现（**谎报可用**） |
| 3 spider | `csp_` 前缀 | 不支持 | ❌（Tvbox 多线路站点大头） |
| 0 xpath | 任意 | 不支持 | ❌ |
| 4 remote | 任意 | 待定 | 外挂进程 |

Tvbox 多线路源站点的绝大多数是 `csp_` JAR（type=3）+ xpath（type=0），
故当前只显示 1 个 CMS 站「魔都资源」。

## 3. 关键认知

`csp_` JAR 爬虫内部**本质仍是 JS 爬虫**——JAR 是一个 zip 包，内藏爬虫 JS +
配置；`site.api` 里的 `csp_XXX` 是 JAR 内某个爬虫的名字，真实下载地址在
配置**顶层 `jar` 字段**（当前 `Config` 模型未捕获，需在 M5.2 补 `Jar` 字段）。因此：

- `.js` 爬虫 = 下载 JS 直接跑（只用「运行时 + 规则引擎 + Provider」）。
- `csp_` JAR = 解包取 JS 后再跑（额外多一层「JAR 解包」）。

先做 JS 运行时，JAR 只是在其上叠加解包层。

## 4. 架构

新增两个单元，不破坏现有：

### 4.1 `internal/crawler`（JS 运行时，不 import provider / Wails）

| 文件 | 职责 |
|---|---|
| `engine.go` | goja 运行时封装：创建 VM、注入全局函数、执行预算与超时 |
| `req.go` | `req(url, opts)` HTTP 封装（GET/POST、header/cookie、重定向、UA、超时） |
| `rule.go` | `pdfh`/`pdfa`/`pd` 规则引擎（goquery 选择器 + `&&` 链 + 特殊方法） |
| `helpers.go` | `log`、`base64`、`md5`、`cookie`、`header`、`sleep`、`env` 等 |
| `template.go` | drpy `rule` 对象模板解释（home/category/search/detail/play） |
| `jar.go` | JAR（zip）解包 + `csp_XXX` → JS 文件映射（**M5.2**） |

依赖新增：`github.com/dop251/goja`（纯 Go JS 引擎）、
`github.com/PuerkitoBio/goquery`（jQuery 式选择器，封装 `x/net/html`）。

### 4.2 `internal/provider/tvbox/spider.go`

`Spider` Provider，实现 `provider.Provider`（ID/Home/Browse/Search/Detail/Resolve），
内部持有一个 `crawler` 实例，把爬虫输出映射为 `provider.Media`/`Item`/`Episode`，
并复用现有 `cmsVideo` 的 `vod_*` 解析与 `epID` 编码（与 `Drpy` 同构）。

## 5. 爬虫 JS API（M5.1 核心 + 常用 helper）

注入到 goja VM 的全局函数（签名与 drpy2/drpyS 对齐）：

```js
// HTTP 请求，返回 {content, headers, statusCode, finalUrl}
req(url, {method, headers, data, timeout})

// 解析（详见 §6 规则引擎）
pdfh(html, rule) -> string          // 第一个匹配
pdfa(html, rule) -> []string        // 全部匹配
pd(html, rule, join) -> string      // 全部匹配 + join

// 常用 helper
log(...args)
base64Encode(s) / base64Decode(s)
md5(s)
// cookie 容器（req 自动带/存）、自定义 header 注入、sleep(ms)
```

**不实现**（后续按需）：AES/DES 等加密库、图片验证码、特殊协议爬虫。

## 6. 规则引擎（pdfh / pdfa / pd）

`rule` 是 `&&` 分隔的选择器链，每段是 CSS 选择器或特殊方法：

```
body&&.list&&a&&href          → 取第一个 <a> 的 href
.item&&Text()                 → 取文本
.item&&a&&attr('data-id')     → 取自定义属性
```

- CSS 选择器：goquery（cascadia）。
- 特殊方法（首期）：`Text()`、`Html()`、`href`、`src`、`attr(x)`、`Array()`、
  `match(re)`、`split(sep)`、`trim`/`ltrim`/`rtrim`、`replace(a,b)`、`substring(a,b)`。
- 完整方法集需在 M5.1 验收时用真实爬虫校准补齐。

## 7. drpy rule 对象与模板

drpy2/drpyS 爬虫 JS 的产物是一个声明式 `rule` 对象（含 `host`、`searchUrl`、
`detailUrl`、`playUrl`、`class_name`/`class_url`、各选择器字段等）。运行时需
一个「模板」把 `rule` 解释成 home/category/search/detail/play 五个动作。

- **首选用 Go 实现模板**（`template.go`）：`rule` 语义稳定、可单测。
- 保留 imperative 函数覆盖：crawler 若自定义了 `homeVod()`/`search()` 等函数，
  直接调用其返回值，不套模板。

## 8. Spider Provider 集成

- `collectVodSites` 里把 classify 为 `js` 的站点改用 `Spider` 构造（修掉
  「js 谎报可用」）；`csp_`（jar）留到 M5.2。
- `Spider.Resolve` 需处理 drpy 的「懒加载播放地址」：部分 crawler 的
  `playContent` 才返回真实 URL，详情页只给 `vod_play_url`（`$`/`#` 分隔），
  可能需要二次调用。此点在 M5.1 验收时钉死。

## 9. 安全

- goja 默认沙箱：无 `require`/文件/进程访问，只暴露 §5 的白名单函数。
- 网络仅经 `req()`，带超时；对 crawler 的任意 URL 不做额外限制（与用户自行导入源
  同责），但 `req` 强制设置 UA、限制响应体大小。
- 执行预算：单个动作超时（如 30s）+ 响应体上限（如 16MB），防止爬虫死循环拖垮应用。

## 10. 测试

- `rule.go`：选择器链、各特殊方法单测。
- `req.go`：`httptest` 测 GET/POST/header/cookie/重定向/超时。
- 一个**合规脱敏后的真实 drpy2 爬虫 fixture**：端到端跑 home/category/search/
  detail/play，断言返回结构。
- `spider.go`：映射到 `provider.Media`/`Item`/`Episode` 的单测。

## 11. 里程碑拆分

### M5.1 JS 爬虫（本设计主目标）

1. `engine.go` + `req.go`：goja VM + `req` 注入。
2. `rule.go`：`pdfh`/`pdfa`/`pd` 规则引擎。
3. `helpers.go`：`log`/`base64`/`md5`/`cookie`/`header`。
4. `template.go`：drpy `rule` 模板。
5. `spider.go` + 集成：classify `js` → `Spider`。
6. **验收**：跑通真实 drpy2 爬虫（首页分类 / 列表 / 详情 / 播放地址）。

### M5.2 JAR（csp_）

1. 前置 spike：下载真实 `csp_` JAR，摸清结构（zip 内 JS 位置、`csp_XXX` → 文件映射）。
2. `Config` 模型补 `Jar` 字段 + `resolve.go` 透传。
3. `jar.go`：解包 + 映射。
4. classify `jar` → `SupportMaybe`。
5. **验收**：跑通真实 `csp_` JAR 站点的爬虫。

### M5.3（可选）XPath（type=0）

视 Tvbox 多线路源里 xpath 站点的实际占比再定。

## 12. 风险与待验证

| 风险 | 应对 |
|---|---|
| `csp_` JAR 真实结构未明（JS 位置、`csp_XXX` 映射） | M5.2 前置 spike，用真实 JAR 验证 |
| drpy2 `rule` 对象语义、`pdfh` 特殊方法全集未钉死 | M5.1 验收即用真实爬虫校准 |
| goja 与爬虫 JS 兼容性（Node 专有 API） | 遇不兼容 API 加 shim 或降级支持 |
| 懒加载播放地址（playContent） | M5.1 验收钉死，Resolve 二次调用 |
