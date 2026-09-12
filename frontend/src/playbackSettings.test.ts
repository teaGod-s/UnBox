import { describe, expect, it, vi } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'
import {
  createPlaybackSettings,
  PLAYBACK_SETTING_ITEMS,
  type PlaybackSettingsApi,
} from './playbackSettings'

const app = readFileSync('src/App.vue', 'utf8')

function api(
  get: PlaybackSettingsApi['GetPlaybackSettings'],
  set: PlaybackSettingsApi['SetPlaybackSettings'] = async () => {},
): PlaybackSettingsApi {
  return { GetPlaybackSettings: get, SetPlaybackSettings: set }
}

describe('createPlaybackSettings', () => {
  it('缺失或非法值归一化为三个开关全关', async () => {
    const store = createPlaybackSettings(api(async () => ({}) as never))
    await store.load()
    expect(store.settings.value).toEqual({ AutoNext: false, AutoSwitchSource: false, PreloadNext: false })

    const illegal = createPlaybackSettings(api(async () => ({ AutoNext: 'yes' }) as never))
    await illegal.load()
    expect(illegal.settings.value).toEqual({ AutoNext: false, AutoSwitchSource: false, PreloadNext: false })
  })

  it('切换开关立即更新运行时值并提交完整聚合对象', async () => {
    const set = vi.fn(async () => {})
    const store = createPlaybackSettings(api(async () => ({ AutoNext: true }) as never, set))
    await store.load()

    await store.set('PreloadNext', true)
    expect(store.settings.value).toEqual({ AutoNext: true, AutoSwitchSource: false, PreloadNext: true })
    expect(set).toHaveBeenCalledWith({ AutoNext: true, AutoSwitchSource: false, PreloadNext: true })
  })

  it('持久化失败保留运行时值并上报错误', async () => {
    const failure = new Error('store closed')
    const onError = vi.fn()
    const store = createPlaybackSettings(
      api(async () => ({}) as never, async () => {
        throw failure
      }),
      onError,
    )
    await store.load()

    await store.set('AutoSwitchSource', true)
    expect(store.settings.value.AutoSwitchSource).toBe(true)
    expect(onError).toHaveBeenCalledWith(failure)
  })

  it('读取失败保持全关并上报错误', async () => {
    const failure = new Error('db unavailable')
    const onError = vi.fn()
    const store = createPlaybackSettings(
      api(async () => {
        throw failure
      }),
      onError,
    )
    await store.load()
    expect(store.settings.value).toEqual({ AutoNext: false, AutoSwitchSource: false, PreloadNext: false })
    expect(onError).toHaveBeenCalledWith(failure)
  })
})

describe('播放设置页接线', () => {
  it('设置页把三个开关直接展示成按钮，不弹窗、不拼接长文案', () => {
    expect(app).toContain('播放设置')
    expect(app).toContain('PLAYBACK_SETTING_ITEMS')
    expect(app).toContain('togglePlaybackSetting(item.key, !playbackSettings[item.key])')
    // 三个按钮直接切换并回显各自状态，不再有「播放设置：a · b · c」这类长文案。
    expect(app).toContain(':class="{ active: playbackSettings[item.key] }"')
    expect(app).toContain("{{ item.label }}：{{ playbackSettings[item.key] ? '开' : '关' }}")
    expect(app).not.toContain('playbackSettingsSummary')
    expect(app).not.toContain('showPlaybackSettings')
    expect(PLAYBACK_SETTING_ITEMS.map((item) => item.label)).toEqual(['自动切集', '自动换源', '预载下一集'])
  })

  it('启动时读取设置，切换时持久化', () => {
    expect(app).toContain('createPlaybackSettings')
    expect(app).toContain('playbackSettingsStore.load()')
    expect(app).toContain('playbackSettingsStore.set(')
  })

  it('点播播放器按自动换源开关决定是否自行降级', () => {
    expect(app).toContain(':suppress-fallback="playbackSettings.AutoSwitchSource"')
  })
})
