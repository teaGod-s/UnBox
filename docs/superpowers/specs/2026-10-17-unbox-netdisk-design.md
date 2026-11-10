# UnBox 网盘源兼容设计（M8）

日期：2026-10-17
状态：设计定稿，待 M8.1/M8.2 实施
关联：`internal/provider/tvbox`、`internal/provider/netdisk`（新增）、`internal/playback`

## 1. 背景

用户提供的聚合源里包含网盘类站点。实测可统计的约 646 个站点中，约 55 个是
网盘相关条目（跨配置有重复，约 8.5%），分为 6 类：

| 类别 | 数量 | 代表类 | 兼容性 |
|---|---:|---|---|
| 网盘搜索/分享搜索 | 17 | `PanSearch`、`UpYun`、`Zhaozy`、`YiSo`、`PanSou`、`QuarkPanso`、`AliShare` | 搜索可做，播放依赖云盘解析 |
| 其他网盘聚合 | 12 | `Panta`、`YunPan`、`Xpanpan`、`EightEight`、`HunHePan`、`DaPanSo`、`YunSo`、`YunPanOne`、`TGYunPan` | 多数是推送/搜索 |
| Wogg/Wobg | 10 | `csp_Wogg`、`csp_Wobg` | 不建议：WebView/输入框/站点反爬 |
| 推送 | 7 | `Push`、`PushAgent`、`PushShare` | 不是内容源 |
| 分享链接解析 | 6 | `QuarkShare`、`UCShare`、`P115Share`、`PikPakShare`、`ThunderShare`、`SambaShare` | 可做，但需 token + 云盘 API + 代理 |
| AList/WebDAV/Samba | 3 | `csp_AList`、`csp_WebDAV`、`csp_SambaShare` | 最可行，标准 HTTP/WebDAV/SMB |

## 2. 调研证据

### 2.1 类结构

以 `pg.jar`（SHA-256 `0e622ce7…`，REA 归档证据 `ev_63cf65ae…`）为例：

```
Push (聚合分发)
├── Ali      (阿里云盘)
├── Quark    (夸克)
├── UC       (UC 网盘)
├── P115     (115)
├── PikPak
├── Thunder  (迅雷)
└── YD       (天翼)

QuarkShare extends Quark
UCShare    extends UC
AliShare   extends Ali
P115Share  extends P115
Wogg       extends Push
Wobg       extends Wogg
PanSearch / UpYun / Zhaozy extends Push/Ali
AList / WebDAV extends Spider
```

- `Push.detailContent` 按 URL 特征分发到 `Ali` / `Quark` / `UC` / `P115` / `YD` /
  `Thunder` / `PikPak`。
- `Quark`/`UC`/`Ali`/`P115` 的核心逻辑在混淆后的
  `com.github.catvod.parser.merge.*` 库中，负责分享链接 → 文件列表 → 直链/下载地址。
- `Wogg` 直接依赖 `android.webkit.WebView`、`AlertDialog`、`EditText`；`Wobg`
  还会 `showInput(...)`。不是纯解析，而是站点交互/反爬/人工输入。
- `QuarkShare`/`UCShare` 使用 SQLite + LruCache + OkHttp，理论上可 Go 重写，
  但仍需云盘 token 和 API。

### 2.2 凭据模型

- `./lib/token.json`：阿里云盘 token 包
  （`token/open_token/open_api_url/oauth_client_id/oauth_client_secret/oauth_auth_url/oauth_refresh_url/is_vip`）。
- `./json/alishare.txt` / `quarkshare.txt` / `ucshare.txt` / `115share.txt` /
  `thundershare.txt`：每行形如 `self 我的夸克网盘` 或 `user:pass@host/share 名称 0`。
- `./json/wogg.json`：分类筛选配置，不是 token。
- 还有 `http://127.0.0.1:9978/file/TV/quark.txt` 这类 TVBox 本地文件服务地址。

结论：云盘解析类站点依赖用户自己的账号/Token；公开配置里的 token 往往是
示例/过期/限权，不能作为产品内置凭据。

### 2.3 AList / WebDAV 的 ext

AList：

```json
{
  "vodPic": "…",
  "drives": [
    {"name": "弱水", "server": "http://shicheng.wang:555/"},
    {"name": "NICS", "server": "https://nics.eu.org"}
  ]
}
```

私有盘可带：

```json
{"name":"模板","server":"https://one.fongmi.com","hidden":true,
 "login":{"username":"…","password":"…"},
 "params":[{"path":"/安斯拉拉","pass":"…"}]}
```

WebDAV：

```json
{"drives":[{"name":"七米藍","server":"https://…/dav","user":"…","pass":"…"}]}
```

AList 是标准 HTTP JSON API（`/api/fs/list`、`/api/fs/get`、`/api/public/path`、
`/d/...`），WebDAV 是 PROPFIND + GET + Basic Auth；都不需要逆向云盘私有 API。

### 2.4 UnBox 播放侧现状

- `player.Stream` 已支持 `Headers`（Referer / UA / Cookie 等）。
- `internal/playback` 的 Web 路径会把流注册到本地代理，代理会注入
  `stream.Headers`，并支持 Range / HLS 分片重写。
- 因此 AList/WebDAV 的直链 + `Authorization` / Basic Auth 可以直接复用现有
  播放链路；不需要为网盘单独做播放器。
- 已知限制：`player.KindForURL` 只识别 MP4/FLV/TS/RTMP/本地，MKV/AVI 等会落到
  HLS 默认值，依赖现有 Web 失败后的 mpv fallback。M8.1/M8.2 不新增文件类型枚举；
  若实测 fallback 体验差，再单独加 `StreamFile` 路由。

## 3. 设计决策

1. **M8 主线：原生 Go AList + WebDAV Provider。** 不执行 JAR、不内嵌 JVM。
2. **多盘聚合**：一个站点（一个 ext）里的多个 drive 映射为多个 `Section`；
   `Browse/Detail/Resolve` 通过编码后的 drive+path 分发。
3. **不做云盘分享解析（Quark/UC/Ali/115/PikPak/Thunder）作为 M8.1/M8.2 范围**。
   先让用户用 AList 挂载这些网盘；AList 是官方/标准的聚合层。
4. **Wogg/Wobg 不做**：依赖 WebView/输入框/反爬；如需支持，走远程 Android/
   Java 桥（M6.4/M7.3 的远程 spider 方案）。
5. **推送类不做**：`Push`/`PushAgent`/`PushShare` 不是内容源。
6. **凭据处理**：
   - M8.1/M8.2 直接使用 ext 里的 server/login/user/pass；不新增 token 设置页。
   - 不在日志、错误信息、前端绑定中输出密码/token。
   - 后续如需云盘解析，再做系统钥匙串/加密存储 + 设置页。
7. **搜索**：AList 支持 `/api/fs/search`；WebDAV 标准没有搜索，返回明确
   「WebDAV 不支持搜索」，不静默伪装。
8. **目录展开**：UnBox 的 Provider 接口没有文件夹导航，`Detail` 对目录做
   递归展开（深度 ≤3、文件数 ≤500），超出则截断并在 `Remarks` 标注。

## 4. 架构

新增包 `internal/provider/netdisk`（不依赖 Wails）：

```
internal/provider/netdisk/
├── netdisk.go     # Provider：ID 编码、Home/Browse/Detail/Resolve/Search、目录展开
├── ext.go         # 读取 ext（字符串 URL / JSON 字符串 / 对象）
├── model.go       # Entry、Backend 接口、视频扩展名
├── alist.go       # AList ext 解析 + Backend
├── alist_client.go# AList HTTP 客户端（login/list/get/search）
├── webdav.go      # WebDAV ext 解析 + Backend
├── webdav_client.go# PROPFIND/GET + Basic Auth + XML 解析
├── *_test.go
```

配置/壳层：

- `internal/config/classify.go`：`csp_AList` → `alist`、`csp_WebDAV` → `webdav`，
  `SupportYes`。
- `internal/shell/service.go`：`collectVodSites` 增加 `case "alist", "webdav"`。
- `Site.ExtString` / `ResolveRelativeURLs` 的 ext 解析复用 M7.1/M6.2 的改动；
  若尚未落地，M8 先补。

数据流：

```
config.Site{API:csp_AList, Ext:"./json/alist.json"}
  → config.ResolveRelativeURLs（ext 变绝对 URL）
  → netdisk.New(site)（只校验类名/ ext）
  → 首次动作：loadExt → alistExt/webdavExt → Backend
  → Provider.Home/Browse/Detail/Resolve
  → player.Stream{URL, Headers} → 现有 Web/mpv + 代理
```

## 5. 里程碑

### M8.1 AList（2–4 天）

- ext 解析、多 drive、公开/私有盘、路径密码。
- Home/Browse/Detail（含目录递归展开）/Resolve/Search。
- 直链 + `Authorization` header。
- **验收**：fixture httptest 覆盖 login/list/get/search；至少 1 个真实公开 AList
  服务器跑通浏览/播放。

### M8.2 WebDAV（2–3 天）

- ext 解析、Basic Auth、PROPFIND depth 0/1、XML 解析。
- Home/Browse/Detail/Resolve；Search 明确不支持。
- **验收**：fixture 覆盖 PROPFIND/GET；至少 1 个真实 WebDAV（或本地
  `x/net/webdav` 测试服务器）跑通浏览/播放。

### M8.3 可选 Samba（2–3 天）

- SMB2 客户端（需评估依赖）；ext 行格式 `user:pass@host/share`。
- 只在有明确需求时做。

### M8.4 可选云盘解析（每个 1–2 周）

- Quark/UC 优先，再 Ali/115；需要 token 设置页、安全存储、直链刷新、代理。
- 或继续用 AList 兜底，不单独实现。

> **2026-11-08 实测（`csp_Duopan`/某聚合源 源码）：** 从
> `https://xn--cnq85d.52011.top/a?id=1` 抓到的「Duopan.jar」实为 **APK**
> （签名 `tv.nxog.example.com_ysc`），`classes.dex` 内为 CatVod spider：
> `com/github/catvod/spider/{Wogg,Bili,Push,Init}`，用 okhttp3 + gson。
> 其核心是**多网盘分享解析 SDK**：Quark/UC 走 `drive-pc.quark.cn`/
> `pc-api.uc.cn/1/clouddrive/file/sort`（需 cookie + shareid），Ali 走
> `open.aliyundrive.com/oauth`（需 refresh token），百度走
> `pan.baidu.com/api/gettemplatevariable`，123 盘走 `file/list`，另含
> `javax.crypto AES`（分享口令解密）。结论：**转 Go 成本 ≈ M8.4 云盘解析
> 本身（每个 1–2 周），且只覆盖 2–3 个站点，方向与 M8 非目标重复**。
> 维持「暂不支持 `csp_Duopan`」，预算留给长尾规则同构类。

### M8.5 可选搜索聚合（按需）

- PanSearch/UpYun/Zhaozy/YiSo/PanSou/QuarkPanso。
- 先只做搜索；播放明确提示「需要 AList/网盘账号」。

## 6. 非目标

- 不实现 Wogg/Wobg（WebView/反爬）。
- 不实现 Push/PushAgent/PushShare（推送，不是内容源）。
- M8.1/M8.2 不做云盘分享解析和 token 设置页。
- 不把 AList/WebDAV 做成通用文件管理器；不做上传/删除/重命名。
- 不做跨盘全局搜索（AList 搜索按 drive 逐个调用）。

## 7. 安全与风险

| 风险 | 对策 |
|---|---|
| ext 含用户名/密码 | 不打印日志；错误信息只报 URL/状态码；前端不展示密码 |
| 私有 AList token 过期 | 首次请求/401 时重新登录；失败给出可操作提示 |
| 直链过期 | `Resolve` 时实时调用 `/api/fs/get`，不缓存直链 |
| 目录过大 | 深度/文件数上限；截断时在 Remarks 标注 |
| WebDAV XML 命名空间差异 | 按 local-name 解析；跳过 self entry；测试覆盖 |
| AList 版本差异（v2/v3） | 同时兼容 `data.content` / `data.files`；字段用 flex 类型 |
| 云盘 API 非官方/ToS | M8.1/M8.2 不碰；后续云盘解析单独评估 |
| 凭据泄露到日志/错误 | 统一 `redact`；测试断言错误不含密码 |

## 8. 证据附录

- 网盘类分布：约 55 个条目；分类见 §1。
- `pg.jar`：SHA-256 `0e622ce7…`；REA 归档证据 `ev_63cf65ae…`。
- 类结构：`Wogg extends Push`、`Wobg extends Wogg`、`QuarkShare extends Quark` 等
  （jadx 1.4.7 反编译）。
- `csp_Duopan`（某聚合源）实测：APK，`classes.dex` 内 Wogg 调 Quark/Ali/百度/123 盘
  分享 API（okhttp3+gson+AES），见 §M8.4 结论。
- ext 样本：`gao_alist.json`、`gao_webdav.json`、`candy_alist.json`、
  `token.json`、`*share.txt`（格式见 §2.2/§2.3）。
- UnBox 播放侧：`internal/playback` 已注入 `Stream.Headers`，支持 Range/HLS。
