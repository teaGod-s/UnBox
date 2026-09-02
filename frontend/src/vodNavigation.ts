export type VodView = 'list' | 'search' | 'detail'
export type VodDetailOrigin = 'home' | 'search' | 'list'
export const VOD_SEARCH_CACHE_TTL = 5 * 60 * 1000

export function nextVodSearchRequest(previous: number): number {
  return previous + 1
}

export interface VodSearchCache<T> {
  query: string
  items: T[]
  expiresAt: number
}

export function vodBackTarget(origin: VodDetailOrigin): Exclude<VodView, 'detail'> | 'home' {
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
