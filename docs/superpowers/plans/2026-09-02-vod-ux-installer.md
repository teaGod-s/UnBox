# 点播、直播与安装体验增量 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在既有点播导航修复之上，完成本轮 11 项点播、直播、持久化、安装器和依赖同步改进。

**Architecture:** Store 提供结构化的搜索历史、点播收藏和观看记录删除接口；ShellService 负责 TTL 分类缓存和 Wails API；Vue 顶层模式负责搜索/收藏导航，播放器状态与固定 frame 负责避免布局抖动。NSIS 通过卸载注册表记忆目录并以提示方式处理运行中的进程。

**Tech Stack:** Go 1.26.3、modernc.org/sqlite、Wails v3 beta.9、Vue 3、TypeScript、Vitest、NSIS。

**Spec:** `docs/superpowers/specs/2026-09-02-vod-ux-installer-design.md`（基于 `docs/superpowers/specs/2026-09-01-vod-playback-navigation-design.md`）。

## Global Constraints

- 继续在 `fix/vod-playback-navigation` 分支工作，每个 task 完成并验证后独立提交。
- 所有公开错误文本和注释使用中文；不把 `.superpowers/sdd` 或真实站点脚本加入 Git。
- Wails 固定为 `3.0.0-beta.9`；业务层不得导入 Wails。
- 修改 Go 后运行 `gofmt` 和对应包测试；最终运行全量 test、vet、gofmt 检查和 Linux CGO build。
- 浏览器依赖升级只采用官方来源、兼容的 patch/minor 版本。

---

### Task 1: Store 搜索历史、点播收藏与观看记录删除

**Files:**
- Modify: `internal/store/store.go`（模型、建表和 CRUD）
- Modify: `internal/store/store_test.go`（新增失败测试）

**Interfaces:**
- Produces `RecordVodSearch(query string) error`、`ListVodSearchHistory(limit int) ([]string, error)`、`DeleteVodSearch(query string) error`。
- Produces `DeleteVodHistory(site, vodID string) error`。
- Produces `AddVodFavorite(site, vodID, title, logo, group string) error`、`RemoveVodFavorite(site, vodID string) error`、`IsVodFavorite(site, vodID string) (bool, error)`、`ListVodFavorites() ([]VodFavorite, error)`。

- [ ] 写测试：建库后记录同一搜索词两次只保留一条且按最近时间排序；删除只影响指定词；收藏按 `site+vod_id` 去重并可删除；历史删除后列表为空。
- [ ] 运行 `GOCACHE=/tmp/unbox-task1-cache go test ./internal/store -run 'Vod(Search|Favorite|History)' -count=1`，确认新增方法/表尚不存在导致失败。
- [ ] 在 `migrate` 增加三张表，在 Store 中实现上述 SQL，并复用现有 `nullString`/时间转换风格。
- [ ] 运行同一测试并执行 `gofmt -w internal/store/store.go internal/store/store_test.go`。
- [ ] 提交：`feat(store): persist vod search history and favorites`。

### Task 2: ShellService API 与分类 TTL 缓存

**Files:**
- Modify: `internal/shell/app.go`、`internal/shell/service.go`
- Modify: `internal/shell/service_test.go`（或现有 shell 测试文件）

**Interfaces:**
- Produces Wails methods `DeleteVodHistory`、`RecordVodSearch`、`ListVodSearchHistory`、`DeleteVodSearchHistory`、`AddVodFavorite`、`RemoveVodFavorite`、`IsVodFavorite`、`ListVodFavorites`。
- `VodCategories(site string)` 使用 5 秒 TTL 缓存；`ImportVodSource`/`DeleteSource` 清除受影响站点缓存。

- [ ] 写测试替换/注入分类 provider，断言 TTL 内只调用一次、过期后重新调用、源删除后立即失效。
- [ ] 运行 `GOCACHE=/tmp/unbox-task2-cache go test ./internal/shell -run 'Vod(Category|Favorite|Search|History)' -count=1`，确认缓存和 API 尚未实现时失败。
- [ ] 在 `ShellService` 增加带 mutex 的缓存条目（分类副本和过期时间），实现 Store 委托 API；缓存只缓存成功结果。
- [ ] 运行 shell 测试、`gofmt -w internal/shell/app.go internal/shell/service.go internal/shell/*_test.go`，再生成 Wails TypeScript bindings。
- [ ] 提交：`feat(shell): add vod persistence APIs and category cache`。

### Task 3: 顶部搜索、搜索历史和观看记录删除

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/src/vodNavigation.ts`（模式/返回辅助函数）
- Modify/Create: `frontend/src/*test.ts`（Vitest 回归测试）

**Interfaces:**
- 顶层 mode 扩展为 `search`、`favorites`；`search` 页面使用现有 token/event 机制。
- 搜索提交调用 `RecordVodSearch`，历史项点击复用搜索，删除调用 `DeleteVodSearchHistory`。
- 首页历史项显示删除按钮，调用 `DeleteVodHistory` 后从本地 ref 移除。

- [ ] 写 Vitest 测试：导航点击进入搜索；搜索词提交后历史更新；删除单项不影响其他项；首页删除记录不触发播放。
- [ ] 运行 `npm test -- --run frontend/src`，确认新模式/API 断言失败。
- [ ] 移除 VOD list/detail 内嵌搜索 row，新增顶部搜索/结果页面，保留 5 分钟结果缓存和正确返回来源。
- [ ] 绑定历史删除、首页删除和空状态，保持取消搜索与 stale token 行为。
- [ ] 运行 `npm test`、`npm run build`，提交：`feat(ui): move vod search to top navigation`。

### Task 4: 点播收藏页面和详情按钮

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`
- Modify: `frontend/src/*test.ts`

**Interfaces:**
- 收藏模式调用 `ListVodFavorites`，条目点击打开详情；详情简介面板按钮调用 `AddVodFavorite`/`RemoveVodFavorite`，按钮状态由 `IsVodFavorite` 初始化。

- [ ] 写测试覆盖收藏/取消收藏按钮调用和收藏页条目打开详情。
- [ ] 运行单测确认失败。
- [ ] 实现收藏模式、详情按钮、收藏条目逐条删除和列表空状态；收藏条目携带 site/id/title/logo/group，不与直播收藏混用。
- [ ] 运行前端单测和生产构建，提交：`feat(ui): add vod favorites`。

### Task 5: 播放器固定 frame、切台反馈与居中布局

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/src/components/PlaybackView.vue`
- Modify: `frontend/public/style.css`
- Modify: `frontend/src/*test.ts`

**Interfaces:**
- 点播/直播播放状态为 `idle|preparing|playing|error`，错误状态清空对应 plan 并显示中文错误。
- `.vod-player-frame` 保持 16:9、`min-height`/`max-height` 边界；控制区与集数区不在 frame 内。

- [ ] 写测试：切换不可用直播频道时旧 plan 被清空且显示错误；点播播放器加载状态变化不改变 frame 高度；折叠简介后控制按钮仍存在于 DOM。
- [ ] 运行单测确认失败。
- [ ] 为播放请求设置 preparing/playing/error 状态，检查 token 后才更新状态；PlaybackView 外层使用稳定 frame 和 overflow 约束。
- [ ] 统一搜索进度、播放器控制、集数分页的 `align-items:center` 与 frame 之外滚动。
- [ ] 运行前端测试和 `npm run build`，提交：`fix(ui): stabilize playback frame and switching feedback`。

### Task 5a: 按需提示 mpv 安装

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`（如需提示样式）
- Modify: `frontend/src/*test.ts`

**Interfaces:**
- Windows/macOS 在 Web 播放能力可用时不主动调用 `MPVStatus`/显示安装提示；只有 PlaybackView 触发 fallback 且 mpv 未就绪时显示安装提示。

- [ ] 写测试：初次打开点播/直播页面不显示 mpv 安装提示；收到 Web fallback 事件后才显示；Linux 保留现有 mpv 就绪提示策略。
- [ ] 运行单测确认失败。
- [ ] 将 mpv 检查从启动/页面进入路径移到 fallback 处理路径，提示包含安装和继续回退的明确操作。
- [ ] 运行前端测试和生产构建，提交：`fix(ui): defer mpv install prompt until web playback fallback`。

### Task 6: 集数与直播频道展示

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`
- Modify: `frontend/src/*test.ts`

**Interfaces:**
- 每页固定 36 集，分页左右箭头和 tab 在同一水平线； episode grid 桌面 6 列、小屏 3 列且按钮等宽。
- 频道名提供 `title`，长名称 hover 在固定宽度内滚动；列表宽度随容器变化。

- [ ] 写测试覆盖 36 集分页范围、箭头滚动事件和频道长名称属性。
- [ ] 运行测试确认失败。
- [ ] 调整模板/CSS，避免播放器加载、窗口缩放或文本长度改变父级高度。
- [ ] 运行测试、构建并提交：`fix(ui): align episodes and responsive live channels`。

### Task 7: 线路/站点切换刷新与缓存回归

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`（错误/加载状态）
- Modify: `frontend/src/*test.ts`

**Interfaces:**
- `selectLine`/`selectSite` 在切换期间清空旧分类、显示加载状态，成功后刷新列表；失败时显示错误并不复用旧分类。

- [ ] 写测试：从可用切到不可用时分类列表清空并展示错误；TTL 内重复切回不重复请求。
- [ ] 运行测试确认失败。
- [ ] 接入 Task 2 API 和分类缓存状态，处理切换请求的 stale token。
- [ ] 运行前端测试并提交：`fix(ui): refresh vod categories after source changes`。

### Task 8: Windows NSIS 安装体验

**Files:**
- Modify: `build/windows/nsis/project.nsi`
- Modify: `build/windows/nsis/wails_tools.nsh`（仅在需要补充通用宏时）

**Interfaces:**
- 安装器从当前 scope 的 `${UNINST_KEY}` 读取 `InstallLocation`；`wails.writeUninstaller` 写入该值。
- `.onInit` 检测 UnBox 窗口，提示关闭并支持 Retry/Abort，不执行强杀。

- [ ] 写静态检查脚本/断言，确认 `InstallDirRegKey`、`InstallLocation`、检测/重试消息存在。
- [ ] 运行静态检查确认失败。
- [ ] 按 NSIS 语法加入 per-user/per-machine 注册表读取和关闭检测函数；将检测放在架构检查之后。
- [ ] 在有 `makensis` 的 Windows 主机执行 `makensis` 编译；Linux 记录未安装 NSIS，不伪称通过。
- [ ] 提交：`fix(windows): remember install directory and handle running app`。

### Task 9: 开源库版本审计与设置页同步

**Files:**
- Modify: `go.mod`/`go.sum`（仅兼容更新）
- Modify: `frontend/package.json`/锁文件（仅兼容更新）
- Modify: `frontend/src/App.vue`（开源库版本清单）

**Interfaces:**
- Wails 仍为 `3.0.0-beta.9`；设置页版本清单与实际 manifest 对齐。

- [ ] 使用联网技能查询 Go 模块和 npm 包官方 release/registry，记录候选更新及兼容理由。
- [ ] 写版本清单测试或构建检查，确认清单中的版本来自 manifest。
- [ ] 只更新有明确收益的 patch/minor 版本，锁文件与 go.sum 同步。
- [ ] 运行全量 Go/前端验证，提交：`chore(deps): sync compatible open source versions`（若无安全更新则提交设置页版本同步或明确不改依赖）。

### Task 10: 全量验收

**Files:**
- No product source changes expected; only test/build artifacts outside Git。

- [ ] 运行 `mise exec -- env GOCACHE=/tmp/unbox-final-test-cache go test ./... -count=1`。
- [ ] 运行 `mise exec -- env GOCACHE=/tmp/unbox-final-vet-cache go vet ./...`。
- [ ] 运行 `gofmt -l` 检查 Go 源码为空输出，运行前端 `npm test` 和 `npm run build`。
- [ ] Linux 运行 `CGO_ENABLED=1 go build ./...`；Windows NSIS 在原生主机验收。
- [ ] 检查 `git status`，确认没有 `.superpowers/sdd`、真实站点脚本或构建产物进入 Git。
