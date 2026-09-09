import { describe, expect, it } from 'vitest'
import { appendVodItems, hasNextVodPage, nextVodPage } from './vodPagination'

describe('vod pagination', () => {
  it('starts at page one and advances one page at a time', () => {
    expect(nextVodPage(0)).toBe(1)
    expect(nextVodPage(1)).toBe(2)
  })

  it('uses provider metadata to decide whether to show continue loading', () => {
    expect(hasNextVodPage({ Page: 1, PageCount: 3, HasMore: false }, 20)).toBe(true)
    expect(hasNextVodPage({ Page: 3, PageCount: 3, HasMore: false }, 20)).toBe(false)
    expect(hasNextVodPage({ Page: 2, PageCount: 0, HasMore: true }, 20)).toBe(true)
    expect(hasNextVodPage({ Page: 3, PageCount: 0, HasMore: false }, 0)).toBe(false)
  })

  it('appends new pages without duplicating an item', () => {
    const existing = [{ ID: '1', Site: 's' }, { ID: '2', Site: 's' }]
    const incoming = [{ ID: '2', Site: 's' }, { ID: '3', Site: 's' }]
    expect(appendVodItems(existing, incoming)).toEqual([
      { ID: '1', Site: 's' },
      { ID: '2', Site: 's' },
      { ID: '3', Site: 's' },
    ])
  })
})
