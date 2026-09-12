import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PreloadView from './PreloadView.vue'
import type { PlaybackPlan } from './PlaybackView.vue'
import { ShellService } from '../../bindings/github.com/unbox/unbox/internal/shell'

vi.mock('../../bindings/github.com/unbox/unbox/internal/shell', () => ({
  ShellService: { ReleasePreload: vi.fn(async () => {}) },
}))

const releaseMock = ShellService.ReleasePreload as unknown as ReturnType<typeof vi.fn>

function webPlan(id: string, url = `/preload/${id}.m3u8`) {
  return { ID: id, Backend: 'web' as const, URL: url, Kind: 'hls', CanFallback: false }
}

async function mountPreload(plan: PlaybackPlan | null) {
  const wrapper = mount(PreloadView, { props: { plan } })
  await nextTick()
  await Promise.resolve()
  await nextTick()
  return wrapper
}

beforeEach(() => {
  releaseMock.mockClear()
})

describe('PreloadView', () => {
  it('用隐藏的静音 video 预载 Web 计划', async () => {
    const wrapper = await mountPreload(webPlan('p1'))
    const video = wrapper.find('video')
    expect(video.exists()).toBe(true)
    expect(video.attributes('src')).toBe('/preload/p1.m3u8')
    expect(video.attributes('preload')).toBe('auto')
    // muted 在 Vue 里按 DOM 属性写入，不是 attribute。
    expect((video.element as HTMLVideoElement).muted).toBe(true)
    // 预载元素不得带播放控件，也不能接管当前播放。
    expect(video.attributes('controls')).toBeUndefined()
  })

  it('把 canplay / error 转换成 ready / error 事件', async () => {
    const wrapper = await mountPreload(webPlan('p2'))
    await wrapper.find('video').trigger('canplay')
    await wrapper.find('video').trigger('error')
    expect(wrapper.emitted('ready')).toEqual([['p2']])
    expect(wrapper.emitted('error')).toHaveLength(1)
  })

  it('切换计划时清空旧 source 并释放旧预载', async () => {
    const wrapper = await mountPreload(webPlan('old'))
    await wrapper.setProps({ plan: webPlan('new', '/preload/new.m3u8') })
    await nextTick()
    await Promise.resolve()
    await nextTick()
    expect(releaseMock).toHaveBeenCalledWith('old')
    expect(wrapper.find('video').attributes('src')).toBe('/preload/new.m3u8')
  })

  it('卸载时清空 source 并释放预载', async () => {
    const wrapper = await mountPreload(webPlan('bye'))
    const element = wrapper.find('video').element as HTMLVideoElement
    wrapper.unmount()
    expect(releaseMock).toHaveBeenCalledWith('bye')
    expect(element.hasAttribute('src')).toBe(false)
  })

  it('mpv 计划不渲染预载元素', async () => {
    const wrapper = await mountPreload({
      ID: 'm1',
      Backend: 'mpv',
      URL: '',
      Kind: 'hevc',
      CanFallback: false,
    })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(releaseMock).not.toHaveBeenCalled()
  })
})
