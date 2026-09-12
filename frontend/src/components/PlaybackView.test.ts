import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PlaybackView, { type PlaybackPlan } from './PlaybackView.vue'
import Hls from 'hls.js'
import mpegts from 'mpegts.js'

// 捕获每次 attach() 创建的实例，测试直接调用其注册的错误回调，
// 不依赖 <script setup> 的内部绑定是否泄露到 vm。
const instances = vi.hoisted(() => ({
  hls: [] as any[],
  flv: [] as any[],
}))

vi.mock('hls.js', () => {
  class MockHls {
    static Events = {
      ERROR: 'hlsError',
      MANIFEST_PARSED: 'hlsManifestParsed',
      AUDIO_TRACKS_UPDATED: 'hlsAudioTracksUpdated',
      SUBTITLE_TRACKS_UPDATED: 'hlsSubtitleTracksUpdated',
      LEVEL_SWITCHED: 'hlsLevelSwitched',
      AUDIO_TRACK_SWITCHED: 'hlsAudioTrackSwitched',
      SUBTITLE_TRACK_SWITCH: 'hlsSubtitleTrackSwitch',
    }
    static ErrorTypes = {
      NETWORK_ERROR: 'networkError',
      MEDIA_ERROR: 'mediaError',
      OTHER_ERROR: 'otherError',
    }
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
    loadSource = vi.fn()
    attachMedia = vi.fn()
    startLoad = vi.fn()
    recoverMediaError = vi.fn()
    destroy = vi.fn()
    constructor() {
      instances.hls.push(this)
    }
  }
  return { default: MockHls, Events: MockHls.Events }
})

vi.mock('mpegts.js', () => {
  class MockFlv {
    static Events = { ERROR: 'flvError' }
    static ErrorTypes = {
      NETWORK_ERROR: 'NetworkError',
      MEDIA_ERROR: 'MediaError',
      OTHER_ERROR: 'OtherError',
    }
    handlers: Record<string, (...args: unknown[]) => void> = {}
    on = vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      this.handlers[event] = cb
    })
    attachMediaElement = vi.fn()
    load = vi.fn()
    unload = vi.fn()
    destroy = vi.fn()
    constructor() {
      instances.flv.push(this)
    }
  }
  return {
    default: {
      getFeatureList: () => ({ mseLivePlayback: true }),
      createPlayer: vi.fn(() => new MockFlv()),
      Events: MockFlv.Events,
      ErrorTypes: MockFlv.ErrorTypes,
    },
  }
})

async function mountView(plan?: Partial<PlaybackPlan>, extraProps: Record<string, unknown> = {}) {
  const wrapper = mount(PlaybackView, {
    props: {
      plan: { ID: 'p1', Backend: 'web', URL: '/x.m3u8', Kind: 'hls', CanFallback: true, ...plan },
      ...extraProps,
    },
  })
  await nextTick()
  await Promise.resolve()
  return wrapper
}

function setVideoTime(wrapper: ReturnType<typeof mount>, seconds: number) {
  ;(wrapper.find('video').element as HTMLVideoElement).currentTime = seconds
}

function hlsError(type: string, fatal = true, details = 'testDetails') {
  return { fatal, type, details }
}

beforeEach(() => {
  instances.hls.length = 0
  instances.flv.length = 0
})

describe('PlaybackView', () => {
  it('attaches HLS to the proxied URL', async () => {
    await mountView()
    expect(instances.hls[0]).toBeTruthy()
    expect(instances.hls[0].loadSource).toHaveBeenCalledWith('/x.m3u8')
  })

  it('ignores non-fatal hls errors', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR, false))
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(hls.recoverMediaError).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('retries fatal hls network errors in place within the budget', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    for (let i = 0; i < 3; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR))
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back after the hls network retry budget is exhausted, carrying position', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    setVideoTime(wrapper, 321)
    for (let i = 0; i < 4; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR))
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 321]])
  })

  it('recovers from the first fatal hls media error in place', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.MEDIA_ERROR))
    expect(hls.recoverMediaError).toHaveBeenCalledTimes(1)
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back after the hls media recovery budget is exhausted', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    setVideoTime(wrapper, 88)
    // 挂载失败在 1.7.1 里的 type 是 MEDIA_ERROR，细节记在 details。
    const attachFail = () =>
      hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.MEDIA_ERROR, true, Hls.ErrorDetails.ATTACH_MEDIA_ERROR))
    for (let i = 0; i < 3; i++) attachFail()
    expect(hls.recoverMediaError).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 88]])
  })

  it('falls back immediately on other fatal hls errors', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(hls.recoverMediaError).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toHaveLength(1)
  })

  it('emits fallback at most once', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    for (let i = 0; i < 5; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(wrapper.emitted('fallback')).toHaveLength(1)
  })

  it('does not emit fallback when the plan cannot fall back', async () => {
    const wrapper = await mountView({ CanFallback: false })
    const hls = instances.hls[0]
    for (let i = 0; i < 5; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back immediately on mpegts media errors, carrying position', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 55)
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(flv.unload).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toEqual([['p1', 55]])
  })

  it('retries mpegts network errors in place, then falls back once the budget is spent', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 77)
    const retry = () =>
      flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.NETWORK_ERROR, 'NetworkException', {})

    for (let i = 0; i < 3; i++) retry()
    expect(flv.unload).toHaveBeenCalledTimes(3)
    expect(flv.load).toHaveBeenCalledTimes(4) // 初始加载 + 3 次重连
    expect(wrapper.emitted('fallback')).toBeUndefined()

    retry()
    expect(flv.unload).toHaveBeenCalledTimes(3) // 预算耗尽，不再重连管线
    expect(flv.load).toHaveBeenCalledTimes(4)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 77]])
  })

  it('renders mpv mode without creating a video source', () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: { ID: 'p2', Backend: 'mpv', URL: '', Kind: 'hevc', CanFallback: false } },
    })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('mpv')
  })

  it('shows the default empty text with no plan', () => {
    const wrapper = mount(PlaybackView, { props: { plan: null } })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('选择频道或剧集开始播放')
  })

  it('shows the empty text the caller provides', () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: null, emptyText: '点右侧视频开始播放' },
    })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('点右侧视频开始播放')
    expect(wrapper.text()).not.toContain('选择频道或剧集开始播放')
  })

  it('hls 路径：manifest 后显示设置按钮，点开菜单含清晰度项', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[instances.hls.length - 1]
    hls.levels = [{ height: 720, bitrate: 2_800_000 }]
    hls.handlers[Hls.Events.MANIFEST_PARSED]({})
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(true)
    await wrapper.find('.track-toggle').trigger('click')
    expect(wrapper.find('.track-menu').exists()).toBe(true)
    expect(wrapper.text()).toContain('720p')
  })

  it('flv 路径不渲染设置按钮', async () => {
    const wrapper = await mountView({ Kind: 'flv' })
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(false)
  })

  it('mpv 后端不渲染设置按钮', async () => {
    const wrapper = await mountView({ Backend: 'mpv' })
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(false)
  })

  it('点击菜单外关闭轨道菜单', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[instances.hls.length - 1]
    hls.handlers[Hls.Events.MANIFEST_PARSED]({})
    await nextTick()
    await wrapper.find('.track-toggle').trigger('click')
    expect(wrapper.find('.track-menu').exists()).toBe(true)
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(wrapper.find('.track-menu').exists()).toBe(false)
  })

  it('快速切换播放计划时只挂载最新 HLS', async () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: { ID: 'old', Backend: 'web', URL: '/old.m3u8', Kind: 'hls', CanFallback: true } },
    })
    await wrapper.setProps({ plan: { ID: 'new', Backend: 'web', URL: '/new.m3u8', Kind: 'hls', CanFallback: true } })
    await nextTick()
    await Promise.resolve()
    expect(instances.hls).toHaveLength(2)
    expect(instances.hls[0].destroy).toHaveBeenCalled()
    expect(instances.hls[1].loadSource).toHaveBeenCalledWith('/new.m3u8')
  })

  it('把原生 video 事件统一上报为标准播放信号', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video')
    await video.trigger('playing')
    await video.trigger('waiting')
    await video.trigger('stalled')
    await video.trigger('canplay')
    await video.trigger('ended')
    expect(wrapper.emitted('playback')).toEqual([
      ['playing'],
      ['buffering'],
      ['buffering'],
      ['ready'],
      ['ended'],
    ])
  })

  it('suppressFallback 为真时只上报错误，不发旧 fallback', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' }, { suppressFallback: true })
    const flv = instances.flv[0]
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(wrapper.emitted('fallback')).toBeUndefined()
    expect(wrapper.emitted('playback')).toEqual([['error', 'MediaMSEError']])
  })

  it('suppressFallback 关闭时保留原有 fallback 行为', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 42)
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(wrapper.emitted('fallback')).toEqual([['p1', 42]])
  })

  it('每次播放计划只上报一次错误信号', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' }, { suppressFallback: true })
    const flv = instances.flv[0]
    const fire = () => flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    fire()
    fire()
    fire()
    expect(wrapper.emitted('playback')).toEqual([['error', 'MediaMSEError']])
  })

  it('原生 video error 事件上报错误信号', async () => {
    const wrapper = await mountView({ Kind: 'mp4', URL: '/x.mp4' }, { suppressFallback: true })
    await wrapper.find('video').trigger('error')
    expect(wrapper.emitted('playback')).toEqual([['error', undefined]])
  })

})
