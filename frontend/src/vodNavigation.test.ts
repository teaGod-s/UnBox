import { describe, expect, it } from 'vitest'
import { createVodSearchCache, isVodSearchCacheValid, nextVodSearchRequest, removeVodFavorite, removeVodHistory, removeVodSearchHistory, upsertVodSearchHistory, vodBackTarget, vodSearchQueryForReturn } from './vodNavigation'

describe('vod navigation', () => {
  it('returns to the page that opened the detail view', () => {
    expect(vodBackTarget('home')).toBe('home')
    expect(vodBackTarget('search')).toBe('search')
    expect(vodBackTarget('list')).toBe('list')
    expect(vodBackTarget('favorites')).toBe('favorites')
  })

  it('keeps search results valid for five minutes and expires them afterwards', () => {
    const cache = createVodSearchCache('重器', [{ ID: 'v1' }], 1_000)
    expect(cache.expiresAt).toBe(301_000)
    expect(isVodSearchCacheValid(cache, 300_999)).toBe(true)
    expect(isVodSearchCacheValid(cache, 301_000)).toBe(false)
  })

  it('uses the cached query when an expired search result is reloaded', () => {
    const cache = createVodSearchCache('原搜索', [], 1_000)
    expect(vodSearchQueryForReturn(cache, '详情页临时输入')).toBe('原搜索')
    expect(vodSearchQueryForReturn(null, '当前输入')).toBe('当前输入')
  })

  it('creates monotonically increasing request tokens', () => {
    const first = nextVodSearchRequest(0)
    const second = nextVodSearchRequest(first)
    expect(second).toBe(first + 1)
  })

  it('keeps the latest search first and removes one history item', () => {
    expect(upsertVodSearchHistory(['旧', '重器'], ' 重器 ')).toEqual(['重器', '旧'])
    expect(removeVodSearchHistory(['重器', '旧'], '重器')).toEqual(['旧'])
  })

  it('removes only the selected home history item', () => {
    const items = [{ Site: 'a', VodID: '1' }, { Site: 'b', VodID: '1' }, { Site: 'a', VodID: '2' }]
    expect(removeVodHistory(items, 'a', '1')).toEqual([{ Site: 'b', VodID: '1' }, { Site: 'a', VodID: '2' }])
  })

  it('removes only one site-scoped vod favorite', () => {
    const items = [{ Site: 'a', VodID: '1' }, { Site: 'b', VodID: '1' }, { Site: 'a', VodID: '2' }]
    expect(removeVodFavorite(items, 'a', '1')).toEqual([{ Site: 'b', VodID: '1' }, { Site: 'a', VodID: '2' }])
  })
})
