# js/drpy2 冒烟 67 条「疑似引擎 bug」逐条核对结论

**日期**：2026-11-09
**范围**：`TestSmokeJS`（305 站全量）汇总 可播=6 / 站点侧=144 / 引擎bug=67
**方法**：67 条全部提取 → 按错误模式聚合（7 类）→ 逐类实跑引擎/curl 站点头验证 → 真 bug 修 + 回归测试。

---

## 总结论

**67 条 = 5 个真引擎 bug（已修 4 个 + 1 个分类器误报）+ 62 条站点侧误报。**

真引擎 bug 修复后，归属「引擎 bug」桶的真实缺陷数应从 67 降到茶壶个位数（余者为「测试分类器漏判网络错误」）。

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