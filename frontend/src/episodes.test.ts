import { describe, expect, it } from 'vitest'
import { clampEpisodePage, episodePageIndex, episodePageRanges, paginateEpisodes } from './episodes'

describe('episode pagination', () => {
  it('splits episodes into groups of thirty-six', () => {
    const pages = paginateEpisodes(Array.from({ length: 73 }, (_, i) => i + 1))
    expect(pages).toHaveLength(3)
    expect(pages[0]).toHaveLength(36)
    expect(pages[1]).toHaveLength(36)
    expect(pages[2]).toEqual([73])
  })

  it('builds TvBox-style ranges and clamps invalid pages', () => {
    expect(episodePageRanges(73)).toEqual([
      { start: 1, end: 36 },
      { start: 37, end: 72 },
      { start: 73, end: 73 },
    ])
    expect(clampEpisodePage(-1, 73)).toBe(0)
    expect(clampEpisodePage(99, 73)).toBe(2)
    expect(clampEpisodePage(0, 0)).toBe(0)
  })

  it('finds the page containing a resumed episode', () => {
    const episodes = Array.from({ length: 73 }, (_, i) => ({ ID: `ep-${i + 1}` }))
    expect(episodePageIndex(episodes, 'ep-37')).toBe(1)
    expect(episodePageIndex(episodes, 'missing')).toBe(0)
  })
})
