export const EPISODES_PER_PAGE = 36

export interface EpisodePageRange {
  start: number
  end: number
}

export function paginateEpisodes<T>(episodes: readonly T[], pageSize = EPISODES_PER_PAGE): T[][] {
  if (pageSize <= 0) return [Array.from(episodes)]
  const pages: T[][] = []
  for (let start = 0; start < episodes.length; start += pageSize) {
    pages.push(Array.from(episodes.slice(start, start + pageSize)))
  }
  return pages
}

export function episodePageRanges(total: number, pageSize = EPISODES_PER_PAGE): EpisodePageRange[] {
  if (total <= 0 || pageSize <= 0) return []
  const ranges: EpisodePageRange[] = []
  for (let start = 0; start < total; start += pageSize) {
    ranges.push({ start: start + 1, end: Math.min(start + pageSize, total) })
  }
  return ranges
}

export function clampEpisodePage(page: number, total: number, pageSize = EPISODES_PER_PAGE): number {
  const pageCount = episodePageRanges(total, pageSize).length
  if (pageCount === 0) return 0
  return Math.min(Math.max(page, 0), pageCount - 1)
}

export function episodePageIndex<T extends { ID: string }>(episodes: readonly T[], episodeID: string, pageSize = EPISODES_PER_PAGE): number {
  const episodeIndex = episodes.findIndex(ep => ep.ID === episodeID)
  if (episodeIndex < 0 || pageSize <= 0) return 0
  return Math.floor(episodeIndex / pageSize)
}
