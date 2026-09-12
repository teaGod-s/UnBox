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

/** 未起播/持续缓冲多久后触发换源。 */
export const PLAYBACK_STALL_MS = 30_000

/**
 * VodAutomationHost 是协调器与页面之间的接缝：协调器只做决策与编排，
 * 具体播放、状态读写都由页面注入。
 */
export interface VodAutomationHost {
  /** 该 token 是否仍是当前点播会话。 */
  isCurrent(token: number): boolean
  settings(): PlaybackSettings
  currentEpisodeID(): string
  currentEpisodeName(): string
  activeSource(): string
  /** 当前线路的剧集（自动切集只在这条数组里取下一集）。 */
  currentSourceEpisodes(): readonly EpisodeLike[]
  /** 全部线路的剧集（跨线路按同名匹配）。 */
  allEpisodes(): readonly EpisodeLike[]
  sources(): readonly string[]
  /** 当前播放进度（秒），换源续播用。 */
  position(): number
  /** 播放指定剧集，seek 为续播秒数；返回是否成功开始。 */
  playEpisode(episode: EpisodeLike, seek: number): Promise<boolean>
  /** 所有候选线路都失败时上报。 */
  onAllSourcesFailed(): void
}

/**
 * VodAutomation 是点播自动化的唯一协调器：自动切集、自动换源与健康计时。
 * 所有异步回调都先校验任务代际与会话 token，旧会话不能覆盖新会话。
 */
export class VodAutomation {
  private attempted = new Set<string>()
  private monitor: PlaybackHealthMonitor | null = null
  private generation = 0

  constructor(
    private readonly host: VodAutomationHost,
    private readonly stallMs: number = PLAYBACK_STALL_MS,
  ) {}

  /** beginSession 开始一次新的点播播放：重置线路尝试集合与健康计时。 */
  beginSession(): void {
    this.generation++
    this.attempted = new Set()
    this.stopMonitor()
  }

  /** stop 让所有在途自动化任务失效并清理计时器。 */
  stop(): void {
    this.generation++
    this.stopMonitor()
  }

  /** armHealth 为新会话挂上「未起播/持续缓冲 30 秒」计时。 */
  armHealth(token: number): void {
    this.stopMonitor()
    const generation = this.generation
    const monitor = new PlaybackHealthMonitor(this.stallMs, () => {
      if (generation !== this.generation || !this.host.isCurrent(token)) return
      void this.switchSource(token)
    })
    monitor.start()
    this.monitor = monitor
  }

  /** signal 把标准化播放信号喂给健康计时；明确错误立即换源。 */
  signal(token: number, signal: PlaybackSignal): void {
    if (!this.host.isCurrent(token)) return
    if (signal === 'error') {
      if (!this.host.settings().AutoSwitchSource) return
      this.stopMonitor()
      void this.switchSource(token)
      return
    }
    this.monitor?.signal(signal)
  }

  /**
   * ended 处理自然结束：只在开启自动切集、且当前线路还有下一集时续播。
   * 当前线路没有下一集就停下，不跨线路找下一集。
   */
  async ended(token: number): Promise<void> {
    if (!this.host.isCurrent(token)) return
    if (!this.host.settings().AutoNext) return
    const next = nextEpisodeInSource(
      this.host.currentSourceEpisodes(),
      this.host.activeSource(),
      this.host.currentEpisodeID(),
    )
    if (!next) return
    this.stopMonitor()
    await this.host.playEpisode(next, 0)
  }

  /**
   * switchSource 按详情页线路顺序尝试其它线路：只匹配同名剧集，找不到就跳过；
   * 每条线路最多一次；全部失败后上报。
   */
  private async switchSource(token: number): Promise<void> {
    if (!this.host.isCurrent(token)) return
    if (!this.host.settings().AutoSwitchSource) return
    const generation = this.generation
    const name = this.host.currentEpisodeName()
    const seek = this.host.position()
    const candidates = sourceCandidates(this.host.sources(), this.host.activeSource(), this.attempted)
    for (const source of candidates) {
      this.attempted.add(source)
      const episode = sameNameEpisodeOnSource(this.host.allEpisodes(), source, name)
      if (!episode) continue
      const played = await this.host.playEpisode(episode, seek)
      // 切换成功后 token/代际已翻新，先看结果再看代际，避免误判为过期。
      if (played) return
      if (generation !== this.generation || !this.host.isCurrent(token)) return
    }
    if (generation !== this.generation || !this.host.isCurrent(token)) return
    this.host.onAllSourcesFailed()
  }

  private stopMonitor(): void {
    this.monitor?.stop()
    this.monitor = null
  }
}
