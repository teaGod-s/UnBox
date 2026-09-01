import { describe, expect, it } from 'vitest'
import { clampEpisodePage, episodePageIndex, episodePageRanges, paginateEpisodes } from './episodes'

describe('episode pagination', () => {
  it('splits episodes into groups of thirty', () => {
    const pages = paginateEpisodes(Array.from({ length: 61 }, (_, i) => i + 1))
    expect(pages).toHaveLength(3)
    expect(pages[0]).toHaveLength(30)
    expect(pages[2]).toEqual([61])
  })

  it('builds TvBox-style ranges and clamps invalid pages', () => {
    expect(episodePageRanges(61)).toEqual([
      { start: 1, end: 30 },
      { start: 31, end: 60 },
      { start: 61, end: 61 },
    ])
    expect(clampEpisodePage(-1, 61)).toBe(0)
    expect(clampEpisodePage(99, 61)).toBe(2)
    expect(clampEpisodePage(0, 0)).toBe(0)
  })

  it('finds the page containing a resumed episode', () => {
    const episodes = Array.from({ length: 61 }, (_, i) => ({ ID: `ep-${i + 1}` }))
    expect(episodePageIndex(episodes, 'ep-31')).toBe(1)
    expect(episodePageIndex(episodes, 'missing')).toBe(0)
  })
})
