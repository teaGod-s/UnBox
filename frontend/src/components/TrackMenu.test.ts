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

describe('TrackMenu', () => {
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
    expect(w.findAll('.track-sec')).toHaveLength(1)
    expect(w.findAll('h4').map((heading) => heading.text())).toEqual(['清晰度'])
  })

  it('「加载字幕」按钮触发隐藏 file input 的 click', async () => {
    const w = mount(TrackMenu, { props: stubProps() })
    const input = w.find('input[type="file"]')
    const click = vi.spyOn(input.element as HTMLInputElement, 'click').mockImplementation(() => {})
    await w.find('.load-subtitle').trigger('click')
    expect(click).toHaveBeenCalled()
  })
})
