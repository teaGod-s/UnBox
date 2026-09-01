import { describe, expect, it } from 'vitest'
import { createVodSearchCache, isVodSearchCacheValid, vodBackTarget } from './vodNavigation'

describe('vod navigation', () => {
  it('returns to the page that opened the detail view', () => {
    expect(vodBackTarget('home')).toBe('home')
    expect(vodBackTarget('search')).toBe('search')
    expect(vodBackTarget('list')).toBe('list')
  })

  it('keeps search results valid for five minutes and expires them afterwards', () => {
    const cache = createVodSearchCache('重器', [{ ID: 'v1' }], 1_000)
    expect(cache.expiresAt).toBe(301_000)
    expect(isVodSearchCacheValid(cache, 300_999)).toBe(true)
    expect(isVodSearchCacheValid(cache, 301_000)).toBe(false)
  })
})
