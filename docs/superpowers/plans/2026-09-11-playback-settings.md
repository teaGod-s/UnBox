# 播放设置与点播自动化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 UnBox 增加可持久化的播放设置，并统一 Web 与 mpv 点播会话的自动切集、自动换源、续播和下一集预载行为。

**Architecture:** `ShellService` 保存三个独立 KV 设置并把 mpv 事件以带 token 的 Wails 事件桥接到前端；播放器故障切换包装器内部消费事件但向外扇出。`App.vue` 作为唯一点播协调器，使用纯 TypeScript 决策函数、会话 token 和任务代际管理自动流程；`PlaybackView` 只报告标准信号，独立 `PreloadView` 只负责 Web 预载元素。

**Tech Stack:** Go 1.26.3、Wails v3 beta.9、modernc SQLite KV、Vue 3、TypeScript、现有 Go/Vitest 测试工具链、mpv JSON IPC。

**Spec:** `docs/superpowers/specs/2026-09-11-playback-settings-design.md`

## Global Constraints

- 每个任务先写失败测试、运行到失败，再实现最小改动；每个任务独立提交。
- 执行前在独立 worktree/功能分支中操作，不直接修改 `master`；Wails 绑定由仓库生成命令更新，不手工维护生成文件。
- 三个 KV 键固定为 `playback.autoNext`、`playback.autoSwitchSource`、`playback.preloadNext`，缺失/非法值全部按关闭处理。
- 明确错误立即换源；连续 30 秒未起播或持续缓冲 30 秒换源；自动换源每条其他线路最多一次并按详情页顺序尝试。
- 自动切集只使用当前线路顺序；线路匹配仅接受名称 `trim()` 后完全相等；不以目标线路第一集替代当前集。
- mpv 预载不得创建第二个 mpv 或调用共享播放器 `Load`；预载失败、超时、取消不得改变当前播放。
- 修改 Go 后运行 `gofmt`；提交前必须通过 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 和 Linux `CGO_ENABLED=1 go build ./...`，前端必须通过 `npm test -- --run` 与 `npm run build`。

---

## 目标与边界

实现已确认的设计文档：新增持久化的“播放设置”分类，并让 Web 播放与 mpv 播放共享自动切集、自动换源、续播进度和下一集预载的点播会话协调逻辑。

固定行为如下：

- 三个选项首次安装默认关闭，KV 键固定为 `playback.autoNext`、`playback.autoSwitchSource`、`playback.preloadNext`。
- 明确播放错误立即换源；连续 30 秒未起播或连续缓冲 30 秒换源。
- 自动换源按详情页线路顺序、每条线路最多一次；只匹配当前集名称（去首尾空白后完全匹配），找不到同名集就跳过。
- 自动切集只在当前线路剧集数组内取下一集，当前线路没有下一集时停止。
- Web 预载使用独立隐藏媒体元素；mpv 只做可取消的后台网络预热，不创建第二个 mpv，也不调用共享播放器 `Load`。
- 自动切集、手动换源、自动换源和旧请求都必须受播放 token/任务代际保护。

## 任务 1：增加播放设置的后端持久化接口

**文件：**

- `internal/shell/service.go`
- `internal/shell/service_test.go`
- Wails 生成的 `frontend/bindings/.../shellservice.ts`（由生成命令更新）

**接口：**

- 输入：现有 `*store.Store` KV 实例。
- 产出：`ShellService.GetPlaybackSettings() PlaybackSettings` 和 `ShellService.SetPlaybackSettings(PlaybackSettings) error`，供前端设置页及点播协调器调用。

测试辅助函数：在 `service_test.go` 增加 `newPlaybackSettingsService(t, st *store.Store) *ShellService`，用 `live.New(nil)`、空 player 和传入 store 调用 `NewShellService`，并在测试结束关闭服务。

**实现：**

1. 在 `internal/shell` 定义导出结构：

   ```go
   type PlaybackSettings struct {
       AutoNext         bool
       AutoSwitchSource bool
       PreloadNext      bool
   }
   ```

2. 增加 `GetPlaybackSettings() PlaybackSettings` 和 `SetPlaybackSettings(PlaybackSettings) error`。读取三个独立 KV 值；缺失、无法解析或 store 读取失败时按 `false` 处理。设置时分别写入三个键，任一写入失败立即返回错误。
3. 运行绑定生成命令，确认 TypeScript 中出现两个新方法和 `PlaybackSettings` 类型。

**步骤：**

- [ ] **先写失败测试**，在 `service_test.go` 中加入：

  ```go
  func TestPlaybackSettingsRoundTripAndDefaults(t *testing.T) {
      st, err := store.Open(t.TempDir() + "/playback-settings.db")
      if err != nil { t.Fatal(err) }
      t.Cleanup(func() { _ = st.Close() })
      svc := newPlaybackSettingsService(t, st)
      if got := svc.GetPlaybackSettings(); got != (PlaybackSettings{}) {
          t.Fatalf("missing keys = %#v, want all false", got)
      }
      want := PlaybackSettings{AutoNext: true, AutoSwitchSource: true, PreloadNext: false}
      if err := svc.SetPlaybackSettings(want); err != nil { t.Fatal(err) }
      svc2 := newPlaybackSettingsService(t, st)
      if got := svc2.GetPlaybackSettings(); got != want { t.Fatalf("round trip = %#v, want %#v", got, want) }
  }
  ```

- [ ] 运行 `go test ./internal/shell -run TestPlaybackSettingsRoundTripAndDefaults -count=1`，确认测试先因方法不存在或断言失败而失败。
- [ ] 实现 `PlaybackSettings`、三个 KV 键的读写和错误归一化；读取错误返回零值，写入错误原样返回。
- [ ] 增加非法值测试：直接写入非 `true`/`false` 文本后，只有对应字段回到 `false`。
- [ ] 运行同一测试命令，确认通过。
- [ ] 执行 `mise exec -- wails3 generate bindings -f '' -clean=true -ts -i ./...`，确认生成绑定包含结构和两个方法。

**验证与提交：**

```bash
gofmt -w internal/shell/service.go internal/shell/service_test.go
go test ./internal/shell -count=1
git add internal/shell/service.go internal/shell/service_test.go frontend/bindings
git commit -m "feat(settings): persist playback automation options"
```

## 任务 2：补齐播放器事件语义并修复故障切换事件扇出

**文件：**

- `internal/player/player.go`
- `internal/player/mpvproc/ipc.go`
- `internal/player/mpvproc/ipc_test.go`
- `internal/player/mpvproc/mpvproc.go`
- `internal/player/mpvproc/mpvproc_test.go`
- `internal/player/failover/failover.go`
- `internal/player/failover/failover_test.go`

**接口：**

- 输入：mpv JSON IPC property-change/end-file 事件，及现有 `player.Player` inner 实例。
- 产出：`player.EventPlaying`；`mpvproc` 通过 `Events()` 发出标准事件；`failover.Player.Events()` 返回不被内部消费的扇出通道。

**实现：**

1. 在 `player.EventKind` 增加 `EventPlaying`；保留 `EventPosition`、`EventBuffering`、`EventError`、`EventEOF` 的现有含义。
2. 在 mpv 观察属性中处理 `paused-for-cache`：`true` 映射为 `EventBuffering`，`false` 映射为 `EventPlaying`；`time-pos` 继续映射为 `EventPosition`；`end-file` 的 `reason=eof` 映射为 `EventEOF`，其他结束原因映射为携带错误的 `EventError`。用户主动暂停不得被误报成缓冲。
3. 修改 failover wrapper，使它拥有自己的事件 channel：内部 goroutine 消费 inner 事件处理切换，同时把所有位置、播放、缓冲、错误和结束事件转发到 wrapper channel；`Events()` 只返回 wrapper channel，`Close()` 同时关闭转发循环和 inner。

**步骤：**

- [ ] **先写失败测试**，为 `parseEvent` 加入表格用例：

  ```go
  tests := []struct{ name string; input string; kind player.EventKind }{
      {"cache start", `{"event":"property-change","name":"paused-for-cache","data":true}`, player.EventBuffering},
      {"cache end", `{"event":"property-change","name":"paused-for-cache","data":false}`, player.EventPlaying},
      {"position", `{"event":"property-change","name":"time-pos","data":12.5}`, player.EventPosition},
      {"eof", `{"event":"end-file","reason":"eof"}`, player.EventEOF},
      {"error", `{"event":"end-file","reason":"error"}`, player.EventError},
  }
  ```

- [ ] 运行 `go test ./internal/player/mpvproc ./internal/player/failover -run 'Test(ParseEvent|Events)' -count=1`，确认新事件类型/扇出测试失败。
- [ ] 实现 `EventPlaying` 和 `paused-for-cache`/非 EOF end-file 映射；不要观察普通 `pause` 属性。
- [ ] 将 failover 的 inner 消费循环改为向 wrapper channel 转发事件，并在 `Close` 中关闭 wrapper channel。
- [ ] 增加 failover 测试，断言 inner 发出的 `EventPosition`、`EventError` 可从 outer `Events()` 收到，同时故障仍会切换。
- [ ] 运行 `go test ./internal/player/... -count=1`，确认通过。

**验证与提交：**

```bash
gofmt -w internal/player/player.go internal/player/mpvproc internal/player/failover
go test ./internal/player/... -count=1
git add internal/player
git commit -m "fix(player): fan out playback events through failover"
```

## 任务 3：把 mpv 事件桥接到当前点播会话

**文件：**

- `internal/shell/service.go`
- `internal/shell/service_test.go`

**接口：**

- 输入：`player.Event` 与当前 `playbackToken`。
- 产出：Wails 事件 `playback:event`，载荷为 `PlaybackEvent{Token, Kind, Position, Error}`；事件字符串固定为 `playing`、`buffering`、`position`、`error`、`ended`。

**实现：**

1. 定义 Wails 事件载荷：

   ```go
   type PlaybackEvent struct {
       Token    uint64
       Kind     string
       Position float64
       Error    string
   }
   ```

2. 在 `NewShellService` 中启动一个后台桥接循环，消费 `s.player.Events()`，读取当前 token 后通过 `application.Get().Event.Emit("playback:event", payload)` 发出；无当前 token 时丢弃事件。`ServiceShutdown` 先停止桥接循环，再关闭播放器和其他服务。
3. 把 `player.EventKind` 映射为稳定字符串 `playing`、`buffering`、`position`、`error`、`ended`。映射逻辑提取成纯函数，避免单元测试依赖 Wails application 全局对象。
4. 前端在任务 8 中监听时只处理 payload token 等于当前点播 token 的事件；监听器必须在组件卸载时移除。

**步骤：**

- [ ] **先写失败测试**，加入纯映射断言：

  ```go
  got := playbackEventFor(player.Event{Kind: player.EventPosition, Position: 8.25}, 17)
  want := PlaybackEvent{Token: 17, Kind: "position", Position: 8.25}
  if got != want { t.Fatalf("got %#v, want %#v", got, want) }
  ```

- [ ] 运行 `go test ./internal/shell -run 'TestPlaybackEvent' -count=1`，确认测试失败。
- [ ] 实现 `playbackEventFor`、桥接 goroutine 和 shutdown stop channel；无 token 时不 Emit。
- [ ] 增加旧 token/新 token 和 shutdown 测试，确认桥接不会泄漏或阻塞。
- [ ] 运行 `go test ./internal/shell -count=1`，确认通过。

**验证与提交：**

```bash
gofmt -w internal/shell/service.go internal/shell/service_test.go
go test ./internal/shell -count=1
git add internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(playback): bridge mpv events with session tokens"
```

## 任务 4：建立可单测的前端点播自动化纯逻辑

**文件：**

- `frontend/src/playbackAutomation.ts`
- `frontend/src/playbackAutomation.test.ts`

**接口：**

- 输入：`EpisodeInfo` 的 `{ID, Source, Name}` 字段、线路数组和播放信号。
- 产出：`normalizePlaybackSettings`、`nextEpisodeInSource`、`sameNameEpisodeOnSource`、`sourceCandidates` 及 `PlaybackHealthMonitor`，供 `App.vue` 直接调用。

**实现：**

新增不依赖 Vue/Wails 的纯函数和计时器类：

```ts
export interface PlaybackSettings {
  AutoNext: boolean
  AutoSwitchSource: boolean
  PreloadNext: boolean
}

export function normalizePlaybackSettings(value: Partial<PlaybackSettings> | null | undefined): PlaybackSettings
export function nextEpisodeInSource(episodes: readonly EpisodeLike[], source: string, currentID: string): EpisodeLike | null
export function sameNameEpisodeOnSource(episodes: readonly EpisodeLike[], source: string, name: string): EpisodeLike | null
export function sourceCandidates(sources: readonly string[], current: string, attempted: ReadonlySet<string>): string[]
```

`PlaybackHealthMonitor` 负责“未起播/持续缓冲 30 秒”的单次计时：`start()` 开始未起播计时，`signal('playing'|'ready')` 清理计时，`signal('buffering')` 启动缓冲计时，`signal('error')` 立即触发回调，`stop()` 清理所有 timer。每次换源前必须重新创建或重置 monitor。

**步骤：**

- [ ] **先写失败测试**：

  ```ts
  expect(normalizePlaybackSettings({ AutoNext: true })).toEqual({ AutoNext: true, AutoSwitchSource: false, PreloadNext: false })
  expect(nextEpisodeInSource(eps, '线路A', 'ep-1')?.ID).toBe('ep-2')
  expect(sameNameEpisodeOnSource(eps, '线路B', '  第一集 ')?.ID).toBe('b-1')
  expect(sourceCandidates(['A', 'B', 'A', 'C'], 'A', new Set(['B']))).toEqual(['C'])
  ```

- [ ] 运行 `cd frontend && npm test -- --run src/playbackAutomation.test.ts`，确认测试失败。
- [ ] 实现四个纯函数，按 `Source` 过滤、按 `Name.trim()` 完全匹配，并保持线路原始顺序。
- [ ] 实现 `PlaybackHealthMonitor` 的一次性 30 秒未起播/缓冲计时、playing/ready 清理、error 立即回调和 `stop()` 清理。
- [ ] 用 Vitest fake timers 覆盖 30 秒边界及取消场景，运行测试确认通过。

**验证与提交：**

```bash
cd frontend
npm test -- --run src/playbackAutomation.test.ts
cd ..
git add frontend/src/playbackAutomation.ts frontend/src/playbackAutomation.test.ts
git commit -m "feat(playback): add automation decision helpers"
```

## 任务 5：让 PlaybackView 只上报标准播放信号

**文件：**

- `frontend/src/components/PlaybackView.vue`
- `frontend/src/components/PlaybackView.test.ts`

**接口：**

- 输入：现有 `plan`、`seek-to`、`fallback` props/事件，以及原生 video/mpv 状态。
- 产出：`playback(state, message?)` 标准事件；`suppressFallback` 为真时只报告错误、不发旧 `fallback`。

**实现：**

1. 扩展组件事件：`playback(state, message?)`，其中 `state` 为 `playing`、`buffering`、`ready`、`error`、`ended`；保留现有 `progress` 和 `fallback` 兼容行为。
2. Web `<video>` 绑定 `playing`、`waiting`、`stalled`、`canplay`、`error`、`ended`，统一转换为上述事件；所有进度更新继续发出 `progress`。
3. 增加布尔 prop `suppressFallback`。为 `true` 时，HLS/原生视频错误只上报 `playback('error')`，不直接触发旧的 fallback；自动换源由 App 会话协调器统一执行。为 `false` 时保留现有非点播调用方的 fallback 行为。
4. mpv 状态变化也转换为 `playing`、`buffering`、`error`、`ended`，组件不在内部决定切集或换源。

**步骤：**

- [ ] **先写失败测试**，挂载组件后触发 `playing`、`waiting`、`stalled`、`canplay`、`error`、`ended`，断言事件序列为对应标准信号。
- [ ] 运行 `cd frontend && npm test -- --run src/components/PlaybackView.test.ts`，确认新事件断言失败。
- [ ] 扩展 `defineEmits`，绑定 Web 原生事件，并让 mpv 状态映射到同一事件名；保留 progress/fallback。
- [ ] 加入 `suppressFallback` 分支，错误路径只在关闭该 prop 时发 fallback。
- [ ] 增加卸载测试，运行组件测试和 `npm run build`，确认通过。

**验证与提交：**

```bash
cd frontend
npm test -- --run src/components/PlaybackView.test.ts
npm run build
cd ..
git add frontend/src/components/PlaybackView.vue frontend/src/components/PlaybackView.test.ts
git commit -m "feat(playback): normalize web and mpv view signals"
```

## 任务 6：实现后端预载与 Web 隐藏预加载组件

**文件：**

- `internal/playback/controller.go`
- `internal/playback/controller_test.go`
- `internal/playback/proxy.go`
- `internal/playback/proxy_test.go`
- `internal/shell/service.go`
- `internal/shell/service_test.go`
- `frontend/src/components/PreloadView.vue`
- `frontend/src/components/PreloadView.test.ts`
- Wails 生成的 `frontend/bindings/.../shellservice.ts`

**接口：**

- 输入：`playback.Controller.Preload(ctx, input) (Plan, error)`、`Release(id string) error`；前端传入 Web `Plan`。
- 产出：独立 preload plan/session；`PreloadView` 仅发出 `ready`/`error`，不改变当前 `PlaybackView`。

**实现：**

1. 增加 `Controller.Preload(ctx, input)` 和 `Controller.Release(id)`。Web 可播放流注册独立 proxy session；mpv-only 流使用带超时的后台 `GET`/`Range: bytes=0-1` 网络预热，绝不调用共享 `Player.Load/Play`。为 proxy 增加按 URL/token 释放的方法，确保 release 不依赖 TTL。
2. 在 `ShellService` 增加 `PreloadVod(site, episodeID)` 和 `ReleasePreload(id)`，复用现有 provider resolver；解析或预热失败只返回预载错误，不改变当前播放 token/状态。
3. 新建 `PreloadView.vue`：只接收 Web preload plan，使用 `muted`、`preload="auto"` 的隐藏 `<video>`；输入 plan 变化时清理旧 source，卸载时清理元素并触发 release；发出 `ready`/`error` 仅供调试和任务代际判断。

**步骤：**

- [ ] **先写失败 Go 测试**：注册 Web preload 后断言返回独立 URL，调用 `Release` 后 proxy 查找返回未找到；mpv-only 测试 fake player 的 `LoadCalls`/`PlayCalls` 保持为 0。
- [ ] 运行 `go test ./internal/playback ./internal/shell -run 'Test(Preload|Release)' -count=1`，确认测试失败。
- [ ] 实现 `Controller.Preload`、`Release`、proxy release 和 ShellService 两个方法；mpv warmup 使用 context timeout 与 `Range: bytes=0-1`，不触碰共享 player。
- [ ] **先写失败前端测试**：切换 plan/卸载后断言 video source 被清空，旧 promise 完成不触发新任务。
- [ ] 运行 `cd frontend && npm test -- --run src/components/PreloadView.test.ts`，确认失败后实现 `PreloadView.vue`。
- [ ] 运行两套 Go 测试、前端组件测试和 `npm run build`，确认通过。

**验证与提交：**

```bash
gofmt -w internal/playback internal/shell/service.go internal/shell/service_test.go
go test ./internal/playback ./internal/shell -count=1
cd frontend
npm test -- --run src/components/PreloadView.test.ts
npm run build
cd ..
git add internal/playback internal/shell frontend/src/components/PreloadView.vue frontend/src/components/PreloadView.test.ts frontend/bindings
git commit -m "feat(playback): add isolated next-episode preloading"
```

## 任务 7：增加播放设置页面与持久化交互

**文件：**

- `frontend/src/App.vue`
- `frontend/public/style.css`
- `frontend/src/playbackSettings.test.ts`

**接口：**

- 输入：任务 1 生成的 `ShellService.GetPlaybackSettings/SetPlaybackSettings` 绑定。
- 产出：响应式 `playbackSettings`，供点播协调器读取；设置页开关使用 `AutoNext`、`AutoSwitchSource`、`PreloadNext` 三个字段。

**实现：**

1. 在现有设置分类中增加“播放设置”，沿用主题变量、面板边框和个性化分类的开关行；显示“自动切集”“自动换源”“预载下一集”及简短说明，不引入新的固定颜色。
2. 启动刷新和打开设置页时调用 `ShellService.GetPlaybackSettings()`；开关变更立即更新运行时 ref，再调用 `SetPlaybackSettings()`。持久化失败时保留运行时值并调用现有错误提示。
3. 将 `PlaybackSettings` 类型与默认值集中在前端，保证旧后端/缺失返回值归一化为关闭；为点播页把 `suppress-fallback` 绑定到自动换源开关。

**步骤：**

- [ ] **先写失败测试**：mock `GetPlaybackSettings` 返回 `{}`，断言三个开关为 `false`；触发一个开关后断言 `SetPlaybackSettings` 收到完整聚合对象。
- [ ] 运行 `cd frontend && npm test -- --run src/playbackSettings.test.ts`，确认测试失败。
- [ ] 在现有设置分类导航中加入“播放设置”，使用现有主题变量、边框和开关行，不添加固定颜色；接入加载、切换和错误提示。
- [ ] 绑定 `suppress-fallback` 到 `playbackSettings.AutoSwitchSource`，保证关闭自动换源时保留原 fallback。
- [ ] 增加持久化失败测试，运行前端测试与构建确认通过。

**验证与提交：**

```bash
cd frontend
npm test -- --run src/playbackSettings.test.ts
npm run build
cd ..
git add frontend/src/App.vue frontend/public/style.css frontend/src/playbackSettings.test.ts
git commit -m "feat(settings): add playback automation controls"
```

## 任务 8：接入点播会话协调器、自动换源、自动切集和预载

**文件：**

- `frontend/src/App.vue`
- `frontend/src/App.playback.test.ts`（抽取出的协调器单测）

**接口：**

- 输入：任务 4 的 `nextEpisodeInSource`、`sameNameEpisodeOnSource`、`sourceCandidates`、`PlaybackHealthMonitor`；任务 5 的 `playback` 事件；任务 6 的 `PreloadVod/ReleasePreload`。
- 产出：App 中唯一的点播自动化协调流程，更新 `activeSource`、`currentEpisodeID`、`vodPlaybackToken`、当前进度及预载 plan。

**实现：**

1. 在 App 点播状态中增加当前播放位置、已尝试线路集合、健康 monitor、自动任务代际和 preload task 状态；所有异步回调先验证 token/代际。
2. 统一 `doPlayEpisode` 参数，使手动播放、自动切集、手动换源、自动换源都能传入显式续播秒数。开始新播放时清理旧 monitor/preload，更新 `activeSource`、`currentEpisodeID`、历史记录并在成功后恢复 seek。
3. 将 `selectEpisodeSource` 改为同名匹配后走统一切源流程；成功后保持当前集高亮和当前进度。找不到同名集时显示错误且不改变现有播放。
4. 处理 `PlaybackView` 的标准事件：`ended` 调用 `nextEpisodeInSource`（仅在自动切集开启时），`error`/30 秒 monitor 触发 `sourceCandidates` 顺序尝试；每条线路最多一次，全部失败则停止并展示最后错误。
5. 注册 Wails `playback:event`：验证 token 后更新 mpv 进度，映射 playing/buffering/error/ended 到同一 monitor/协调流程；监听器在卸载时移除。Web `progress` 始终更新当前进度，10 秒持久化节流逻辑保持不变。
6. 当前集成功开始后，若自动预载开启且当前线路有下一集，调用 `PreloadVod` 并挂载 `PreloadView`；切集、换源、返回列表、停止或 token 改变时调用 `ReleasePreload`。预载结果只接受当前任务代际，失败仅记录调试信息。
7. 在 `onMounted`/`onBeforeUnmount` 成对管理 mpv 事件监听、进度轮询、健康计时器和预载任务，避免重复监听或定时器泄漏。

**步骤：**

- [ ] **先写失败协调器测试**：使用 mock `ShellService` 和 fake timers，断言自动切集只调用当前线路下一集；无下一集不调用换源。
- [ ] 运行 `cd frontend && npm test -- --run src/App.playback.test.ts`，确认测试失败。
- [ ] 增加当前进度 ref、尝试线路 Set、任务 generation、monitor 和 preload release；所有异步回调先比较 token/generation。
- [ ] 修改统一 `doPlayEpisode`/`selectEpisodeSource`，显式传 seek 秒数并按同名匹配；成功时更新线路、当前集高亮和记录，失败不覆盖当前会话。
- [ ] 接入 `PlaybackView` 的 ended/error/playing/buffering/ready/progress 与 Wails `playback:event`，实现立即错误、30 秒换源和 token 过滤。
- [ ] 接入 `PreloadView` 与 `PreloadVod/ReleasePreload`，在切集、换源、返回列表、停止及卸载时取消旧任务；预载错误只写调试日志。
- [ ] 加入旧 token、手动操作使 generation 失效、重复线路、30 秒边界和预载隔离测试；运行前端测试及 `npm run build` 确认通过。

**验证与提交：**

```bash
cd frontend
npm test -- --run src/App.playback.test.ts
npm run build
cd ..
git add frontend/src/App.vue frontend/src/playbackAutomation.ts frontend/src/App.playback.test.ts
git commit -m "feat(playback): coordinate auto-next and source failover"
```

## 任务 9：更新交接文档并做完整验证

**文件：**

- `docs/HANDOFF.md`
- `README.md`（仅更新确实过时的设置/播放行为说明）

**接口：**

- 输入：任务 1—8 已提交的公开方法、事件名和设置字段。
- 产出：与实际实现一致的交接文档；不新增运行时代码。

**步骤：**

- [ ] **先写文档检查清单**：逐条核对设置默认值、三个 KV 键、30 秒/立即错误触发、同名匹配、Web/mpv 预载差异和 WSLg 注意事项。
- [ ] 更新 `docs/HANDOFF.md` 和确实过时的 `README.md` 段落，删除与新行为冲突的描述。
- [ ] 运行完整验证：

```bash
gofmt -l .
go test ./... -count=1
go vet ./...
CGO_ENABLED=1 go build ./...
cd frontend
npm test -- --run
npm run build
cd ..
git diff --check
git status --short
```

- [ ] 确认 `gofmt -l .` 无输出、Go/前端测试和构建均成功、`git diff --check` 无输出，且所有任务提交已在功能分支、工作区干净；不在本任务中自动合并 `master`。
