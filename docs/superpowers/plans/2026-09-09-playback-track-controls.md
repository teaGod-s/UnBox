# Web 播放器轨道控制实现计划

里程碑：M4 增量（Web 播放器 UX：分辨率 / 音轨 / 字幕）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `PlaybackView` 的 Web 播放路径（hls.js）上，把 hls.js 已有但未暴露的轨道 API——分辨率（`levels`/`currentLevel`）、音轨（`audioTracks`/`audioTrack`）、字幕轨（`subtitleTracks`/`subtitleTrack`）——做成播放器内的「设置」菜单，并支持外挂 srt/vtt 字幕。

**Architecture:** 拆三层：纯逻辑 composable（`useHlsTracks`，从 hls 实例提取轨道状态 + 选择方法）→ 外挂字幕转换工具（`srtToVtt` 纯函数 + 加 TextTrack）→ 菜单 UI 组件（`TrackMenu.vue`）→ `PlaybackView` 接线（设置按钮触发菜单、接外挂字幕入口、加 CSS）。零后端改动、零新依赖，纯前端 UI。

**Tech Stack:** Vue 3 Composition API、hls.js 1.7.1（`frontend/node_modules/hls.js/dist/hls.d.ts`）、Vitest + @vue/test-utils。

**Spec:** 无独立 spec；这是对 M4 Web 播放路径的 UX 增强，背景见 `docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`。

## Global Constraints

- 不引入新 npm 依赖（用 hls.js 既有 API + 浏览器原生 `<track>` / TextTrack API）。
- 不改后端 / Wails 绑定 / 播放路由；纯 `frontend/src/` 改动。
- 只作用于 **Web 后端（`plan.Backend === 'web'` 且 `plan.Kind === 'hls'`）**；mpv 后端与原生 `<video>`（MP4）路径不在此期范围（见「不做」）。
- hls.js API 名已从 1.7.1 类型定义核实（见各 Task 的 Interfaces）。
- 开发走新分支 `feat/playback-track-controls`。
- TDD：每个 composable/工具函数先写失败测试；提交前 `cd frontend && npm test`、`npm run build` 全绿。
- 公开错误信息/注释用中文。
- 现有 `<video controls>` 原生控件保留（它提供播放/暂停/进度/音量）；轨道菜单是**叠加**其上的一个设置按钮 + 弹层，不替换原生控件。

## hls.js 1.7.1 事实（已核实 `hls.d.ts`）

- `get levels(): Level[]`（`Level` 有 `height`/`width`/`bitrate`/`name`/`url`）。
- `get/set currentLevel(): number`（`-1` = 自动 ABR）。
- `get audioTracks(): MediaPlaylist[]`（`MediaPlaylist` 有 `name`/`lang`/`groupId`）。
- `get/set audioTrack(): number`（`-1` 或不存在 = 无）。
- `get subtitleTracks(): MediaPlaylist[]`。
- `get/set subtitleTrack(): number`（`-1` = 关）。
- 事件：`Events.MANIFEST_PARSED`、`Events.AUDIO_TRACKS_UPDATED`、`Events.SUBTITLE_TRACKS_UPDATED`。

---

### Task 1: useHlsTracks composable —— 提取 hls 轨道状态

**Files:**
- Create: `frontend/src/useHlsTracks.ts`
- Test: `frontend/src/useHlsTracks.test.ts`

**Interfaces:**
- Consumes: 一个 `Hls` 实例（`hls.on` / `hls.levels` / `hls.audioTracks` / `hls.subtitleTracks` 等）。
- Produces:
  ```ts
  interface TrackState {
    levels: { index: number; label: string }[]      // [{index:-1,label:'自动'}, {index:0,label:'720p'}, ...]
    currentLevel: number                              // -1 = 自动
    audioTracks: { index: number; label: string }[]
    currentAudio: number
    subtitleTracks: { index: number; label: string }[]
    currentSubtitle: number                           // -1 = 关
    selectLevel(i: number): void
    selectAudio(i: number): void
    selectSubtitle(i: number): void
    detach(): void                                     // 撤销事件监听
  }
  function useHlsTracks(hls: Hls): TrackState
  ```

- [ ] **Step 1: 写失败测试**

`frontend/src/useHlsTracks.test.ts`：构造一个 fake `Hls`（带 `on`/`off`/`levels`/`audioTracks`/`subtitleTracks` getter + `currentLevel`/`audioTrack`/`subtitleTrack` setter），手动触发 `MANIFEST_PARSED` 等事件，断言：
- 事件触发后 `levels` 含「自动」+ fake levels 拼出的列表。
- `selectLevel(0)` 把 fake 的 `currentLevel` 设为 0。
- `audioTracks` 为空数组时不出现「自动」前缀；`subtitleTracks` 为空时列表为空、`currentSubtitle` 为 -1。
- `detach()` 后再触发事件不改变状态（监听已移除）。

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- useHlsTracks`
Expected: FAIL（`useHlsTracks` 未实现）。

- [ ] **Step 3: 实现**

`frontend/src/useHlsTracks.ts`：
- 用 `reactive` 持有 `currentLevel`/`currentAudio`/`currentSubtitle` 与三个 `{index,label}[]`。
- `label` 规则：level = `height ? height+'p' : (name||'未知')`；audio = `name || lang || '音轨'+index`；subtitle 同理；subtitle 列表不含「自动」，但播 `selectSubtitle(-1)` 关闭。
- levels 列表**首项恒为「自动」(index=-1)**，后接真实 levels。
- 监听 `MANIFEST_PARSED` → 刷新 levels；`AUDIO_TRACKS_UPDATED` → 刷新 audioTracks；`SUBTITLE_TRACKS_UPDATED` → 刷新 subtitleTracks。从 hls 实例读当前 getter 同步 `currentXxx`。
- `selectLevel(i)` → `hls.currentLevel = i`（-1 = 自动）；其余同理。
- `detach()` → `hls.off` 三个事件。
- 注意 `reactive` 返回值：state 字段用 `ref` 还是 `reactive` 对象均可，但要保证 Vue 模板能响应式追踪；推荐 `reactive` 对象 + 解构方法保持引用。

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- useHlsTracks`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/useHlsTracks.ts frontend/src/useHlsTracks.test.ts
git commit -m "feat(frontend): useHlsTracks 提取 hls 轨道状态"
```

---

### Task 2: 外挂字幕 srt→vtt 转换 + 加载

**Files:**
- Create: `frontend/src/subtitle.ts`
- Test: `frontend/src/subtitle.test.ts`

**Interfaces:**
- Produces:
  ```ts
  function srtToVtt(srt: string): string  // 时间轴 ','→'.'，前加 'WEBVTT\n\n'
  function addExternalTrack(video: HTMLVideoElement, vttUrl: string, label: string, lang?: string): TextTrack
  function loadSubtitleFile(video: HTMLVideoElement, file: File): Promise<TextTrack>  // 读文件 → srt/vtt 分流 → addExternalTrack
  ```

- [ ] **Step 1: 写失败测试**

`frontend/src/subtitle.test.ts`（纯字符串 + fake video）：
- `srtToVtt('1\n00:00:01,000 --> 00:00:04,000\n你好\n')` → 以 `WEBVTT` 开头，时间轴含 `.` 不含 `,`。
- vtt 文件直接传给 `addExternalTrack` 不走 srt 转换（`loadSubtitleFile` 按 MIME/扩展名分流；测试用 `.vtt` 跳过转换）。
- fake `HTMLVideoElement`（jsdom 的 `<video>` 无 `addTextTrack`，测试里 stub `addTextTrack` 返回假 TextTrack），断言 `addTextTrack` 被以 `('subtitles', label, lang)` 调用。

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- subtitle`
Expected: FAIL。

- [ ] **Step 3: 实现**

`frontend/src/subtitle.ts`：
- `srtToVtt`：split 行，把 `00:00:01,000 --> 00:00:04,000` 的逗号换点（正则 `(\d{2}:\d{2}:\d{2}),(\d{3})` → `$1.$2`），整体前加 `WEBVTT\n\n`。
- `addExternalTrack`：`video.addTextTrack('subtitles', label, lang)` → 返回 TextTrack；同时为兼容某些 webview，可创建 `URL.createObjectURL(new Blob([vtt],{type:'text/vtt'}))` 作为 `<track src>` 的备选——**首版用 `addTextTrack` + `addCue` 不可行（需解析 vtt）**，故首版用 `<track>` 元素注入：在 video 下 append `<track src=vttUrl kind=subtitles label=... srclang=...>`，返回其 `.track`。实现者择一，以「三平台 webview 都能渲染外挂字幕」为验收。
- `loadSubtitleFile`：`file.text()` → 若文件名以 `.vtt` 结尾直接用，`.srt` 走 `srtToVtt` → `URL.createObjectURL(new Blob([vtt],{type:'text/vtt'}))` → `addExternalTrack`。

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- subtitle`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/subtitle.ts frontend/src/subtitle.test.ts
git commit -m "feat(frontend): 外挂字幕 srt→vtt 转换与加载"
```

---

### Task 3: TrackMenu.vue —— 轨道选择菜单

**Files:**
- Create: `frontend/src/components/TrackMenu.vue`
- Test: `frontend/src/components/TrackMenu.test.ts`

**Interfaces:**
- Props: `TrackState`（Task 1 的子集：三个列表 + 三个 current + 三个 select 方法）+ `onLoadSubtitle?: (file: File) => void`。
- 无 emit（选择直接调 prop 的 select 方法）。

- [ ] **Step 1: 写失败测试**

`frontend/src/components/TrackMenu.test.ts`：mount `TrackMenu`，传 stub TrackState，断言：
- 渲染「自动」+ 各分辨率项；当前项有高亮标记（class/aria）。
- 点击一项调用 `selectLevel`。
- 音轨/字幕区在列表非空时渲染、为空时不渲染整块。
- 「加载字幕」按钮存在且点击触发文件选择（`<input type=file>` click）。

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- TrackMenu`
Expected: FAIL。

- [ ] **Step 3: 实现**

`frontend/src/components/TrackMenu.vue`：
- 三段列表（分辨率 / 音轨 / 字幕），每段一个标题 + `<ul>`；当前项加 `aria-current="true"` + class `active`。
- 「加载字幕」入口：隐藏 `<input type="file" accept=".srt,.vtt">`，按钮点击触发其 click，change 时 `emit('load-subtitle', file)`（或直接调 prop `onLoadSubtitle`）。
- 列表为空整段不渲染。
- 菜单根容器 class `track-menu`，点击菜单外部关闭由父组件控制（见 Task 4）。

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- TrackMenu`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/TrackMenu.vue frontend/src/components/TrackMenu.test.ts
git commit -m "feat(frontend): TrackMenu 轨道选择菜单"
```

---

### Task 4: PlaybackView 接线 + CSS + 既有测试更新

**Files:**
- Modify: `frontend/src/components/PlaybackView.vue`
- Modify: `frontend/src/components/PlaybackView.test.ts`（mock 补轨道 API）
- Modify: `frontend/public/style.css`（菜单样式）

**Interfaces:**
- Consumes: `useHlsTracks`（Task 1）、`TrackMenu`（Task 3）、`loadSubtitleFile`（Task 2）。
- Produces: PlaybackView 新增「设置」按钮（齿轮图标，`v-if` 仅 hls 路径显示）+ 弹出 `TrackMenu`。

- [ ] **Step 1: 更新 PlaybackView.test.ts 的 MockHls**

给 `MockHls` 补：`levels`/`audioTracks`/`subtitleTracks` getter（默认空数组）、`currentLevel`/`audioTrack`/`subtitleTrack` setter、`MANIFEST_PARSED`/`AUDIO_TRACKS_UPDATED`/`SUBTITLE_TRACKS_UPDATED` 事件常量 + `off` 方法。现有 11 个用例不应回归。

- [ ] **Step 2: 写失败测试（轨道菜单接线）**

新增用例：
- hls 路径挂载后，`MANIFEST_PARSED` 触发（手动调 fake 的回调，带 levels）→ 「设置」按钮可见，点击后 `TrackMenu` 可见且含 levels 项。
- `Kind='flv'` 时「设置」按钮不渲染（mpegts 无轨道 API）。
- 非 web plan 不渲染按钮。
- 点菜单外关闭（`@click` 冒泡到 document）。

- [ ] **Step 3: 实现 PlaybackView 接线**

`PlaybackView.vue`：
- 在 hls 分支 `new Hls(...)` 后调 `useHlsTracks(hls)` 拿 `trackState`；存为组件局部 ref（非 setup 顶层导出，避免影响既有模板）。
- 模板：`<video>` 旁加 `<button class="track-toggle" v-if="isHls" @click.stop="menuOpen=!menuOpen">⚙</button>`；`<TrackMenu v-if="menuOpen" v-bind="trackState" @load-subtitle="onLoadSubtitle" />`。
- `onLoadSubtitle(file)` → `loadSubtitleFile(video.value, file)`。
- `attach()` cleanup 时 `trackState.detach()` + `menuOpen=false`。
- 菜单外部点击关闭：`onMounted` 加 `document.addEventListener('click', closeMenu)`，`onBeforeUnmount` 移除（或用 `@click.outside` 模式）。

- [ ] **Step 4: CSS**

`frontend/public/style.css`：`.track-toggle`（右上角定位，半透明背景，hover 高亮）+ `.track-menu`（绝对定位弹层，三段列表样式，复用现有 `--btn-bg`/`--border`/`--text` 变量，`max-height` + `overflow-y:auto`）。适配暗/亮主题（用既有 CSS 变量即自动适配）。

- [ ] **Step 5: 运行确认通过 + 生产构建**

Run: `cd frontend && npm test`、`npm run build`
Expected: 全部用例（含新增 + 既有 11 个）PASS；构建通过。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/PlaybackView.vue frontend/src/components/PlaybackView.test.ts frontend/public/style.css
git commit -m "feat(frontend): PlaybackView 接线轨道菜单与外挂字幕"
```

---

## 不做（YAGNI / 留待后续）

- **原生 MP4 多音轨 / ass 字幕**：`<video>.audioTracks` 在 WebKitGTK 支持差、ass 原生不认；这俩靠 mpv 才可靠，属「mpv 内嵌」计划的范畴，不在本期。
- **字幕样式自定义**（字号/颜色/描边）：原生 `<track>` + `::cue` 可做，v2 再议。
- **菜单键盘导航**：首版鼠标点选；v2 补 `aria` + 方向键。
- **记住上次选择**（跨集/跨次记忆音轨/字幕偏好）：需后端 KV，留待后续。
- **替换原生 `<video controls>` 为自定义控制条**：原生控件够用，不重造。

## 风险

- **WebkitGTK 的 `<track>` 渲染**：原生 `<video>` 外挂 vtt 字幕在 WebView2/WKWebView 稳定；WebKitGTK 的 GSubtitle 对 vtt 支持需实测。若某平台不渲染，回退为「字幕菜单仅列 HLS 内嵌轨，外挂入口隐藏」。
- **hls.js `subtitleTrack` 与原生 `<track>` 共存**：两条字幕通路（hls 内嵌 vs 原生外挂）同时启用可能显示两套；首版「选 HLS 内嵌字幕时关闭外挂、反之亦然」，由 `selectSubtitle` 联动控制外挂 track 的 `mode`。
