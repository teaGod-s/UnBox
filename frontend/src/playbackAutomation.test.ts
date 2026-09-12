import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  normalizePlaybackSettings,
  nextEpisodeInSource,
  sameNameEpisodeOnSource,
  sourceCandidates,
  PlaybackHealthMonitor,
} from './playbackAutomation'

describe('normalizePlaybackSettings', () => {
  it('defaults missing fields to off', () => {
    expect(normalizePlaybackSettings({ AutoNext: true })).toEqual({
      AutoNext: true,
      AutoSwitchSource: false,
      PreloadNext: false,
    })
    expect(normalizePlaybackSettings(null)).toEqual({
      AutoNext: false,
      AutoSwitchSource: false,
      PreloadNext: false,
    })
    expect(normalizePlaybackSettings(undefined)).toEqual({
      AutoNext: false,
      AutoSwitchSource: false,
      PreloadNext: false,
    })
    expect(normalizePlaybackSettings({})).toEqual({
      AutoNext: false,
      AutoSwitchSource: false,
      PreloadNext: false,
    })
  })
})

const eps = [
  { ID: 'ep-1', Source: '线路A', Name: '第一集' },
  { ID: 'ep-2', Source: '线路A', Name: '第二集' },
  { ID: 'ep-3', Source: '线路A', Name: '第三集' },
  { ID: 'b-1', Source: '线路B', Name: '第一集' },
  { ID: 'b-2', Source: '线路B', Name: '第二集' },
] as const

describe('nextEpisodeInSource', () => {
  it('returns the next episode in the same source only', () => {
    expect(nextEpisodeInSource(eps, '线路A', 'ep-1')?.ID).toBe('ep-2')
    expect(nextEpisodeInSource(eps, '线路A', 'ep-3')).toBeNull()
  })

  it('stays within the source and ignores other sources', () => {
    expect(nextEpisodeInSource(eps, '线路B', 'b-1')?.ID).toBe('b-2')
    expect(nextEpisodeInSource(eps, '线路B', 'b-2')).toBeNull()
  })
})

describe('sameNameEpisodeOnSource', () => {
  it('matches by trimmed exact name on the target source', () => {
    expect(sameNameEpisodeOnSource(eps, '线路B', '  第一集 ')?.ID).toBe('b-1')
  })

  it('returns null when the source has no same-name episode', () => {
    expect(sameNameEpisodeOnSource(eps, '线路B', '第三集')).toBeNull()
    expect(sameNameEpisodeOnSource(eps, '线路B', ' 不存在 ')).toBeNull()
  })
})

describe('sourceCandidates', () => {
  it('keeps order, skips current and already attempted sources', () => {
    expect(sourceCandidates(['A', 'B', 'A', 'C'], 'A', new Set(['B']))).toEqual(['C'])
    expect(sourceCandidates(['A', 'B', 'C'], 'A', new Set())).toEqual(['B', 'C'])
    expect(sourceCandidates(['A', 'B'], 'A', new Set(['B']))).toEqual([])
  })
})

describe('PlaybackHealthMonitor', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('fires after 30s without playback start', () => {
    const cb = vi.fn()
    const m = new PlaybackHealthMonitor(30_000, cb)
    m.start()
    vi.advanceTimersByTime(29_999)
    expect(cb).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(cb).toHaveBeenCalledTimes(1)
    m.stop()
  })

  it('cancels the timer on playing or ready', () => {
    const cb = vi.fn()
    const m = new PlaybackHealthMonitor(30_000, cb)
    m.start()
    m.signal('playing')
    vi.advanceTimersByTime(60_000)
    expect(cb).not.toHaveBeenCalled()
    m.stop()
  })

  it('restarts buffering timer while buffering and cancels on ready', () => {
    const cb = vi.fn()
    const m = new PlaybackHealthMonitor(30_000, cb)
    m.start()
    m.signal('playing')
    m.signal('buffering')
    vi.advanceTimersByTime(15_000)
    m.signal('buffering')
    vi.advanceTimersByTime(29_999)
    expect(cb).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(cb).toHaveBeenCalledTimes(1)
    m.stop()
  })

  it('fires immediately on error', () => {
    const cb = vi.fn()
    const m = new PlaybackHealthMonitor(30_000, cb)
    m.start()
    m.signal('error')
    expect(cb).toHaveBeenCalledTimes(1)
    m.stop()
  })

  it('stop clears pending timers', () => {
    const cb = vi.fn()
    const m = new PlaybackHealthMonitor(30_000, cb)
    m.start()
    m.stop()
    vi.advanceTimersByTime(60_000)
    expect(cb).not.toHaveBeenCalled()
  })
})
