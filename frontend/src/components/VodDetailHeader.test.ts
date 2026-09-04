import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodDetailHeader from './VodDetailHeader.vue'

describe('VodDetailHeader', () => {
  it('keeps the back action and current episode status in one row', () => {
    const wrapper = mount(VodDetailHeader, { props: { nowPlaying: '第1集' } })
    const row = wrapper.find('.vod-detail-head')
    expect(row.find('button.vod-back').exists()).toBe(true)
    expect(row.find('.now').text()).toBe('正在播放：第1集')
    expect(row.element.children).toHaveLength(2)
  })
})
