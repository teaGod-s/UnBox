import { describe, expect, it } from 'vitest'
import { createVodSearchCache, isCurrentVodCategoryRequest, isVodSearchCacheValid, nextVodCategoryRequest, nextVodSearchRequest, pickResumeSeek, pickResumeSource, removeVodFavorite, removeVodHistory, removeVodSearchHistory, resolveVodSelection, shouldShowVodNoResults, upsertVodSearchHistory, vodBackTarget, vodResumeView, vodSearchQueryForReturn } from './vodNavigation'

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

  it('accepts only the latest category request', () => {
    const request = nextVodCategoryRequest(3)
    expect(request).toBe(4)
    expect(isCurrentVodCategoryRequest(4, request)).toBe(true)
    expect(isCurrentVodCategoryRequest(3, request)).toBe(false)
  })

  it('does not show no-results before a search is submitted and completed', () => {
    expect(shouldShowVodNoResults('斗罗', '', false, 0)).toBe(false)
    expect(shouldShowVodNoResults('斗罗', '斗罗', true, 0)).toBe(false)
    expect(shouldShowVodNoResults('斗罗', '斗罗', false, 0)).toBe(true)
    expect(shouldShowVodNoResults('斗罗2', '斗罗', false, 0)).toBe(false)
  })

  it('prefers the line recorded in history and falls back to the first available line', () => {
    const available = ['线路一', '线路二', '线路三']
    expect(pickResumeSource('线路二', available)).toBe('线路二')
    expect(pickResumeSource('已下线', available)).toBe('线路一')
    expect(pickResumeSource('', available)).toBe('线路一')
    expect(pickResumeSource('线路二', [])).toBe('')
    expect(pickResumeSource('', null)).toBe('')
  })

  it('seeks only into the episode the deferred resume was recorded for', () => {
    const target = { EpID: 'ep-5', Progress: 1234 }
    expect(pickResumeSeek(target, 'ep-5')).toBe(1234)
    expect(pickResumeSeek(target, 'ep-1')).toBe(0)
    expect(pickResumeSeek(null, 'ep-5')).toBe(0)
    expect(pickResumeSeek({ EpID: 'ep-5', Progress: 0 }, 'ep-5')).toBe(0)
  })

  it('resolves the line, highlighted episode and page for the recorded resume target', () => {
    const detail = {
      Sources: ['线路一', '线路二'],
      Episodes: [
        { ID: 'ep-1', Source: '线路一' },
        { ID: 'ep-2', Source: '线路一' },
        { ID: 'ep-7', Source: '线路二' },
      ],
    }
    expect(vodResumeView(detail, { EpID: 'ep-7', Source: '线路二' })).toEqual({
      source: '线路二',
      episodeID: 'ep-7',
      page: 0,
    })
  })

  it('pages over only the episodes of the resumed line', () => {
    const episodes = Array.from({ length: 50 }, (_, i) => ({ ID: `ep-${i + 1}`, Source: '线路一' }))
    expect(vodResumeView({ Sources: ['线路一'], Episodes: episodes }, { EpID: 'ep-40', Source: '线路一' })).toEqual({
      source: '线路一',
      episodeID: 'ep-40',
      page: 1,
    })
  })

  it('falls back to the first line when the recorded line is gone', () => {
    const detail = { Sources: ['线路一'], Episodes: [{ ID: 'ep-1', Source: '线路一' }] }
    expect(vodResumeView(detail, { EpID: 'ep-9', Source: '线路三' })).toEqual({
      source: '线路一',
      episodeID: 'ep-9',
      page: 0,
    })
  })

  it('shows no highlighted episode when there is no history record', () => {
    const detail = { Sources: ['线路一', '线路二'], Episodes: [{ ID: 'ep-1', Source: '线路一' }] }
    expect(vodResumeView(detail, null)).toEqual({ source: '线路一', episodeID: '', page: 0 })
    expect(vodResumeView(null, null)).toEqual({ source: '', episodeID: '', page: 0 })
  })

  it('restores the remembered site and its line for the site selector', () => {
    const sites = [
      { ID: 'site-a', Line: '线路一' },
      { ID: 'site-b', Line: '线路二' },
    ]
    expect(resolveVodSelection(sites, 'site-b')).toEqual({ site: 'site-b', line: '线路二' })
    expect(resolveVodSelection(sites, 'missing')).toEqual({ site: 'site-a', line: '线路一' })
  })
})
