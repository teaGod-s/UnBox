import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlaybackView from './PlaybackView.vue'

const hlsLoad = vi.fn()
const hlsDestroy = vi.fn()
vi.mock('hls.js', () => {
  class MockHls {
    static Events = { ERROR: 'hlsError' }
    static isSupported = () => true
    on = vi.fn((_event: string, cb: (event: string, data: { fatal: boolean }) => void) => { ;(this as any).error = cb })
    loadSource = hlsLoad
    attachMedia = vi.fn()
    destroy = hlsDestroy
  }
  return { default: MockHls }
})
vi.mock('mpegts.js', () => ({ default: { getFeatureList: () => ({ mseLivePlayback: true }), createPlayer: vi.fn() } }))

describe('PlaybackView', () => {
  it('attaches HLS and emits one fallback on fatal error', async () => {
    const wrapper = mount(PlaybackView, { props: { plan: { ID: 'p1', Backend: 'web', URL: '/x.m3u8', Kind: 'hls', CanFallback: true } } })
    await Promise.resolve()
    expect(hlsLoad).toHaveBeenCalledWith('/x.m3u8')
    const instance = (wrapper.vm as any)
    instance.requestFallback?.()
    expect(wrapper.emitted('fallback')).toEqual([['p1']])
  })
  it('renders mpv mode without creating a video source', () => {
    const wrapper = mount(PlaybackView, { props: { plan: { ID: 'p2', Backend: 'mpv', URL: '', Kind: 'hevc', CanFallback: false } } })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('mpv')
  })
})
