# M7 drpy2 / ESM 真实站点验收（2026-10-17）

验收对象：M7.1（drpy2 选源）与 M7.2（ES Module 加载器）。
代码基线：`feat/m7-drpy2-esm`，验收前 HEAD `3c810c98`。
环境：WSL2 Linux 6.18.54.1（microsoft-standard-WSL2），Go 1.26.3 linux/amd64，
CGO 开启；宿主机**缺少 GTK4 / WebKitGTK 开发包**（`pkg-config --exists gtk4`、
`pkg-config --exists webkitgtk-6.0` 均为 false）。

> **后续状态（2026-10-31）**：本文件是 10-17 那次验收的原始记录，保留不改。当时的
> 缺口后续都已处理：`fyfilter`/`filter_url` 展开、`cat.js`/`gbk.js` 自有 shim、
> `某影视站.js` 的 id `$` 语义与规则脚本作用域（10-30 修复后搜索→详情→剧集链路跑通）。
> 仍缺 `setInterval` 注入与上游未声明许可证的 `模板.js`。现状见 `docs/HANDOFF.md`
> 「本轮大扫除」。

结论：**M7.1/M7.2 的代码路径（选源、ESM 改写与加载）已实现并通过单元/端到端测试，
但三条真实站点验收均未完整跑通分类 → 列表 → 详情 → 播放**。三个失败各有明确根因，
均记录在下方，未修改验收测试、未伪造成功。

> **Task 10 后续修复（2026-10-17）**：上述三处失败中的两处已在 Task 10 修复——
> `console` shim（`某短剧站.js` 恢复分类）与 `maskNonCode` 的「正则字面量 + 嵌套
> 模板字面量 + 无空格 `export default{`」识别（`lf_search3_min.js` 恢复编译加载）。
> 修复后两条脚本均能加载并进入分类阶段，但仍有新的运行时缺口（`setTimeout`）与
> 既有 `fyfilter` 缺口，详见 2.2/2.3 的更新结果与第 4 节待办。未声称完整验收。
>
> **Task 11 后续修复（2026-10-17）**：注入同步、等待上限 1000ms 的 `setTimeout`
> shim 与 no-op `clearTimeout`。`lf_search3_min.js` 不再因 `setTimeout` 中断；分类
> 阶段现在卡在脚本自身对 `https://www.netflix.com/tudum/top10` 的请求——脚本把批量
> 请求超时设为 3000ms，而本环境实测该 URL 需约 4.8–5.0s。该阻塞属网络/站点延迟，
> 不是引擎缺口；`fyfilter` 仍是缺口，仍未声称完整 M7 验收。
>
> **Task 12 最终 review 修复波（2026-10-17）**：
> - `export async function ...` 现能正确改写（新增 `TestRewriteModuleExportAsyncFunction`、
>   `TestLoadESMExportAsyncFunction`），不再因 `Unexpected reserved word` 编译失败；
> - `export default { __jsEvalReturn(){...} }` 内嵌的 `__jsEvalReturn` 会被调用并提升，
>   覆盖 ESM 与非 ESM 两条路径（`TestLoadESMNestedJsEvalReturn*`）；
> - 内置 cheerio 从 FongMi 自定义 bundle 换成**官方 npm `cheerio@1.0.0-rc.12`**
>   （esbuild 0.25.0，ESM/minify，约 344 KB），并新增
>   `TestVendoredCheerioExecutes` / `TestVendoredCryptoJSExecutes` 直接执行真实
>   bundle；许可证（cheerio MIT + 打包依赖 MIT/BSD-2/ISC）见
>   `LICENSE.cheerio.txt` / `LICENSE.bundled-deps.txt`，并随 deb/rpm、NSIS、.app
>   分发；AppImage 与 Windows 便携 exe 为单文件格式，随发布产物提供
>   `*-licenses.tar.gz`（Linux 为 `UnBox-licenses.tar.gz`，Windows 为
>   `UnBox-<ver>-windows-<arch>-licenses.tar.gz`）；
> - drpy2 引擎库识别放宽（`drpy2.1.min.js`、`?v=`、`#`），ext 带 query 也能选中；
> - 真实脚本复跑结果与 Task 11 一致：`lf_search3_min.js` 仍卡在脚本自身的 3000ms
>   Netflix 请求超时；`某影视站.js` 仍卡在 `fyfilter`；`某短剧站.js` 仍能取 9 个分类、
>   列表返回 0 条（`filterOptions 解码失败`）。**M7 仍未完整验收。**

## 1. 验证命令

### 1.1 可运行包（排除 Wails 相关包）

因宿主缺少 GTK4/WebKitGTK，`internal/shell` 与 `cmd/unbox` 无法编译。对其余全部包执行：

```bash
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' \
  | xargs go test -count=1
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' | xargs go vet
gofmt -l .
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' \
  | grep -v '/build/windows/nsis$' | xargs env CGO_ENABLED=1 go build
```

结果：

| 命令 | 结果 |
|---|---|
| `go test`（非 Wails 包） | 全部 `ok`：`config`、`crawler`、`library`、`library/thumb`、`playback`、`player`、`player/failover`、`player/mpvplugin`、`player/mpvproc`、`probe`、`provider/live`、`provider/tvbox`、`store`、`cmd/unbox-scan`、`cmd/unbox-donors`、`build/windows/nsis`；`internal/provider` 无测试文件 |
| `go vet`（非 Wails 包） | 无输出，退出码 0 |
| `gofmt -l .` | 无输出 |
| `CGO_ENABLED=1 go build`（非 Wails 包） | 退出码 0 |

### 1.2 全量命令的限制（如实记录）

```bash
go test ./... -count=1
CGO_ENABLED=1 go build ./...
```

两者均在 `internal/shell` 失败，`pkg-config` 报告：

```
Package 'glib-2.0' was not found in the pkg-config search path.
Package 'gobject-2.0' not found
Package 'gtk4' not found
Package 'webkitgtk-6.0' not found
Package 'gio-unix-2.0' not found
Package 'libsoup-3.0' not found
```

这是宿主环境限制（未安装 Wails 的 Linux cgo 依赖），不是 M7 代码回归；按要求未安装
系统包。

### 1.3 Provider 选源与 ESM 端到端单测

```bash
go test ./internal/provider/tvbox -run 'TestSpiderScriptURL|TestSpiderESMJsEvalReturn' -count=1 -v
```

结果：两项均 PASS。

- `TestSpiderScriptURL` 覆盖 `api=.../drpy2.min.js` + `ext=.../某影视站.js` → 实际选择
  `ext`（含带 query 的 api、非 js ext、无 ext 等分支）。
- `TestSpiderESMJsEvalReturn` 用 `httptest` 覆盖 ESM `export function __jsEvalReturn`
  的加载与动作提升。

## 2. 真实站点验收结果

### 2.1 drpy2：`某影视站.js`（`var rule`）

```bash
UNBOX_REAL_JS_PATH=/tmp/drpy-research/scripts/drpy2_360.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

实测日志要点：

```
脚本形态: rule=true init=false homeContent=false ...
分类(4):
  - id="2" name="电视剧"
  - id="1" name="电影"
  - id="3" name="综艺"
  - id="4" name="动漫"
VodCategory("2") 失败: json 列表路径不存在: data.movies
```

- 加载与分类：**成功**（`var rule` 解析成功，4 个分类）。
- 列表/详情/播放：**未执行**（列表即失败）。

根因（既有 drpy2 方言缺口，不是 M7 选源问题）：脚本的列表模板是

```
url:'https://api.web.360kan.com/v1/filter/list?catid=fyclassfyfilter'
```

`fillURL` 只替换 `fyclass`/`fypage`/`**`，不替换 `fyfilter`，生成
`catid=2fyfilter`。实测该请求返回：

```json
{"data":null,"errno":1,"msg":"catid参数错误"}
```

而同一接口 `catid=2` 返回 `data.movies` 列表。即：**引擎尚未实现 drpy2 的
`fyfilter` / 分类筛选占位**（`filter` / `filter_url` 也未实现），含该占位的站点
无法取到一级列表。这属于 M7 之后需补的 drpy2 方言能力。

选源本身已由 `TestSpiderScriptURL` 验证：drpy2 站点加载的是 `ext`（`某影视站.js`），
不是 `api`（`drpy2.min.js`）。

### 2.2 ESM：`某短剧站.js`

```bash
UNBOX_REAL_JS_URL='https://clun.example.com/js/某短剧站.js' UNBOX_REAL_SEARCH=测试 \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

实测日志要点（Task 10 修复后，2026-10-17 复跑）：

```
脚本形态: rule=false init=true homeContent=false ...
[console.error]filterOptions 解码失败map[]
分类(9):
  - id="甜圈" name="甜圈短剧"
  - id="七猫" name="七猫短剧"
  - id="锦鲤" name="锦鲤短剧"
  - id="番茄" name="番茄短剧"
  - id="星芽" name="星芽短剧"
  - id="西饭" name="西饭短剧"
  - id="软鸭" name="软鸭短剧"
  - id="百度" name="百度短剧"
  - id="围观" name="围观短剧"
分类列表(0)（取前 3）:
搜索结果(0)（取前 3）:
分类列表为空，无法验证详情/播放
```

- 下载与 ESM 加载：**成功**（`init` 已提升为全局，crypto-js 导入成功）。
- 分类：**成功**（修复前 `console.error` 抛 `ReferenceError` 使分类中断；修复后
  `console` 注入，`getFilterOptions` 的 catch 日志正常输出，返回 9 个分类）。
- 列表/详情/播放：**未执行**（`VodCategory` 返回 0 条；日志显示
  `filterOptions 解码失败`，即分类筛选参数解码为空，列表接口拿不到数据——
  属于新的真实站点问题，不是 console 缺失）。

根因（已修复）：脚本在 `getFilterOptions` 的 catch 中调用 `console.error(...)`，
goja 运行时未注入 `console`，异常处理路径本身再抛 `ReferenceError` 导致分类中断。
Task 10 注入最小 `console` shim（`log/info/warn/error/debug` 映射到日志）。

### 2.3 ESM：`lf_search3_min.js`（混淆 import/export）

首选按 URL 运行：

```bash
UNBOX_REAL_JS_URL='https://raw.example.com/某规则仓库/gao/master/lib/lf_search3_min.js' \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

首次及重试均为：

```
下载/加载爬虫失败: 下载爬虫失败: Get ".../lf_search3_min.js": net/http: TLS handshake timeout
```

该地址用 `curl` 可正常访问（HTTP 200，363687 字节）；Go 客户端在用 HTTP/2 时
TLS 握手超时，关闭 HTTP/2 后同一 URL 可下载：

```bash
GODEBUG=http2client=0 UNBOX_REAL_JS_URL='https://raw.example.com/某规则仓库/gao/master/lib/lf_search3_min.js' \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

```
模块 https://raw.example.com/某规则仓库/gao/master/lib/lf_search3_min.js 编译失败:
SyntaxError: SyntaxError: (anonymous): Line 1:363339 Unexpected reserved word (and 5 more errors)
```

用与上述 URL 内容完全一致的本地副本（SHA-256 均为
`3e13db194b3b62fd39c27eff7de82afe3719dc0b228e27f5f9fceb9e145c5b20`，
`/tmp/drpy-research/sample/070_api_lf_search3_min.js`）运行，得到同一错误：

```bash
UNBOX_REAL_JS_PATH=/tmp/drpy-research/sample/070_api_lf_search3_min.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

```
加载爬虫失败: 模块  编译失败: SyntaxError: SyntaxError: (anonymous): Line 1:363339 Unexpected reserved word (and 5 more errors)
```

根因：`maskNonCode` 不识别正则字面量。脚本在约 172 KB 处有正则 `/^\/\//m`，
其中的 `//` 被误判为行注释，掩码把该行（整个压缩脚本就是一行）直到文件末尾全部
清空，于是文件末尾的 `export default {...}` 未被 `rewriteModule` 改写；两条
`import` 正常剥离后，goja 仍看到 `export`，报 `Unexpected reserved word`。
这正是代码注释里已声明的「不识别正则字面量」限制，本脚本实际触发了它。

**Task 10 修复后复跑（本地副本，SHA 同远程）**：

```
脚本形态: rule=false init=true homeContent=false ...
VodClasses 失败: ReferenceError: setTimeout is not defined at _0x269d1f (<eval>:1:108605(4))
VodCategory("1") 失败: ReferenceError: setTimeout is not defined at _0x269d1f (<eval>:1:108605(4))
```

- 加载/编译：**成功**（`maskNonCode` 升级为识别正则字面量后，`export default`
  不再被漏改；脚本与内置 `cheerio.min.js` 均能编译加载，`init` 已提升）。
- 分类/列表：**进入阶段但失败**——脚本自身的 `_0x269d1f` 调用 `setTimeout`，
  goja 运行时未注入定时器 API，`ReferenceError: setTimeout is not defined`。

根因（已修复）：修复了两处掩码缺陷——①正则字面量 `/^\/\//m` 中的 `//` 不再被
当注释；②内置 `cheerio.min.js` 里的嵌套模板字面量
`` `${...?"":`${function(...)}</${n}>`}` `` 与末尾无空格的 `export default{...}`
此前导致 `export{...}` 被掩码吞掉、`rewriteModule` 漏改。修复后编译通过。

> 新暴露的 `setTimeout` 缺口不在 Task 10 范围（console + 正则掩码），已由
> Task 11 注入 shim 解决。

**Task 11 修复后复跑（本地副本，SHA 同远程）**：

```
脚本形态: rule=false init=true homeContent=false ...
VodClasses 失败: GoError: req 请求失败: Get "https://www.netflix.com/tudum/top10": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
分类列表(0)（取前 3）:
分类列表为空，无法验证详情/播放
```

- 加载/编译：**成功**（与 Task 10 后一致）。
- 分类：**不再因 `setTimeout` 中断**——脚本的 10ms 日志回调已同步执行，流程继续进入
  `home` 的请求阶段。当前失败点是脚本自身把批量请求超时设为 3000ms（反混淆代码中的
  `_0x4a6be9=0xbb8`），而本环境访问该 Netflix 榜单 URL 实测约 4.8–5.0s（连续三次
  `curl` 均 HTTP 200，耗时 4.75s / 4.91s / 5.01s），Go 客户端在拿到响应头前超时。
  属网络/站点延迟，不是 `setTimeout` 或引擎缺陷。
- 列表/详情/播放：**未执行**（分类阶段即失败）。

根因（已修复）：goja 无定时器全局。Task 11 在 `installHelpers()` 注入同步
`setTimeout(fn, ms)`（等待上限 1000ms，返回 `0`）与 no-op `clearTimeout(id)`；只适用
于不依赖真正异步顺序的延迟/日志，不实现事件循环。

## 3. 验收汇总

首轮（Task 9）结果；Task 10 修复后的变化见各小节「修复后复跑」。

| 脚本 | 加载 | 分类 | 列表 | 详情 | 播放 | 阻塞点 |
|---|---|---|---|---|---|---|
| `drpy2_360.js` | ✅ `rule` 解析 | ✅ 4 个 | ❌ | 未执行 | 未执行 | `fyfilter` 占位未实现 → `catid=2fyfilter` 被 API 拒绝 |
| `某短剧站.js` | ✅ ESM + crypto-js | ✅（修复后 9 个） | ❌ 0 条 | 未执行 | 未执行 | 修复后：`filterOptions 解码失败`（筛选参数为空）；原 `console` 缺失已修复 |
| `lf_search3_min.js` | ✅（修复后） | ⚠️ 进入请求阶段 | ❌ | 未执行 | 未执行 | Task 11 修复 `setTimeout`；新阻塞点：脚本自身 3000ms 请求超时 `<` 本机访问 Netflix 榜单约 5s |

## 4. 后续待办（按优先级）

1. ~~**`setTimeout` / 定时器 shim**~~（Task 11 已完成）：注入同步 `setTimeout`
   （等待上限 1000ms）与 no-op `clearTimeout`。`lf_search3_min.js` 已不再因
   `setTimeout` 中断；新阻塞点是它对 `https://www.netflix.com/tudum/top10` 的请求
   使用脚本内置 3000ms 超时，而本环境访问该地址约需 5s。如需跑通该脚本，可后续评估
   在请求层提供可配置超时（不属于引擎定时器范围）；`setInterval` 仍未注入。
2. **drpy2 `fyfilter` / 筛选**：实现 `fyfilter` 占位与 `filter`/`filter_url`
   展开，至少支持「无筛选值 → 空串」，否则大量 drpy2 站点的列表模板不可用。
3. **M7.3 `cat.js` shim**：仍待办；当前不内置 GPL-3.0 的 cat.js，`bili_open.js`
   这类强依赖 `Crypto`/`jinja2`/`_` 的脚本需要自有 shim 才能完整运行。
4. **`某短剧站.js` 筛选参数**：`filterOptions 解码失败` 导致列表返回空，需排查
   分类筛选参数的解码/传递链路。
5. 网络：Go 客户端访问 `raw.example.com` 的 HTTP/2 TLS 握手在本次环境
   超时，`GODEBUG=http2client=0` 可绕过；属本机网络环境问题，不是爬虫代码问题。

## 5. 许可证

内置资源及其许可证见 `internal/crawler/assets/THIRD-PARTY-NOTICES.txt`：

- cheerio（MIT）— 官方 npm `cheerio@1.0.0-rc.12`，esbuild 0.25.0 打包，
  `LICENSE.cheerio.txt`；
- crypto-js（MIT）— `LICENSE.crypto-js.txt`；
- cheerio bundle 内的依赖（parse5、htmlparser2、domhandler、domutils、
  dom-serializer、cheerio-select、css-select、css-what、domelementtype、
  entities、nth-check、boolbase 等；MIT / BSD-2-Clause / ISC）—
  `LICENSE.bundled-deps.txt`（含各包 license 原文与版本）。

分发位置（Task 12）：deb/rpm → `/usr/share/doc/unbox/`；NSIS →
`$INSTDIR\licenses\`；macOS → `UnBox.app/Contents/Resources/licenses/`；
AppImage 与 Windows 便携 exe 为单文件格式，随发布产物提供
`*-licenses.tar.gz`（Linux `UnBox-licenses.tar.gz`，Windows
`UnBox-<ver>-windows-<arch>-licenses.tar.gz`）。

未内置：模板.js（未声明许可证）。cat.js / gbk.js 已改为自有 shim
（见 `internal/crawler/assets/`）。

## 2026-10-19 补充：前端筛选 UI（M7 剩余缺口①）

点播页筛选 UI 已实现并提交（见 `docs/superpowers/specs/2026-10-19-unbox-vod-filter-ui-design.md`）：

- **引擎层**：`Engine.VodCategoryExtend(tid, pg, extend)` 把用户选择并入境
  `categoryContent` 的 extend 对象，声明式路径经 `applyFilterTemplateWithSel`
  （选择覆盖 `filter_def` 默认值后渲染 `{{fl.xxx}}`）。
- **drpy2 规则站**：`Spider.Home` 从 `rule.filter` / `rule.filter_def` 为每个分类
  挂载筛选组（`n`/`v` 双 key、空 options 剔除、标量默认值形态不暴露）；字典形态
  按维度 key 排序保证顺序稳定；`BrowseFiltered` 走 `VodCategoryExtend`。
- **CMS（maccms）**：`ac=list` 的每类 `filter`/`filters` 解析成筛选组，兼容
  `{name,options}`、`{name,value}`、以及 value 直接为数组三种形态；失败静默降级；
  `videolistFilteredPage` 拼 `filter=<URL 编码 JSON>`。
- **前端**：`vodFilters.ts` 纯逻辑（有筛选项才显示入口/单选取值/复位，6 测试）；
  `VodFilterPanel.vue` 弹层（2 测试，Esc/遮罩关闭 + 焦点归还）；App.vue 接
  `VodListFiltered`，切分类/站点/线路/重新导入都复位，分页沿用当前筛选。

**已知限制**：

- 筛选项定义若写在脚本局部常量里（如 `juhe.js` 的 `ruleFilterDef`）而非
  `rule.filter`/`rule.filter_def`，静态提取不到，该站点不会出现筛选入口。
- 实测 360zy / bfzyapi / ffzyapi / wujinapi 的 `ac=list` 返回的 `class` 里都没有
  `filter`/`filters` 字段；CMS 筛选只对定制过、确实下发该字段的源生效。

**未验收项**：真实站点手工冒烟（需 `mise run dev` + 带 filter 的站点实测）；`模板.js` 许可证。
