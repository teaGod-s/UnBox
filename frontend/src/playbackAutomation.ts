// playbackAutomation.ts — 点播自动切集/自动换源/预载的纯决策逻辑。
//
// 本模块不依赖 Vue 或 Wails，全部函数与计时器都是可单测的纯 TypeScript，
// App.vue 只负责把播放信号喂进来并按返回值执行协调。

/** EpisodeLike 是剧集的最小形状（EpisodeInfo 的前端投影）。 */
export interface EpisodeLike {
  ID: string
  Source: string
  Name: string
}

/** PlaybackSettings 是三个点播播放自动化开关。 */
export interface PlaybackSettings {
  AutoNext: boolean
  AutoSwitchSource: boolean
  PreloadNext: boolean
}

/** normalizePlaybackSettings 把后端/旧版返回的残缺设置归一化为三开关。 */
export function normalizePlaybackSettings(
  value: Partial<PlaybackSettings> | null | undefined,
): PlaybackSettings {
  return {
    AutoNext: value?.AutoNext === true,
    AutoSwitchSource: value?.AutoSwitchSource === true,
    PreloadNext: value?.PreloadNext === true,
  }
}

/**
 * nextEpisodeInSource 只取当前线路剧集数组内、currentID 之后的下一集；
 * 当前线路没有下一集时返回 null，不跨线路寻找。
 */
export function nextEpisodeInSource(
  episodes: readonly EpisodeLike[],
  source: string,
  currentID: string,
): EpisodeLike | null {
  const inSource = episodes.filter((e) => e.Source === source)
  const idx = inSource.findIndex((e) => e.ID === currentID)
  if (idx < 0 || idx + 1 >= inSource.length) return null
  return inSource[idx + 1]
}

/** sameNameEpisodeOnSource 在目标线路内按 Name.trim() 完全匹配同一集。 */
export function sameNameEpisodeOnSource(
  episodes: readonly EpisodeLike[],
  source: string,
  name: string,
): EpisodeLike | null {
  const want = name.trim()
  // 空白名不构成「同一集」的身份，宁可当作该线路不可用。
  if (want === '') return null
  return episodes.find((e) => e.Source === source && e.Name.trim() === want) ?? null
}

/**
 * sourceCandidates 返回按原顺序、去重后、跳过 current 与已尝试线路的候选。
 */
export function sourceCandidates(
  sources: readonly string[],
  current: string,
  attempted: ReadonlySet<string>,
): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const s of sources) {
    if (s === current || attempted.has(s) || seen.has(s)) continue
    seen.add(s)
    out.push(s)
  }
  return out
}

/** 播放器健康信号。 */
export type PlaybackSignal = 'playing' | 'ready' | 'buffering' | 'error'

/**
 * PlaybackHealthMonitor 负责「连续未起播 / 持续缓冲 30 秒 → 换源」的一次性计时。
 * 每次换源前必须重新创建或重置 monitor。
 *  - start(): 开始未起播计时。
 *  - signal('playing' | 'ready'): 进入可播放状态，清除计时。
 *  - signal('buffering'): 开始或继续缓冲计时；重复信号不重置，
 *    这样「连续缓冲 30 秒」才会真正到点。
 *  - signal('error'): 立即触发回调。
 *  - stop(): 清理所有 timer，之后不再触发。
 */
export class PlaybackHealthMonitor {
  private timeout: ReturnType<typeof setTimeout> | null = null
  private stopped = false
  private fired = false

  constructor(
    private readonly stallMs: number,
    private readonly onStall: () => void,
  ) {}

  start(): void {
    if (this.stopped || this.fired) return
    // 已在计时则继续，避免重复 start 无限延后窗口。
    if (this.timeout === null) this.schedule()
  }

  signal(sig: PlaybackSignal): void {
    if (this.stopped || this.fired) return
    if (sig === 'error') {
      this.fired = true
      this.clear()
      this.onStall()
      return
    }
    if (sig === 'playing' || sig === 'ready') {
      this.clear()
      return
    }
    // buffering：已在计时则继续，不重置到 30 秒。
    if (this.timeout === null) this.schedule()
  }

  stop(): void {
    this.stopped = true
    this.clear()
  }

  private schedule(): void {
    this.clear()
    this.timeout = setTimeout(() => {
      if (this.stopped || this.fired) return
      this.fired = true
      this.onStall()
    }, this.stallMs)
  }

  private clear(): void {
    if (this.timeout !== null) {
      clearTimeout(this.timeout)
      this.timeout = null
    }
  }
}
