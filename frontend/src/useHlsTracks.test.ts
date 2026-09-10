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
  expect(s.levels).toEqual([{ index: -1, label: '自动' }])
  h.handlers[Events.MANIFEST_PARSED]({})
  expect(s.levels).toEqual([
    { index: -1, label: '自动' }, { index: 0, label: '720p' }, { index: 1, label: '1080p' },
  ])
})

it('selectLevel/selectAudio/selectSubtitle 写回 hls 实例', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  h.handlers[Events.MANIFEST_PARSED]({})
  s.selectLevel(1); expect(h.currentLevel).toBe(1)
  expect(s.currentLevel).toBe(1)
  s.selectLevel(-1); expect(h.currentLevel).toBe(-1)
  expect(s.currentLevel).toBe(-1)
  s.selectAudio(1); expect(h.audioTrack).toBe(1)
  expect(s.currentAudio).toBe(1)
  s.selectSubtitle(0); expect(h.subtitleTrack).toBe(0)
  expect(s.currentSubtitle).toBe(0)
  s.selectSubtitle(-1); expect(h.subtitleTrack).toBe(-1)
  expect(s.currentSubtitle).toBe(-1)
})

it('轨道切换事件同步当前选择', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  h.handlers[Events.MANIFEST_PARSED]({})
  h.currentLevel = 1; h.handlers[Events.LEVEL_SWITCHED]({})
  h.audioTrack = 1; h.handlers[Events.AUDIO_TRACK_SWITCHED]({})
  h.subtitleTrack = 0; h.handlers[Events.SUBTITLE_TRACK_SWITCH]({})
  expect(s.currentLevel).toBe(1)
  expect(s.currentAudio).toBe(1)
  expect(s.currentSubtitle).toBe(0)
})

it('音轨和字幕列表更新事件刷新列表', () => {
  const h = fakeHls(); const s = useHlsTracks(h)
  h.audioTracks = [{ name: '英语', lang: 'en' }]
  h.subtitleTracks = [{ name: '繁体', lang: 'zh-Hant' }]
  h.handlers[Events.AUDIO_TRACKS_UPDATED]({})
  h.handlers[Events.SUBTITLE_TRACKS_UPDATED]({})
  expect(s.audioTracks).toEqual([{ index: 0, label: '英语' }])
  expect(s.subtitleTracks).toEqual([{ index: 0, label: '繁体' }])
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
  expect(h.off).toHaveBeenCalledWith(Events.LEVEL_SWITCHED, expect.any(Function))
  expect(h.off).toHaveBeenCalledWith(Events.AUDIO_TRACK_SWITCHED, expect.any(Function))
  expect(h.off).toHaveBeenCalledWith(Events.SUBTITLE_TRACK_SWITCH, expect.any(Function))
})
