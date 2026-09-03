export type VodView = 'list' | 'search' | 'detail'
export type VodDetailOrigin = 'home' | 'search' | 'list' | 'favorites'
export const VOD_SEARCH_CACHE_TTL = 5 * 60 * 1000

export function nextVodSearchRequest(previous: number): number {
  return previous + 1
}

export function nextVodCategoryRequest(previous: number): number {
  return previous + 1
}

export function isCurrentVodCategoryRequest(request: number, current: number): boolean {
  return request === current
}

export interface VodSearchCache<T> {
  query: string
  items: T[]
  expiresAt: number
}

export function vodBackTarget(origin: VodDetailOrigin): Exclude<VodView, 'detail'> | 'home' | 'favorites' {
  return origin
}

export function createVodSearchCache<T>(query: string, items: T[], now = Date.now()): VodSearchCache<T> {
  return { query, items: [...items], expiresAt: now + VOD_SEARCH_CACHE_TTL }
}

export function isVodSearchCacheValid<T>(cache: VodSearchCache<T> | null, now = Date.now()): boolean {
  return !!cache && cache.expiresAt > now
}

export function vodSearchQueryForReturn<T>(cache: VodSearchCache<T> | null, currentQuery: string): string {
  return cache?.query || currentQuery
}

export function upsertVodSearchHistory(history: string[], query: string, limit = 30): string[] {
  const value = query.trim()
  if (!value) return history.slice(0, limit)
  return [value, ...history.filter(item => item !== value)].slice(0, limit)
}

export function removeVodSearchHistory(history: string[], query: string): string[] {
  return history.filter(item => item !== query)
}

export function removeVodHistory<T extends { Site: string; VodID: string }>(items: T[], site: string, vodID: string): T[] {
  return items.filter(item => item.Site !== site || item.VodID !== vodID)
}

export function removeVodFavorite<T extends { Site: string; VodID: string }>(items: T[], site: string, vodID: string): T[] {
  return items.filter(item => item.Site !== site || item.VodID !== vodID)
}

export function shouldShowVodNoResults(inputQuery: string, completedQuery: string, searching: boolean, resultCount: number): boolean {
  const query = inputQuery.trim()
  return query !== '' && query === completedQuery && !searching && resultCount === 0
}
