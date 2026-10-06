import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import { VodAutomation, type PlaybackSettings, type VodAutomationHost } from './playbackAutomation'

const app = readFileSync('src/App.vue', 'utf8')

const ALL_EPISODES = [
  { ID: 'a-1', Source: '线路A', Name: '第一集' },
  { ID: 'a-2', Source: '线路A', Name: '第二集' },
  { ID: 'b-2', Source: '线路B', Name: '第二集' },
  { ID: 'c-2', Source: '线路C', Name: '第二集' },
]

const OFF: PlaybackSettings = { AutoNext: false, AutoSwitchSource: false, PreloadNext: false }

function makeHost(overrides: Partial<VodAutomationHost> = {}) {
  const played: Array<{ id: string; seek: number }> = []
  const failed = vi.fn()
  const host: VodAutomationHost = {
    isCurrent: () => true,
    settings: () => ({ ...OFF }),
    currentEpisodeID: () => 'a-1',
    currentEpisodeName: () => '第一集',
    activeSource: () => '线路A',
    currentSourceEpisodes: () => ALL_EPISODES.filter((e) => e.Source === '线路A'),
    allEpisodes: () => ALL_EPISODES,
    sources: () => ['线路A', '线路B', '线路C'],
    position: () => 42,
    playEpisode: async (episode, seek) => {
      played.push({ id: episode.ID, seek })
      return true
    },
    onAllSourcesFailed: failed,
    ...overrides,
  }
  return { host, played, failed }
}

describe('自动切集', () => {
  it('开启时播放当前线路的下一集', async () => {
    const { host, played } = makeHost({ settings: () => ({ ...OFF, AutoNext: true }) })
    const automation = new VodAutomation(host)
    automation.beginSession()

    await automation.ended(1)
    expect(played).toEqual([{ id: 'a-2', seek: 0 }])
  })

  it('关闭时自然结束不动作', async () => {
    const { host, played } = makeHost()
    const automation = new VodAutomation(host)
    automation.beginSession()

    await automation.ended(1)
    expect(played).toEqual([])
  })

  it('当前线路没有下一集时停下，不跨线路找下一集', async () => {
    const { host, played, failed } = makeHost({
      settings: () => ({ ...OFF, AutoNext: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    await automation.ended(1)
    expect(played).toEqual([])
    expect(failed).not.toHaveBeenCalled()
  })
})

describe('自动换源', () => {
  it('明确错误时按线路顺序切到同名剧集并续接进度', async () => {
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    automation.signal(1, 'error')
    await vi.waitFor(() => expect(played).toHaveLength(1))
    expect(played).toEqual([{ id: 'b-2', seek: 42 }])
  })

  it('目标线路没有同名剧集时跳过该线路', async () => {
    // 线路B 缺少「第二集」，应直接跳到线路C，不用第一集顶替。
    const episodes = ALL_EPISODES.filter((e) => e.ID !== 'b-2')
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
      allEpisodes: () => episodes,
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    automation.signal(1, 'error')
    await vi.waitFor(() => expect(played).toHaveLength(1))
    expect(played).toEqual([{ id: 'c-2', seek: 42 }])
  })

  it('每条其他线路最多尝试一次，全部失败后上报', async () => {
    const { host, played, failed } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
      playEpisode: async () => false,
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    automation.signal(1, 'error')
    await vi.waitFor(() => expect(failed).toHaveBeenCalledTimes(1))
    expect(played).toEqual([])

    // 再报一次错误：线路 B/C 已尝试过，不会重复尝试，只会上报失败。
    automation.signal(1, 'error')
    await vi.waitFor(() => expect(failed).toHaveBeenCalledTimes(2))
  })

  it('关闭自动换源时错误不触发切换', async () => {
    const { host, played } = makeHost({
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    automation.signal(1, 'error')
    await Promise.resolve()
    expect(played).toEqual([])
  })
})

describe('健康计时', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('持续缓冲 30 秒触发换源', async () => {
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()
    automation.armHealth(1)
    automation.signal(1, 'buffering')

    await vi.advanceTimersByTimeAsync(29_999)
    expect(played).toEqual([])
    await vi.advanceTimersByTimeAsync(1)
    expect(played).toEqual([{ id: 'b-2', seek: 42 }])
  })

  it('恢复播放后清除计时，不再误触发', async () => {
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()
    automation.armHealth(1)
    automation.signal(1, 'buffering')
    await vi.advanceTimersByTimeAsync(15_000)
    automation.signal(1, 'playing')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(played).toEqual([])
  })

  it('stop 之后到点的计时器不再触发换源', async () => {
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoSwitchSource: true }),
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()
    automation.armHealth(1)
    automation.signal(1, 'buffering')
    await vi.advanceTimersByTimeAsync(15_000)
    automation.stop()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(played).toEqual([])
  })
})

describe('会话隔离', () => {
  it('旧 token 的错误与结束事件不影响新会话', async () => {
    const { host, played } = makeHost({
      settings: () => ({ ...OFF, AutoNext: true, AutoSwitchSource: true }),
      isCurrent: (token) => token === 2,
      currentEpisodeID: () => 'a-2',
      currentEpisodeName: () => '第二集',
    })
    const automation = new VodAutomation(host)
    automation.beginSession()

    automation.signal(1, 'error')
    await automation.ended(1)
    await Promise.resolve()
    expect(played).toEqual([])
  })
})

describe('App 播放事件接线', () => {
  // @(event) 会被编译成字面 prop "on(event)"，emit 时永远匹配不上——
  // 曾经因此让 web→mpv 降级回调完全失效。断言必须用标准 v-on 简写。
  it('不使用 @(...) 语法，并监听 fallback / playback', () => {
    expect(app).not.toMatch(/@\(/)
    expect(app).toContain('@fallback=')
    expect(app).toContain('@playback=')
  })

  it('点播播放器把标准信号接到协调器', () => {
    expect(app).toContain('onVodPlaybackSignal(vodPlaybackToken, state, message)')
    expect(app).toContain("Events.On('playback:event'")
  })

  it('起播到出画之间显示加载反馈', () => {
    expect(app).toContain('const vodPlayerLoading = ref(false)')
    expect(app).toContain('vodPlayerLoading.value = true')
    expect(app).toContain("if (state === 'playing' || state === 'error') vodPlayerLoading.value = false")
    expect(app).toContain(':loading="vodPlayerLoading"')
  })

  it('跳过片头片尾接入 App', () => {
    expect(app).toContain('GetVodSkipMarks')
    expect(app).toContain('SetVodSkipMarks')
    expect(app).toContain('resolveSkipAction')
    expect(app).toContain('@mark-intro')
    expect(app).toContain('skip-notice')
  })

  // 切集时清空 plan 会卸载 <video>，导致全屏退出、画面黑屏且新流不自动播放。
  it('切换剧集时不预先清空 web 播放计划', () => {
    expect(app).toContain("if (vodPlaybackPlan.value?.Backend !== 'web') vodPlaybackPlan.value = null")
  })

  it('设置页通过捐助榜单弹窗展示，且关于区不再有独立捐助按钮', () => {
    expect(app).toContain('const showDonations = ref(false)')
    expect(app).toContain('GetDonationLeaderboard()')
    expect(app).toContain('openURL(DONATE_URL)')
    expect(app).toContain('donation-leaderboard')

    const aboutStart = app.indexOf('<h3>关于</h3>')
    const aboutEnd = app.indexOf('<h3>日志</h3>', aboutStart)
    const aboutSection = app.slice(aboutStart, aboutEnd)
    expect(aboutSection).toContain('@click="openDonations"')
    expect(aboutSection).not.toContain('@click="openURL(DONATE_URL)"')
  })

  // 播放失败的原因如果只显示在界面上，用户没法把「查看日志」发给我们，只能截图。
  it('播放失败原因同时写入后端日志', () => {
    expect(app).toContain('setVodPlaybackError')
    expect(app).toContain("if (state === 'error' && message) setVodPlaybackError(message)")
    expect(app).toContain("setVodPlaybackError('所有线路均无法播放当前剧集')")
  })

  // 弹窗一打开 Donors 还是空数组，直接按长度判断会把「加载中」显示成「还没人捐助」。
  it('捐助弹窗按拉取状态区分加载中、失败与空态', () => {
    expect(app).toContain("donationViewState === 'loading'")
    expect(app).toContain("donationViewState === 'error'")
    expect(app).toContain('正在加载捐助榜单…')
    expect(app).toContain('加载失败，请稍后重试')
    // 空态文案不能再由 Donors.length 直接决定，否则拉取期间会误报「没人捐助」。
    expect(app).not.toContain('v-if="donationLeaderboard.Donors.length"')
  })
})
