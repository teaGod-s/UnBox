# UnBox drpy2 / ES Module 爬虫兼容设计（M7）

日期：2026-10-17
状态：M7.1/M7.2 已实现；真实站点首轮验收未全通过（见
`docs/2026-10-17-m7-acceptance.md`），M7.3 待办
关联：M5.1 JS 爬虫、M5.3 dr_py、`internal/crawler`、`internal/provider/tvbox`

## 1. 背景

当前 UnBox 的 JS 爬虫引擎只支持 FongMi js0 形态（`async function` + 末尾
`export default {...}`）。命中 `import`/`export` 时返回：

> 该站点使用 drpy2 / ES Module 脚本，当前版本不支持（仅支持 FongMi js0 形态的 .js 爬虫）

2026-10-17 对真实 drpy2 / drpyS / ESM 脚本做了逆向与原型验证，结论需要修正：

- **drpy2 站点脚本本身不是 ES Module**。我们抽样的 60+ 个真实 `ext` 脚本
  （`某影视站.js`、`ddys.js` 等）全部是 `var rule = {...}`，0 个 `import`/`export`。
  当前引擎**已经能加载并解析它们**。
- 真正的 bug 是 Provider 选错了脚本源：drpy2 站点的 `api` 指向
  `drpy2.min.js`（引擎库，ESM），站点规则在 `ext` 指向的 `.js` 里；当前
  `tvbox.Spider` 只加载 `site.API`，所以永远加载到引擎库并报 ESM 错误。
- **真正的 ESM 脚本**（`某短剧站.js`、`bili_open.js`、`lf_search3_min.js`）
  需要模块加载器；实测其依赖的 TVBox 资源（cheerio、crypto-js、cat.js、
  模板、gbk）全部能在 goja 中运行。

## 2. 证据

### 2.1 当前引擎直接运行真实脚本

| 脚本 | 形态 | 当前引擎 |
|---|---|---|
| `某影视站.js`、`ddys.js` | drpy2 `var rule` | ✅ `Load OK`，`rule` 解析成功 |
| `apple.js`、`荐片.js` | FongMi js0 `export default` + async | ✅ `Load OK`，动作函数提升为全局 |
| `drpy2.min.js` | ESM（4 个 import） | ❌ `ErrESModuleUnsupported` |
| `某短剧站.js` | `import './lib/crypto-js.js'` + `export function __jsEvalReturn` | ❌ `ErrESModuleUnsupported` |
| `bili_open.js` | `import {Crypto,jinja2,_} from 'assets://.../cat.js'` | ❌ `ErrESModuleUnsupported` |
| `lf_search3_min.js` | 混淆后的 `import`（`\x61...`） | ❌ 原始 `SyntaxError`（未触发友好提示） |

### 2.2 原型验证（临时测试，未入库）

把 ESM 语法剥离、模块体包进 IIFE、注入资源后：

| 脚本 | 结果 |
|---|---|
| `drpy2.min.js` 引擎库 | ✅ 导出 `init/home/homeVod/category/detail/play/search/DRPY` |
| `某短剧站.js` | ✅ 导出 `__jsEvalReturn` |
| `bili_open.js` | ✅ 导出 `__jsEvalReturn` |
| `lf_search3_min.js` | ✅ 导出 `init/home/homeVod/category/detail/play/search` |
| cheerio / crypto-js / cat.js / 模板 / gbk | ✅ goja 可执行；`cheerio.load`、`CryptoJS.MD5`、`Crypto/jinja2/_` 均可用 |

REA 静态分析：对 8 个代表脚本执行 `analyze_javascript_application`，
89,251 个 AST 节点、0 parse failure；父证据 `ev_f209f5b7…`，摘要
`ev_91da9fc2…`。模块图为空（工具限制），模块/导入关系由直接静态扫描补齐。

### 2.3 真实脚本的导入/导出形态

- `import cheerio from "assets://js/lib/cheerio.min.js"`
- `import "assets://js/lib/crypto-js.js"`
- `import { Crypto, jinja2, _ } from "assets://js/lib/cat.js"`
- `import "./lib/crypto-js.js"`
- `import _0x108086 from'\x61\x73\x73...'`（混淆字符串）
- `export default { ... }`
- `export function __jsEvalReturn() { ... }`

## 3. 设计决策

1. **先修 Provider 选源**：当 `api` 是 `drpy2.min.js` / `drpy.min.js` 这类
   引擎库且 `ext` 是 `.js` 时，加载 `ext`，不加载 `api`。
2. **不执行 dex，也不内嵌 JVM**（与 M6 一致）。ESM 用纯 Go 加载器。
3. **ESM 加载器只做「模块解析 + 语法改写 + 同 VM 执行」**，不做打包器：
   - 支持 default / named / namespace / side-effect import。
   - 支持 `export default` / `export function` / `export const|let|var` /
     `export {a as b}`。
   - 每个模块包进 IIFE，避免 `const C` 之类的词法冲突。
   - 支持 UMD/CommonJS（crypto-js 是 UMD）。
   - 不支持的语法（`export *`、动态 `import()`、`import.meta`）明确报错。
4. **资源只内置许可证允许的**：
   - 内置：cheerio（MIT）、crypto-js（MIT）。
   - 不内置：`cat.js`（FongMi/TV，GPL-3.0）、`模板.js` / `gbk.js`
     （某规则仓库/drpy 未声明 License）。
   - `cat.js` 的 `Crypto`/`jinja2`/`_` 用自有 shim 提供；`模板.js`/`gbk.js`
     仅在运行 drpy2 引擎库时需要，M7 不把「执行 drpy2 引擎库」作为主线。
5. **drpy2 主线仍走现有 Go 解释器**（M5.3），不加载 `drpy2.min.js`。
   引擎库加载能力仅作为 M7.3 的可选增强。
6. **`__jsEvalReturn` 是 drpyS/ESM 的入口**：加载后调用它，把返回对象的动作
   方法提升为全局函数，复用现有动作分发。

## 4. 架构

### 4.1 M7.1 Provider 选源

```
config.Site{API: "./dr/lib/drpy2.min.js", Ext: "./dr/js/某影视站.js"}
  → config.ResolveRelativeURLs（解析 Ext 相对路径）
  → tvbox.Spider.ensureLoaded
  → spiderScriptURL(site)：drpy2 引擎库 + ext=.js → 用 Ext
  → crawler.Engine.LoadFromURL(Ext)
  → 现有 var rule 解释器（M5.3）
```

### 4.2 M7.2 ESM 加载器

新增 `internal/crawler/esm.go`：

- `maskNonCode(src) string`：把字符串/模板/注释内容替换为空格，供语法定位。
- `needsModuleLoader(src) bool`：是否存在 `import` 或 `export default` 之外的
  `export`。
- `rewriteModule(vm, src, baseURL) (string, error)`：把 ESM 改写成同 VM 可执行
  的 IIFE 模块。
- `moduleLoader.load(url) (goja.Value, error)`：解析、缓存、执行模块。
- `Engine.loadESM(ctx, src, baseURL) error`：主模块 + 导出提升。
- `Engine.loadWithContext(ctx, src, baseURL) error`：判断走旧路径还是 ESM 路径。

新增 `internal/crawler/assets.go` + `internal/crawler/assets/`：

- `//go:embed assets/cheerio.min.js`
- `//go:embed assets/crypto-js.js`
- `assetSource(url) (string, bool)`：按 basename 映射，兼容
  `assets://js/lib/...` 和相对路径 `./lib/crypto-js.js`。

### 4.3 模块执行语义

```js
(function (__exports, module, exports) {
  <rewritten source>
  ;if (Object.keys(__exports).length > 0) { return __exports; }
  return module.exports;
})(__exportsObj, moduleObj, exportsObj)
```

- `__module(url)` 是 Go 注入的同步函数；改写时相对路径已解析成绝对 URL。
- 缓存按绝对 URL；执行前先把 exports 对象放进缓存，支持循环导入；改写/编译/
  执行失败时删除该缓存项。
- `__jsEvalReturn` 提升：既支持顶层导出，也支持 `export default` 对象内嵌的
  `__jsEvalReturn`（drpyS 风格）；否则 `default` 对象的键也会直接提升为全局。

## 5. 里程碑

### M7.1 drpy2 选源修复（1–2 天）

- `Site.ExtString()`；`ResolveRelativeURLs` 解析字符串型 `Site.Ext`。
- `spiderScriptURL(site)`：drpy2 引擎库 + `ext` 为 `.js` 时用 `ext`。
- 改进错误提示：区分「引擎库被当成站点脚本」与「真正的 ESM 语法」。
- **验收**：`某影视站.js` / `ddys.js` 通过 `Spider` Provider 加载；单元测试覆盖
  选源逻辑；`go test ./...` 全绿。

### M7.2 ESM 加载器（3–5 天）

- 语法扫描/改写、模块解析、缓存、IIFE、UMD。
- 内置 cheerio / crypto-js（MIT）；`assets://` 映射。
- `__jsEvalReturn` 提升。
- **验收**：`某短剧站.js`、`lf_search3_min.js` 加载成功；单元测试覆盖
  default/named/namespace/side-effect、export 四种形态、Unicode 标识符、
  混淆字符串、IIFE 隔离、UMD。

### M7.3 ESM 资源与入口增强（2–3 天）

- `cat.js` 自有 shim：`Crypto`、`jinja2`、`_`（或 MIT 替代）。
- 可选：加载 `drpy2.min.js` 引擎库（需要 `模板.js`/`gbk.js`，按运行时抓取
  或自有实现；不作为主线）。
- **验收**：`bili_open.js` 加载成功；`__jsEvalReturn` 端到端可调用。

### M7.4 真实站点验收（1–2 天）

- drpy2：真实配置的 `api=drpy2.min.js` + `ext=站点.js`，跑通分类/列表/
  搜索/详情/播放。
- ESM：`某短剧站` / `荐片` / `lf_search3_min` 至少各 1 个真实站点。
- 记录到 `docs/2026-10-17-m7-acceptance.md`。

## 6. 非目标

- 不实现运行时通用打包器；构建期仅用 esbuild 生成内置 cheerio bundle。
- 不执行 `drpy2.min.js` 作为默认路径；现有 Go 解释器优先。
- 不内置 GPL 的 `cat.js` 或未声明 License 的 `模板.js`/`gbk.js`。
- 不支持动态 `import()`、`import.meta`、`export * from`、`export * as ns from`、
  `export { a } from`、`export class`（均明确报错）。
- 多声明导出 `export const a=1,b=2` 当前只导出第一个标识符 `a`（文本改写限制）；
  需要时由脚本作者拆成多条导出，或后续升级为声明解析。
- 不解决 async/Promise 的完整语义；沿用现有「文本级剥离 async/await +
  同步 req」策略，ESM 脚本同样处理。

## 7. 许可证矩阵

| 资源 | 来源 | License | 处理 |
|---|---|---|---|
| cheerio.min.js | 官方 npm `cheerio@1.0.0-rc.12`，esbuild 0.25.0 打包 | MIT；bundle 内 parse5/htmlparser2/domutils/css-select 等为 MIT/BSD-2/ISC | 可内置，附 LICENSE.cheerio.txt + LICENSE.bundled-deps.txt |
| crypto-js.js | brix/crypto-js | MIT（GitHub 标 NOASSERTION，需附原文） | 可内置，附 LICENSE |
| cat.js | FongMi/TV `quickjs/src/main/assets/js/lib/cat.js` | GPL-3.0 | **不内置**，用自有 shim |
| 模板.js | 某规则仓库/drpy | 未声明 | 不内置；需要时运行时抓取或自实现 |
| gbk.js | 某规则仓库/drpy | 未声明 | 不内置；需要时运行时抓取或自实现 |
| 站点脚本 | 各配置源 | 各作者 | 运行时下载，不随 UnBox 分发 |

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| 文本级改写误伤字符串/注释 | `maskNonCode` 状态机；测试覆盖字符串、模板、注释、混淆转义 |
| 模块间 `const`/`let` 冲突 | 每个模块包 IIFE；测试两个模块都声明 `const C` |
| `assets://` 资源版本漂移 | 按 basename 固定映射到内置版本；记录版本与 LICENSE |
| 远端相对模块 404/返回 HTML | 对已知 basename 走内置资源；其他 URL 校验 Content-Type/前缀，失败明确报错 |
| async 函数返回 Promise | 沿用 `stripAsyncAwait`；M7.2 增加 ESM async 测试 |
| GPL 污染 | 不内置 cat.js；shim 独立实现 |
| 旧 js0 行为回归 | `needsModuleLoader` 只对 import/非 default export 启用新路径；保留旧测试 |

## 9. 证据附录

- 真实脚本样本：`某影视站.js`、`ddys.js`、`apple.js`、`荐片.js`、
  `某短剧站.js`、`bili_open.js`、`lf_search3_min.js`、`drpy2.min.js`。
- 当前引擎测试结果见 §2.1；原型结果见 §2.2。
- REA：`analyze_javascript_application`（8 文件、89,251 AST 节点、0 parse
  failure；父证据 `ev_f209f5b7…`，摘要 `ev_91da9fc2…`）。
- 资源 goja 验证：`cheerio.load("<div>hi</div>")("div").text()` → `hi`；
  `CryptoJS.MD5("abc")` → `900150983cd24fb0d6963f7d28e17f72`；
  `cat.js` 导出 `Crypto/Uri/_/cheerio/jinja2/...`。
