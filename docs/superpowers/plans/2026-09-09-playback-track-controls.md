# Web 播放器轨道控制实现计划

里程碑：M4 增量（Web 播放器 UX：分辨率 / 音轨 / 字幕）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `PlaybackView` 的 Web 播放路径（hls.js）上，把 hls.js 已有但未暴露的轨道 API——分辨率（`levels`/`currentLevel`）、音轨（`audioTracks`/`audioTrack`）、字幕轨（`subtitleTracks`/`subtitleTrack`）——做成播放器内的「设置」菜单，并支持加载外挂 srt/vtt 字幕文件。

**Architecture:** 三层拆解：纯逻辑 composable `useHlsTracks`（从 hls 实例提取轨道状态 + 选择方法，事件驱动刷新）→ 外挂字幕工具 `subtitle.ts`（`srtToVtt` 纯函数 + 用原生 `<track>` 注入 `attachSubtitleTrack`/`removeSubtitleTrack`/`loadSubtitleFile`）→ 菜单 UI `TrackMenu.vue`（纯展示，选择走 prop 回调）→ `PlaybackView.vue` 接线（齿轮按钮触发菜单、外挂字幕入口、与内嵌字幕互斥的联动、CSS）。零后端改动、零新依赖。

**Tech Stack:** Vue 3 Composition API、hls.js 1.7.1（`frontend/node_modules/hls.js/dist/hls.d.ts`）、Vitest + @vue/test-utils（jsdom）。

**Spec:** 无独立 spec；这是对 M4 Web 播放路径的 UX 增强，背景见 `docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`。

## Global Constraints

- 不引入新 npm 依赖（用 hls.js 既有 API + 浏览器原生 `<track>`）。
- 不改后端 / Wails 绑定 / 播放路由；纯 `frontend/src/` 改动。
- 只作用于 **Web 后端（`plan.Backend === 'web'` 且 `plan.Kind === 'hls'`）**；mpv 后端与原生 `<video>`（MP4）路径不在范围（见「不做」）。
- hls.js API 名已从 1.7.1 的 `hls.d.ts` 逐条核实（见「hls.js 1.7.1 事实」）。
- 开发走新分支 `feat/playback-track-controls`。
- TDD：每个 composable/工具函数先写失败测试。
- 提交前全套门禁（AGENTS.md，前端-only 改动同样要跑）：`cd frontend && npm test`、`npm run build`、`gofmt -l`、`go vet ./...`、`go test ./... -count=1`、`CGO_ENABLED=1 go build ./...` 全绿。
- **提交/推送需用户点头**（项目约定）。各 Task 末尾的 `git commit` 执行时先把改动 + 测试做绿、`git add` 暂存，待用户确认后再 commit。
- 公开错误信息/注释用中文。
- 现有 `<video controls>` 原生控件保留（提供播放/暂停/进度/音量）；轨道菜单是**叠加**其上的一个设置按钮 + 弹层，不替换原生控件。

## hls.js 1.7.1 事实（已核实 `hls.d.ts`）

- `get levels(): Level[]`（`Level` 有 `height`/`width`/`bitrate`/`name`/`url`，见 d.ts 2042）。
- `get/set currentLevel(): number`（`-1` = 自动 ABR，d.ts 2054/2058）。
- `get audioTracks(): MediaPlaylist[]`（`MediaPlaylist` 有 `name`/`lang`/`groupId`，d.ts 2196）。
- `get/set audioTrack(): number`（d.ts 2200/2204）。
- `get subtitleTracks(): MediaPlaylist[]`（d.ts 2225）。
- `get/set subtitleTrack(): number`（`-1` = 关，d.ts 2229/2234）。
- 事件枚举（d.ts 1397/1405/1411）：`Events.MANIFEST_PARSED = 'hlsManifestParsed'`、`Events.AUDIO_TRACKS_UPDATED = 'hlsAudioTracksUpdated'`、`Events.SUBTITLE_TRACKS_UPDATED = 'hlsSubtitleTracksUpdated'`。
- `hls.on(event, handler)` / `hls.off(event, handler)`：handler 必须是**命名函数**才能 `off` 掉，匿名函数 off 不掉。

---

### Task 1: useHlsTracks composable —— 提取 hls 轨道状态

**Files:**
- Create: `frontend/src/useHlsTracks.ts`
- Test: `frontend/src/useHlsTracks.test.ts`

**Interfaces:**
- Consumes: 一个 `Hls` 实例（`hls.on`/`hls.off`/`hls.levels`/`hls.audioTracks`/`hls.subtitleTracks`/`hls.currentLevel`/`hls.audioTrack`/`hls.subtitleTrack`）。
- Produces:
  ```ts
  export interface TrackItem { index: number; label: string }
  export interface TrackState {
    levels: TrackItem[]        // 首项恒为 { index: -1, label: '自动' }
    currentLevel: number       // -1 = 自动
    audioTracks: TrackItem[]   // 空时不含任何前缀项
    currentAudio: number
    subtitleTracks: TrackItem[] // 空时为空；不含「自动」
    currentSubtitle: number    // -1 = 关
    selectLevel(i: number): void
    selectAudio(i: number): void
    selectSubtitle(i: number): void
    detach(): void             // hls.off 三个事件
  }
  export function useHlsTracks(hls: Hls): TrackState
  ```
  实现从 `hls.js` 引入：`import { Events } from 'hls.js'`（值枚举）+ `import type Hls from 'hls.js'`（类型）。事件名用 `Events.MANIFEST_PARSED` 等，不手写字符串。

- [ ] **Step 1: 写失败测试**

`frontend/src/useHlsTracks.test.ts`。**不 mock hls.js 模块**（`Events` 直接从真包引入即可），只构造一个 fake hls 对象（`on`/`off` 用 `vi.fn` 存回调，getter/setter 用普通对象）。用例：

```ts
import { describe, expect, it, vi } from 'vitest'
import { Events } from 'hls.js'
import { useHlsTracks } from './useHlsTracks'

function fakeHls() {
  const handlers: Record<string, Function> = {}
  const h = {
    on: vi.fn((e: string, cb: Function) => { handlers[e] = cb }),
    off: vi.fn((e: string) => { delete handlers[e] }),
    levels: [{ height: 720, bitrate: 2_800_000, name: '720p' }, { height: 1080, bitrate: 5_800_000, name: '1080p' }],
    currentLevel: -1,
    audioTracks: [{ name: '国语', lang: 'zh' }, { name: '粤语', lang: 'yue' }],
    audioTrack: 0,
    subtitleTracks: [{ name: '简体', lang: 'zh-Hans' }],
    subtitleTrack: -1,
    handlers,
  }
  return h as any
}

it('levels 首项恒为「自动」，事件触发后刷新为真实 levels', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  expect(s.levels).toEqual([{ index: -1, label: '自动' }])       // 事件前只有自动
  h.handlers[Events.MANIFEST_PARSED]({})                          // 模拟事件
  expect(s.levels).toEqual([
    { index: -1, label: '自动' }, { index: 0, label: '720p' }, { index: 1, label: '1080p' },
  ])
})

it('selectLevel/selectAudio/selectSubtitle 写回 hls 实例', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  h.handlers[Events.MANIFEST_PARSED]({})
  s.selectLevel(1); expect(h.currentLevel).toBe(1)
  s.selectLevel(-1); expect(h.currentLevel).toBe(-1)
  s.selectAudio(1); expect(h.audioTrack).toBe(1)
  s.selectSubtitle(0); expect(h.subtitleTrack).toBe(0)
  s.selectSubtitle(-1); expect(h.subtitleTrack).toBe(-1)
})

it('音轨/字幕空列表不带「自动」前缀，字幕默认 -1', () => {
  const h = fakeHls(); h.audioTracks = []; h.subtitleTracks = []
  const s = useHlsTracks(h)
  h.handlers[Events.MANIFEST_PARSED]({})
  expect(s.audioTracks).toEqual([])
  expect(s.subtitleTracks).toEqual([])
  expect(s.currentSubtitle).toBe(-1)
})

it('detach 后撤销三个事件监听', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  s.detach()
  expect(h.off).toHaveBeenCalledWith(Events.MANIFEST_PARSED, expect.any(Function))
  expect(h.off).toHaveBeenCalledWith(Events.AUDIO_TRACKS_UPDATED, expect.any(Function))
  expect(h.off).toHaveBeenCalledWith(Events.SUBTITLE_TRACKS_UPDATED, expect.any(Function))
})
```

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- useHlsTracks`
Expected: FAIL（`useHlsTracks` 未实现 / 模块不存在）。

- [ ] **Step 3: 实现**

`frontend/src/useHlsTracks.ts`：

```ts
import { Events } from 'hls.js'
import type Hls from 'hls.js'
import { reactive } from 'vue'

export interface TrackItem { index: number; label: string }
export interface TrackState { /* 见 Interfaces */ }

function levelLabel(l: any, i: number): string {
  if (l.height) return `${l.height}p`
  if (l.name) return l.name
  return `码率 ${Math.round((l.bitrate || 0) / 1000)}k`  // 无 height/name 时用码率兜底，避免多个「未知」
}

export function useHlsTracks(hls: Hls): TrackState {
  const state = reactive({
    levels: [{ index: -1, label: '自动' }] as TrackItem[],
    currentLevel: -1,
    audioTracks: [] as TrackItem[],
    currentAudio: -1,
    subtitleTracks: [] as TrackItem[],
    currentSubtitle: -1,
  })

  function refreshLevels() {
    state.levels = [{ index: -1, label: '自动' }, ...hls.levels.map((l, i) => ({ index: i, label: levelLabel(l, i) }))]
    state.currentLevel = hls.currentLevel
  }
  function refreshAudio() {
    state.audioTracks = hls.audioTracks.map((t, i) => ({ index: i, label: t.name || t.lang || `音轨${i + 1}` }))
    state.currentAudio = hls.audioTrack
  }
  function refreshSubtitle() {
    state.subtitleTracks = hls.subtitleTracks.map((t, i) => ({ index: i, label: t.name || t.lang || `字幕${i + 1}` }))
    state.currentSubtitle = hls.subtitleTrack
  }

  // 命名函数，detach 时才能 off 掉
  function onManifest() { refreshLevels(); refreshAudio(); refreshSubtitle() }
  function onAudioTracks() { refreshAudio() }
  function onSubtitleTracks() { refreshSubtitle() }

  hls.on(Events.MANIFEST_PARSED, onManifest)
  hls.on(Events.AUDIO_TRACKS_UPDATED, onAudioTracks)
  hls.on(Events.SUBTITLE_TRACKS_UPDATED, onSubtitleTracks)

  return Object.assign(state, {
    selectLevel(i: number) { hls.currentLevel = i },
    selectAudio(i: number) { hls.audioTrack = i },
    selectSubtitle(i: number) { hls.subtitleTrack = i },
    detach() {
      hls.off(Events.MANIFEST_PARSED, onManifest)
      hls.off(Events.AUDIO_TRACKS_UPDATED, onAudioTracks)
      hls.off(Events.SUBTITLE_TRACKS_UPDATED, onSubtitleTracks)
    },
  })
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- useHlsTracks`
Expected: PASS。

- [ ] **Step 5: 暂存（提交待用户点头）**

```bash
git add frontend/src/useHlsTracks.ts frontend/src/useHlsTracks.test.ts
# 待用户确认后：git commit -m "feat(frontend): useHlsTracks 提取 hls 轨道状态"
```

---

### Task 2: 外挂字幕 srt→vtt 转换 + 原生 `<track>` 注入

**Files:**
- Create: `frontend/src/subtitle.ts`
- Test: `frontend/src/subtitle.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export function srtToVtt(srt: string): string
  // 把 vtt 文本包成 blob URL，注入 <track kind="subtitles"> 到 video，返回该 track 元素。
  export function attachSubtitleTrack(video: HTMLVideoElement, vtt: string, label: string, lang?: string): HTMLTrackElement
  // 移除 track 元素并 revoke 其 blob URL（el.remove() 已能从 DOM 摘除，无需父节点参数）。
  export function removeSubtitleTrack(el: HTMLTrackElement | null): void
  // 读文件 → .srt 走 srtToVtt、.vtt 直接用 → attachSubtitleTrack；失败返回 null 并 console.warn（中文）。
  export async function loadSubtitleFile(video: HTMLVideoElement, file: File): Promise<HTMLTrackElement | null>
  ```
  锁定机制：**用原生 `<track>` 元素注入**（`URL.createObjectURL(new Blob([vtt], {type:'text/vtt'}))` → `document.createElement('track')` → `video.appendChild`）。不用 `addTextTrack`（它要求已经解析好的 VTT cue，超出本任务范围）。返回 `HTMLTrackElement`（调用方需要它来 `removeSubtitleTrack`，不是 TextTrack）。

- [ ] **Step 1: 写失败测试**

`frontend/src/subtitle.test.ts`。jsdom 没有 `URL.createObjectURL`/`revokeObjectURL`，测试里统一 stub：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { srtToVtt, attachSubtitleTrack, removeSubtitleTrack, loadSubtitleFile } from './subtitle'

beforeEach(() => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:fake'), revokeObjectURL: vi.fn() })
})

it('srtToVtt 时间轴逗号换点、前缀 WEBVTT', () => {
  const out = srtToVtt('1\n00:00:01,000 --> 00:00:04,000\n你好\n')
  expect(out.startsWith('WEBVTT\n\n')).toBe(true)
  expect(out).toContain('00:00:01.000 --> 00:00:04.000')
  expect(out).not.toContain(',000 -->')
})

it('attachSubtitleTrack 注入 <track kind=subtitles> 并返回元素', () => {
  const video = { appendChild: vi.fn() } as any
  const el = attachSubtitleTrack(video, 'WEBVTT\n\n', '外挂', 'zh')
  expect(video.appendChild).toHaveBeenCalledTimes(1)
  const track = (video.appendChild as any).mock.calls[0][0]
  expect(track.kind).toBe('subtitles')
  expect(track.src).toBe('blob:fake')
  expect(track.label).toBe('外挂')
  expect(track.srclang).toBe('zh')
  expect(el).toBe(track)
})

it('removeSubtitleTrack 移除元素并 revoke blob URL', () => {
  const video = { appendChild: vi.fn() } as any
  const el = attachSubtitleTrack(video, 'WEBVTT\n\n', '外挂')
  ;(el as any).remove = vi.fn()
  removeSubtitleTrack(el)
  expect((el as any).remove).toHaveBeenCalled()
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fake')
})

it('loadSubtitleFile：.srt 转 vtt、.vtt 直用', async () => {
  const video = { appendChild: vi.fn() } as any
  const mkFile = (name: string, text: string) => ({ name, text: () => Promise.resolve(text) }) as unknown as File
  const srtEl = await loadSubtitleFile(video, mkFile('a.srt', '1\n00:00:01,000 --> 00:00:02,000\nx\n'))
  expect(srtEl).not.toBeNull()
  const vttEl = await loadSubtitleFile(video, mkFile('b.vtt', 'WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nx\n'))
  expect(vttEl).not.toBeNull()
})
```

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- subtitle`
Expected: FAIL（`subtitle.ts` 不存在）。

- [ ] **Step 3: 实现**

`frontend/src/subtitle.ts`：

```ts
// 把 srt 时间轴逗号换成点，前面加 WEBVTT 头，交给原生 <track> 解析。
export function srtToVtt(srt: string): string {
  const body = srt.replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, '$1.$2')
  return `WEBVTT\n\n${body}`
}

export function attachSubtitleTrack(video: HTMLVideoElement, vtt: string, label: string, lang?: string): HTMLTrackElement {
  const url = URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' }))
  const el = document.createElement('track')
  el.kind = 'subtitles'
  el.src = url
  el.label = label
  if (lang) el.srclang = lang
  el.default = true   // 让 webview 默认显示这条外挂字幕
  video.appendChild(el)
  return el
}

export function removeSubtitleTrack(el: HTMLTrackElement | null): void {
  if (!el) return
  if (el.src?.startsWith('blob:')) URL.revokeObjectURL(el.src)
  el.remove()
}

export async function loadSubtitleFile(video: HTMLVideoElement, file: File): Promise<HTMLTrackElement | null> {
  try {
    const text = await file.text()
    const vtt = file.name.toLowerCase().endsWith('.srt') ? srtToVtt(text) : text
    return attachSubtitleTrack(video, vtt, file.name)
  } catch (e) {
    console.warn('[subtitle] 读取字幕文件失败：', e)
    return null
  }
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- subtitle`
Expected: PASS。

- [ ] **Step 5: 暂存（提交待用户点头）**

```bash
git add frontend/src/subtitle.ts frontend/src/subtitle.test.ts
# 待用户确认后：git commit -m "feat(frontend): 外挂字幕 srt→vtt 转换与 <track> 注入"
```

---

### Task 3: TrackMenu.vue —— 轨道选择菜单

**Files:**
- Create: `frontend/src/components/TrackMenu.vue`
- Test: `frontend/src/components/TrackMenu.test.ts`

**Interfaces:**
- Props（**纯 prop 回调，无 emit**——与 Task 4 的绑定方式一致）：
  ```ts
  defineProps<{
    levels: TrackItem[]; currentLevel: number
    audioTracks: TrackItem[]; currentAudio: number
    subtitleTracks: TrackItem[]; currentSubtitle: number
    selectLevel(i: number): void
    selectAudio(i: number): void
    selectSubtitle(i: number): void
    onLoadSubtitle(file: File): void
  }>()
  ```
  `TrackItem` 从 `../useHlsTracks` 导入（`import type { TrackItem } from '../useHlsTracks'`）。
- 无 emit；点击选择直接调 prop 的 select 方法；文件选择调 `onLoadSubtitle`。

- [ ] **Step 1: 写失败测试**

`frontend/src/components/TrackMenu.test.ts`：mount `TrackMenu`，传 stub TrackState，断言：

```ts
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TrackMenu from './TrackMenu.vue'

function stubProps() {
  return {
    levels: [{ index: -1, label: '自动' }, { index: 0, label: '720p' }],
    currentLevel: 0,
    audioTracks: [{ index: 0, label: '国语' }],
    currentAudio: 0,
    subtitleTracks: [{ index: 0, label: '简体' }],
    currentSubtitle: -1,
    selectLevel: vi.fn(), selectAudio: vi.fn(), selectSubtitle: vi.fn(), onLoadSubtitle: vi.fn(),
  }
}

it('渲染「自动」+ 各分辨率项，当前项带 aria-current', () => {
  const w = mount(TrackMenu, { props: stubProps() })
  expect(w.text()).toContain('自动')
  expect(w.text()).toContain('720p')
  expect(w.find('[aria-current="true"]').exists()).toBe(true)
})

it('点击一项调用对应 select 方法', async () => {
  const p = stubProps(); const w = mount(TrackMenu, { props: p })
  const items = w.findAll('li'); await items[1].trigger('click')
  expect(p.selectLevel).toHaveBeenCalledWith(0)
})

it('音轨/字幕区列表为空时不渲染整块', () => {
  const p = stubProps(); p.audioTracks = []; p.subtitleTracks = []
  const w = mount(TrackMenu, { props: p })
  expect(w.text()).not.toContain('音轨')
  expect(w.text()).not.toContain('字幕')
})

it('「加载字幕」按钮触发隐藏 file input 的 click', async () => {
  const w = mount(TrackMenu, { props: stubProps() })
  const input = w.find('input[type="file"]')
  const click = vi.spyOn(input.element, 'click').mockImplementation(() => {})
  await w.find('.load-subtitle').trigger('click')
  expect(click).toHaveBeenCalled()
})
```

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- TrackMenu`
Expected: FAIL（组件不存在）。

- [ ] **Step 3: 实现**

`frontend/src/components/TrackMenu.vue`：

```vue
<script setup lang="ts">
import type { TrackItem } from '../useHlsTracks'
const props = defineProps<{ /* 见 Interfaces */ }>()

const fileInput = ref<HTMLInputElement | null>(null)
function pickSubtitle() { fileInput.value?.click() }
function onFileChange(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) props.onLoadSubtitle(f)
  ;(e.target as HTMLInputElement).value = ''  // 允许重复选同一文件
}
</script>

<template>
  <div class="track-menu">
    <section v-if="levels.length" class="track-sec">
      <h4>清晰度</h4>
      <ul>
        <li v-for="l in levels" :key="l.index" :aria-current="l.index === currentLevel ? 'true' : undefined"
            :class="{ active: l.index === currentLevel }" @click="selectLevel(l.index)">{{ l.label }}</li>
      </ul>
    </section>
    <section v-if="audioTracks.length" class="track-sec">
      <h4>音轨</h4>
      <ul>
        <li v-for="t in audioTracks" :key="t.index" :aria-current="t.index === currentAudio ? 'true' : undefined"
            :class="{ active: t.index === currentAudio }" @click="selectAudio(t.index)">{{ t.label }}</li>
      </ul>
    </section>
    <section v-if="subtitleTracks.length" class="track-sec">
      <h4>字幕</h4>
      <ul>
        <li v-for="t in subtitleTracks" :key="t.index" :aria-current="t.index === currentSubtitle ? 'true' : undefined"
            :class="{ active: t.index === currentSubtitle }" @click="selectSubtitle(t.index)">{{ t.label }}</li>
      </ul>
    </section>
    <button class="load-subtitle" @click="pickSubtitle">加载字幕…</button>
    <input ref="fileInput" type="file" accept=".srt,.vtt" hidden @change="onFileChange" />
  </div>
</template>
```

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- TrackMenu`
Expected: PASS。

- [ ] **Step 5: 暂存（提交待用户点头）**

```bash
git add frontend/src/components/TrackMenu.vue frontend/src/components/TrackMenu.test.ts
# 待用户确认后：git commit -m "feat(frontend): TrackMenu 轨道选择菜单"
```

---

### Task 4: PlaybackView 接线 + CSS + MockHls 扩展

**Files:**
- Modify: `frontend/src/components/PlaybackView.vue`
- Modify: `frontend/src/components/PlaybackView.test.ts`（mock 补轨道 API + 新增接线用例；**既有 14 个用例不得回归**）
- Modify: `frontend/public/style.css`（菜单样式）

**Interfaces:**
- Consumes: `useHlsTracks`（Task 1）、`TrackMenu`（Task 3）、`loadSubtitleFile`/`removeSubtitleTrack`（Task 2）。
- Produces: PlaybackView 在 hls 路径新增「⚙」按钮 + `TrackMenu` 弹层；外挂字幕加载与内嵌字幕互斥联动。

- [ ] **Step 1: 扩展 MockHls（关键：保留既有 ERROR 通道）**

现状：`MockHls.on` 只对 ERROR 存回调（`if (event === ERROR) this.error = cb`），其他事件直接丢弃，也没有 `off`、没有 `handlers` map。**必须先让 `on` 能存非 ERROR 事件的回调，才能测 MANIFEST_PARSED 刷新**。改造（保持 `this.error = cb` 不动，14 个既有用例依赖它）：

```ts
vi.mock('hls.js', () => {
  class MockHls {
    static Events = {
      ERROR: 'hlsError',
      MANIFEST_PARSED: 'hlsManifestParsed',
      AUDIO_TRACKS_UPDATED: 'hlsAudioTracksUpdated',
      SUBTITLE_TRACKS_UPDATED: 'hlsSubtitleTracksUpdated',
    }
    static ErrorTypes = { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError', OTHER_ERROR: 'otherError' }
    static ErrorDetails = { ATTACH_MEDIA_ERROR: 'attachMediaError' }
    static isSupported = () => true
    handlers: Record<string, Function> = {}
    levels: any[] = []
    currentLevel = -1
    audioTracks: any[] = []
    audioTrack = -1
    subtitleTracks: any[] = []
    subtitleTrack = -1
    on = vi.fn((event: string, cb: Function) => {
      if (event === MockHls.Events.ERROR) (this as any).error = cb
      else this.handlers[event] = cb
    })
    off = vi.fn((event: string) => { delete this.handlers[event] })
    loadSource = vi.fn(); attachMedia = vi.fn(); startLoad = vi.fn()
    recoverMediaError = vi.fn(); destroy = vi.fn()
    constructor() { instances.hls.push(this) }
  }
  return { default: MockHls }
})
```

`instances.hls` 数组已有（`instances` 是 `vi.hoisted` 的），新用例从中取最新实例，触发 `instances.hls[0].handlers['hlsManifestParsed']({ levels: [...] })` 之类。

- [ ] **Step 2: 写失败测试（接线）**

`PlaybackView.test.ts` 新增用例（注意 `hls` 实例经 `vi.hoisted` 的 `instances.hls` 捕获，见文件头）：

```ts
const plan = (over: Partial<PlaybackPlan> = {}): PlaybackPlan => ({
  ID: 'x', Backend: 'web', URL: 'http://x/a.m3u8', Kind: 'hls', CanFallback: true, ...over,
})

it('hls 路径：manifest 后显示设置按钮，点开菜单含清晰度项', async () => {
  const w = mount(PlaybackView, { props: { plan: plan() } })
  await nextTick()
  const hls = instances.hls[instances.hls.length - 1]
  hls.levels = [{ height: 720, bitrate: 2_800_000 }]
  hls.handlers['hlsManifestParsed']({})
  await nextTick()
  expect(w.find('.track-toggle').exists()).toBe(true)
  await w.find('.track-toggle').trigger('click')
  expect(w.find('.track-menu').exists()).toBe(true)
  expect(w.text()).toContain('720p')
})

it('flv 路径不渲染设置按钮', async () => {
  const w = mount(PlaybackView, { props: { plan: plan({ Kind: 'flv' }) } })
  await nextTick()
  expect(w.find('.track-toggle').exists()).toBe(false)
})

it('mpv 后端不渲染设置按钮', async () => {
  const w = mount(PlaybackView, { props: { plan: plan({ Backend: 'mpv' }) } })
  await nextTick()
  expect(w.find('.track-toggle').exists()).toBe(false)
})
```

- [ ] **Step 3: 实现 PlaybackView 接线**

`PlaybackView.vue`（`<script setup>` 内新增，不改动既有 `attach`/`cleanup` 的现有逻辑行）：

```ts
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'   // 加 computed / onMounted
import { useHlsTracks, type TrackState } from '../useHlsTracks'
import TrackMenu from './TrackMenu.vue'
import { loadSubtitleFile, removeSubtitleTrack } from '../subtitle'

const trackState = ref<TrackState | null>(null)
const menuOpen = ref(false)
let externalTrackEl: HTMLTrackElement | null = null
const isHls = computed(() => props.plan?.Backend === 'web' && props.plan?.Kind === 'hls' && Hls.isSupported())
```

在 `attach()` 的 hls 分支（`new Hls(...)` 之后）创建 composable，并在 `cleanup()` 里 detach/清空：

```ts
// attach() 内 hls 分支末尾（loadSource/attachMedia 之后，return 之前）：
trackState.value = useHlsTracks(hls)

// cleanup() 开头（hls?.destroy() 之前）：
trackState.value?.detach(); trackState.value = null
removeSubtitleTrack(externalTrackEl); externalTrackEl = null
menuOpen.value = false
```

模板接线（`<video>` 旁加按钮；菜单显式绑定，**不 `v-bind` 整个 trackState**，避免 `detach` 函数漏成 HTML 属性）：

```html
<button v-if="isHls" class="track-toggle" @click.stop="menuOpen = !menuOpen">⚙</button>
<TrackMenu v-if="isHls && menuOpen && trackState"
  :levels="trackState.levels" :current-level="trackState.currentLevel"
  :audio-tracks="trackState.audioTracks" :current-audio="trackState.currentAudio"
  :subtitle-tracks="trackState.subtitleTracks" :current-subtitle="trackState.currentSubtitle"
  :select-level="trackState.selectLevel" :select-audio="trackState.selectAudio"
  :select-subtitle="onSelectSubtitle" :on-load-subtitle="onLoadSubtitle" />
```

外挂/内嵌字幕互斥联动（两个函数）：

```ts
function onSelectSubtitle(i: number) {
  if (i !== -1) { removeSubtitleTrack(externalTrackEl); externalTrackEl = null }
  trackState.value?.selectSubtitle(i)
}
async function onLoadSubtitle(file: File) {
  const el = video.value; if (!el) return
  removeSubtitleTrack(externalTrackEl); externalTrackEl = null
  externalTrackEl = await loadSubtitleFile(el, file)
  trackState.value?.selectSubtitle(-1)   // 挂外挂时关掉内嵌，避免双字幕
}
```

菜单外部点击关闭：

```ts
function onClickDoc(e: MouseEvent) {
  if (menuOpen.value && !(e.target as HTMLElement).closest('.track-menu, .track-toggle')) menuOpen.value = false
}
onMounted(() => document.addEventListener('click', onClickDoc))
onBeforeUnmount(() => document.removeEventListener('click', onClickDoc))
```

> 说明：`cleanup()` 会先于每次 `attach()` 运行（`watch(() => props.plan, attach, { immediate: true })`），所以换集/换源时 `trackState` 先 detach 清空、再由 hls 分支重建；`onBeforeUnmount(cleanup)` 兜底卸载。`trackState.value` 为空时菜单按钮 `v-if="isHls"` 仍显示但 `TrackMenu` 因 `trackState` 为空不渲染——用 `isHls && trackState` 门控可避免 manifest 未到时空菜单。

- [ ] **Step 4: CSS**

`frontend/public/style.css`（`.playback-view` 已 `position: relative; overflow: hidden`，可直接绝对定位子元素）：

```css
.track-toggle {
  position: absolute; top: 0.5rem; right: 0.5rem; z-index: 3;
  width: 2rem; height: 2rem; border-radius: 0.35rem;
  border: 1px solid var(--border); background: var(--btn-bg); color: var(--text);
  cursor: pointer; line-height: 1;
}
.track-toggle:hover { background: var(--btn-hover); }

.track-menu {
  position: absolute; right: 0.5rem; bottom: 3rem; z-index: 3;
  min-width: 11rem; max-height: 70%; overflow-y: auto;
  padding: 0.5rem; border-radius: 0.5rem;
  background: var(--panel); border: 1px solid var(--panel-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.35);
}
.track-menu .track-sec { margin-bottom: 0.4rem; }
.track-menu h4 { margin: 0.2rem 0 0.3rem; font-size: 0.72rem; color: var(--text-dim); }
.track-menu ul { list-style: none; margin: 0; padding: 0; }
.track-menu li {
  padding: 0.3rem 0.5rem; border-radius: 0.25rem; cursor: pointer;
  color: var(--text-soft); font-size: 0.82rem;
}
.track-menu li:hover { background: var(--btn-bg); }
.track-menu li.active { color: var(--accent); background: var(--btn-active-bg); }
.track-menu .load-subtitle {
  width: 100%; margin-top: 0.3rem; padding: 0.35rem;
  border: 1px solid var(--border); border-radius: 0.3rem;
  background: var(--btn-bg); color: var(--text); cursor: pointer;
}
```

用既有 CSS 变量（`--panel`/`--border`/`--text`/`--accent`/`--btn-bg` 等），暗/亮/木纹主题自动适配。

- [ ] **Step 5: 运行确认通过 + 生产构建 + Go 门禁**

Run:
```bash
cd frontend && npm test && npm run build
cd .. && gofmt -l . && go vet ./... && go test ./... -count=1 && CGO_ENABLED=1 go build ./...
```
Expected: 全部用例（新增 + 既有 14 个）PASS；构建通过；Go 门禁全绿。

- [ ] **Step 6: 暂存（提交待用户点头）**

```bash
git add frontend/src/components/PlaybackView.vue frontend/src/components/PlaybackView.test.ts frontend/public/style.css
# 待用户确认后：git commit -m "feat(frontend): PlaybackView 接线轨道菜单与外挂字幕"
```

---

## 不做（YAGNI / 留待后续）

- **原生 MP4 多音轨 / ass 字幕**：`<video>.audioTracks` 在 WebKitGTK 支持差、ass 原生不认；这俩靠 mpv 才可靠，属「mpv 内嵌」计划的范畴，不在本期。
- **字幕样式自定义**（字号/颜色/描边）：原生 `<track>` + `::cue` 可做，v2 再议。
- **菜单键盘导航**：首版鼠标点选；v2 补方向键与完整 `aria`。
- **记住上次选择**（跨集/跨次记忆音轨/字幕偏好）：需后端 KV，留待后续。
- **替换原生 `<video controls>` 为自定义控制条**：原生控件够用，不重造。

## 风险

- **WebkitGTK 的 `<track>` 渲染**：原生 `<video>` 外挂 vtt 在 WebView2/WKWebView 稳定；WebKitGTK 对 vtt 字幕支持需实测。若某平台不渲染，回退为「字幕菜单仅列 HLS 内嵌轨，外挂入口隐藏」。
- **hls.js `subtitleTrack` 与原生 `<track>` 共存**：两条字幕通路（hls 内嵌 vs 原生外挂）同时启用会显示两套。本计划已做互斥：选内嵌字幕时移除外挂（`onSelectSubtitle`），挂外挂时关内嵌（`onLoadSubtitle` 里 `selectSubtitle(-1)`）。
- **`Hls.isSupported()` = false 时**：`isHls` 计算为 false，按钮不渲染；此时 `attach()` 走 `element.src = plan.URL` 原生回退，无轨道菜单（正确）。
