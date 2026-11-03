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

## 已知限制

- `csp_XYQHiker`、其余 `csp_` 长尾未实现；
- XBPQ「筛选」目前只解析为规则模型，尚未接入分类筛选参数展开；
- `SearchMode` 只保留在规则模型中，M6.1 的搜索按 GET 处理；
- 详情页依赖规则字段；缺少 `播放数组`/`播放列表` 等字段时返回空剧集，而不是猜测网页结构；
- 站点反爬、Cookie 之外的挑战（如 JavaScript 计算、WebView 输入）不在 M6.1 范围。
