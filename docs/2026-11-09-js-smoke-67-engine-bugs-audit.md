# js/drpy2 冒烟 67 条「疑似引擎 bug」逐条核对结论

**日期**：2026-11-09
**范围**：`TestSmokeJS`（305 站全量）汇总 可播=6 / 站点侧=144 / 引擎bug=67
**方法**：67 条全部提取 → 按错误模式聚合（7 类）→ 逐类实跑引擎/curl 站点头验证 → 真 bug 修 + 回归测试。

---

## 总结论

**67 条 = 10 个真引擎 bug（全部已修）+ 1 个分类器误报（已修）+ 56 条站点侧误报。**

两轮收敛：修复+分类器修正后重跑，engine 桶 **67 → 23 → 16**，可播 **6 → 7 → 12**。
最终 16 条引擎桶残留全部为站点侧（11 条 agit.ai Cloudflare 反爬、69美剧/迷你库接口改版、
路飞混淆 TypeError、哔哩 -352 风控、KTV clun.top 脚本失效），无一新增引擎缺陷。
芒果系 5 站从失败跃升至可播（jsp/setResult/playPartId/host编码/filter 修复的直接成果）。

真引擎 bug 清单（按修复 commit）：
- commit `72d9beb5`：① 整数 id 科学计数法 ② 详情数字赋字符串字段 ③ JSONP `_callback()` 未剥壳 ④ filter `or 默认值` 未展开 ⑤ host 段 `%XX` 编码解析失败
- commit `00810e34`：⑥ 冒烟分类器 `looksNetErr` 漏网络关键词（eof/connection/502…）→ ~54 条 EOF 误报
- commit `a15ae144`：⑦ `jsp` 选择器 shim 缺失 ⑧ `setResult` 原语缺失 ⑨ `playPartId` 未识别为 id 字段
- commit `e27712cb`：⑩ JSONP 尾部 `;` 需容忍 + `vid` id 字段别名（网易公版专用）

## 分组

### A. 分类 EOF 网络错误（~54 条）→ 误报，真因是分类器缺陷
错误形态：`分类 GoError: req 请求失败: Get "...": EOF`

**根因**：冒烟测试在**分类**阶段调 `looksNetErr(cerr)`，它只认 `timeout/超时/http /连接失败/tls/404/403/deadline/无法解析` **9 个关键词**；而其他阶段用的 `classifyJsError` 有 `eof/connection/502/522/请求失败/返回 http` 等更多词。分类阶段的 **EOF / stream error / connection** 错误全部落进引擎桶。

**证据**：
- `ddys.pro`、`pg.fenwe078.cf`、`yu.akgj.com` 等 curl 全部是 Cloudflare `/lander` 反爬页或直连 EOF（batch-check 脚本 49 URL 实测）。
- 多个站（爱弹幕 `sto…`/兄弟影视 `PROTOCOL_ERROR`）是 http2 stream 层网络错误。

**处置**：非引擎 bug。应把 `looksNetErr` 换成 `classifyJsError`（或补齐关键词）——这是**测试分类器 bug**，见第 5 项。

### B. 整数 id 科学计数法（博看听书等）→ 真引擎 bug #1 ✅ 已修
错误：`详情 js 详情规则执行失败: TypeError: Cannot read property 'list' of undefined`
根因：列表 `json:…;id` 字段值 `3331822` 经 `jsonScalarString` 的 `%v` 变 `3.331822e+06` → `VodID="3.331822e+06"` → 详情 URL `album_id=3.331822e+06` → 站点「集合不存在」→ `data.list` undefined。

修复：`inline.go jsonScalarString` 对整数 float64 用 `strconv.FormatInt` 十进制输出。测试 `inline_idint_test.go`。博看听书整链验证通过（32 集 m4a）。

### C. 详情数字赋字符串字段 → 真引擎 bug #2 ✅ 已修
错误：`爬虫详情返回格式错误: json: cannot unmarshal number into Go struct field Detail.vod_name of type string`
根因：二级 js 脚本 `VOD.vod_name = data.list[0].id`（数字 5690048）→ JSON 里数字 → `json.Unmarshal` 到 string 字段失败。

修复：`lenientDetailFromJSON`（`coerceDetailNumbers` 递归把 JSON 数字转字符串再解码）。测试 `exportdetail_num_test.go`。

### D. JSONP 响应未剥壳（网易公版影像/聚合网易公版）→ 真引擎 bug #3 ✅ 已修
错误：`列表 json 列表响应解析失败: invalid character '_'`
根因：`active.163.com` 的 `.jsonp` 接口返回 `_callback({...})`（实测 25KB），引擎按纯 JSON 解析。

修复：`stripJSONP` 剥壳 + `cover_pic` 字段别名。测试 `jsonp_test.go`。

### E. filter_url 的 `or 默认值` 未展开（芒果/百忙无果）→ 真引擎 bug #4 ✅ 已修
错误：`列表 json 列表响应解析失败: unexpected end of JSON input`
根因：`filter_url:'year={{fl.year or "all"}}&sort=…'` 中 `{{fl.year or "all"}}` 不在 `flPlaceholderRe` 匹配 → 模板字面量原样发到站点 → 空 JSON。

修复：`renderFilterTemplate` 支持 jinja `or 默认值`。测试 `filter_or_test.go`。

### F. host 段 %XX 编码（芒果）→ 真引擎 bug #5 ✅ 已修
错误：（同 E 的`unexpected end`，但二次排查发现独立根因）
根因：`rule.Host="https://pianku.api.%6d%67%74%76.com"` 的 host 段 `%XX` 使 `url.Parse` 报 invalid URL escape → `joinURL` 退回相对路径 → `resolveRequestURL` 打到脚本基址（gh-proxy）→ 反爬 HTML。

修复：`joinURL` 失败时 `decodeHostEscapes` 解码 host 段重试。测试 `joinurl_host_test.go`。芒果整链 80 项验证通过。

### G. 路飞 newTitleBuckets TypeError（1 条）→ 脚本侧，非引擎 bug
错误：`列表 TypeError: Cannot read property 'newTitleBuckets' of undefined at _0x3c9801`
根因：`lf_search3_min.js` 混淆脚本读 `undefined.newTitleBuckets`，混淆过（源码无此字面量），依赖运行时登录态 → JS 自身运行时错误，非引擎缺全局。引擎已注入 UA 常量等全套 shim。

### H. 脚本加载失败（~13 条）→ 站点侧
`加载 下载爬虫失败: …返回了网页而不是 JS 脚本`：agit.ai 全部是 Cloudflare `/lander` 反爬页（实测 114B 跳转页），`clun.top/js/drpy2.js` 失效（返回 TVBox 主页 HTML）。引擎正确识别。

### I. 站点/接口已改版（~3 条）→ 站点侧
`站点返回了网页而不是 JSON`：迷你库[V2]、69美剧[V2] 接口实测返回 HTML（69美剧 `xgapp.php/v2/video` 已改版）。引擎正确报错。

### J. 芒果[js]/哔哩直播 接口间歇空（~2 条）→ 站点侧
`unexpected end`（修复后复验 3 次均 200+110KB，间歇性）/ `data.list 路径不存在 {"code":-352}`（B 站风控）。

---

## 待办项 #5：测试分类器 `looksNetErr` 关键词缺口

`internal/crawler/drpy_playback_test.go` 分类阶段用 `looksNetErr`（缺 `eof`/`connection`/`502`/`522`/`请求失败`），应统一改用 `classifyJsError`。此改动只影响测试分类精度，不影响运行时。

## 修复文件清单
- `internal/crawler/inline.go`：jsonScalarString 整数十进制 / stripJSONP / cover_pic 别名 / bytes import
- `internal/crawler/template.go`：lenientDetailFromJSON + coerceDetailNumbers / joinURL 编码 host 解码
- `internal/crawler/drpy.go`：renderFilterTemplate 支持 `or 默认值` + flTemplateRe/flOrDefaultRe
- 测试：`inline_idint_test.go`、`exportdetail_num_test.go`、`jsonp_test.go`、`filter_or_test.go`、`joinurl_host_test.go`

---

# 八仙源全量冒烟结论（追加）

**日期**：2026-11-09（同日）
**范围**：八仙源（`https://xn--cnq85d.52011.top/a?id=1`）四层导航全量冒烟。
**链路**：`a?id=1`（现 302 → 主仓库 `/nxog/d.php?b=八仙`，中文参数须 URL 编码）→ 22 条线路 `b.php?id=N` → 每条线路是 `\njhSPAyzn**`（11 字节）前缀 + **StdEncoding base64** 的站点配置 → `extractAllSites` + `expandSiteExt` 收集全部站点 → 去重后 1674 站逐站 Home → Browse/Search 冒烟。
**汇总（当前）**：**通过=13 / 站点侧=4 / 疑似引擎bug=13 / 引擎未实现=1644。**

## 一句话

**13 条引擎桶 = 1 个真引擎 bug（XBPQ JSONC 注释，已修）+ 12 条站点侧。**

修复前第一次全量（ext 遗漏 + JSONC 未支持 + 分类器缺内联识别）：疑似引擎 34 → 修复后 13，通过 0 → 13。

## 真引擎 bug（1 条，已修）

### K. ParseXBPQ 不支持 JSONC 注释（养生堂系 2 站）→ 真 bug ✅ 已修
错误：`解析 XBPQ 规则失败: invalid character '/' looking for beginning of object key string`
站：`🪆养生｜知识学堂`（wget.la 外链）、`🧿养生｜知识`（jsdmirror 外链）——同一份 `养生堂.json`。
根因：`ParseXBPQ` 只做 `sanitizeJSONEscapes`，没有像 `ParseXYQ` 那样先 `stripJSONCComments`。真实规则文件里夹整行 `// "播放链接前缀": ...`，直接 `json.Unmarshal` 报 `invalid character '/'`。
修复：`ParseXBPQ` 先 `stripJSONCComments`（`rule.go`）。测试 `TestParseXBPQJSONC`（xbpq_test.go）。

## 引擎未实现形态（非 bug，归「未实现」桶）

八仙源给 `csp_XBPQ` 站两种**内联规则 ext**——TVBox 原版支持，UnBox 引擎只支持「ext 是规则文件 URL」一种形态：
- **对象形态**（`🧿┃农民┃影视`、`🧿VIP｜影视`、`🧿先生｜影视`）：`"ext": {"副标题":…,"请求头":"User-Agent$MOBILE_UA","编码":"UTF-8","分类url":"https://…;!H"}`。`Site.ExtString()` 对非字符串返回空 → 引擎拿到空 URL。
- **文本形态**（`樱花动漫`）：`"ext": "分类url:https://…,分类:日本动漫$…"` 逗号分隔文本，直接当 URL parse 失败。
处理：冒烟分类器把非 URL 形态 ext（`extIsRuleURL` 判断）计为「引擎未实现」，不混入 bug 桶。后续实现时按 `scriptExt` 的序列化先例把对象转规则 JSON。

## 站点侧（12 条，非 bug）

| 站 | 问题 |
|---|---|
| 🚑️养生知识 / 💎VIP / 韩剧看看 / 韩剧在线 / 巴士动漫 / 路漫 / 番茶 / AEG / 啊哈 / 巴士动漫`/ext/39.png` | 八仙自营 `/json/*.json`、`/XBPQ/*.json`、`/ext/39.png` 全部 **404**（统一返回门户 iframe 兜底页）。这些相对 ext 按线路 URL 展开后 404，带 Referer/不同 UA 亦 404——八仙自营规则文件已失效/路径不对。同款站（VIP/养生堂）在其它线路给了 iyouhun/gh-proxy 绝对地址，可见多线路各自维护、自营链接多数过期。 |
| 🧲电影港湾 / 🎃电影磁力 | 规则文件 **200 但格式不匹配**：海阔 XPath 体系字段（`class_url`/`class_name`/`Coding_format`），XYQ 引擎只认 `分类链接/搜索链接/首页推荐链接` → 报「规则缺少 首页推荐链接/分类链接/搜索链接」。站点规则格式与引擎不同，非引擎缺陷。 |
| 动漫巴士 | ext 外链 `gh.con.sh` 现 302、GitHub 直链 404 → `invalid character 'S'`（代理/源文件失效）。 |

## 冒烟基础设施修复（非引擎 bug，但属测试正确性）

- `extractAllSites` 此前**不提取 ext 字段**（永远空串）→ csp_XBPQ/XYQHiker 站全部用空 URL 加载，34 条「疑似引擎」的根因。已补 `ext` 提取。
- `expandSiteExt`：相对 ext（`./XBPQ/养生堂.json`）按线路 URL `ResolveReference` 展开（带 `://` 原样留）。
- 分类器：内联规则 ext（对象/文本）归「引擎未实现」桶；非 `http(s)://` 前缀、非 `./`、非 `/` 开头一律视作非 URL。

## 遗留观察
- 八仙线路基址漂移：`a?id=1` 现 302 到 `/m/e.php`，主仓库在 `/nxog/d.php`；`TestSmokeBaxianFull` 已适应（正则找 `d.php` 相对引用）。主仓库中文参数必须 `encodeQuery`（`b=八仙`）否则 HTTP 400。
- 站点计数因多线路重复注册不稳定（哆啦新番社在两次 run 中通过/失败翻转），根因是 jsdmirror 间歇慢速而非引擎。