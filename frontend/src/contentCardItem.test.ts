import { describe, expect, it } from 'vitest'
import { contentCardSubtitle, historyCardItem, type ContentCardItem } from './contentCardItem'

const base: ContentCardItem = {
  key: 'k',
  logo: '',
  title: '某剧',
  site: '站点A',
  detail: '分类B',
}

describe('contentCardSubtitle', () => {
  it('站点与补充文本用 · 连接', () => {
    expect(contentCardSubtitle(base)).toBe('站点A · 分类B')
  })

  // 点播列表里站点名可能为空（未匹配到源配置），此时不该留下孤零零的分隔符。
  it('缺少站点时只输出补充文本', () => {
    expect(contentCardSubtitle({ ...base, site: '' })).toBe('分类B')
  })

  it('缺少补充文本时只输出站点', () => {
    expect(contentCardSubtitle({ ...base, detail: '' })).toBe('站点A')
  })

  it('两者都空时输出空串', () => {
    expect(contentCardSubtitle({ ...base, site: '', detail: '' })).toBe('')
  })
})

describe('historyCardItem', () => {
  it('列表副标题为 站点 · 集数 · 看到 X', () => {
    const item = historyCardItem('k', '', '某剧', '站点A', '第3集', '12分34秒')
    expect(contentCardSubtitle(item)).toBe('站点A · 第3集 · 看到 12分34秒')
  })

  // 卡片进度徽标与列表副标题必须来自同一次格式化，否则两种展示会对不上。
  it('卡片进度徽标与列表副标题保持一致', () => {
    const item = historyCardItem('k', '', '某剧', '站点A', '第3集', '12分34秒')
    expect(item.progress).toEqual({ ep: '第3集', text: '看到 12分34秒' })
  })

  it('没有进度时只显示集数', () => {
    const item = historyCardItem('k', '', '某剧', '站点A', '第3集', '')
    expect(contentCardSubtitle(item)).toBe('站点A · 第3集')
    expect(item.progress).toEqual({ ep: '第3集', text: '' })
  })

  it('没有集数时只显示进度', () => {
    const item = historyCardItem('k', '', '某剧', '站点A', '', '12分34秒')
    expect(contentCardSubtitle(item)).toBe('站点A · 看到 12分34秒')
  })

  // 两者皆空时整块不渲染，避免出现一个空的徽标。
  it('既无集数也无进度时不输出进度徽标', () => {
    const item = historyCardItem('k', '', '某剧', '站点A', '', '')
    expect(contentCardSubtitle(item)).toBe('站点A')
    expect(item.progress).toBeUndefined()
  })
})
