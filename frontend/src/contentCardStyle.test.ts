import { describe, expect, it } from 'vitest'
import { contentCardStyleLabel, contentCardStyleOptions, normalizeContentCardStyle, type ContentCardStyle } from './contentCardStyle'

describe('content card style', () => {
  it('accepts list and grid styles', () => {
    const styles: ContentCardStyle[] = ['list', 'grid']

    expect(styles.map(normalizeContentCardStyle)).toEqual(['list', 'grid'])
  })

  it('falls back to list for missing or invalid values', () => {
    expect(normalizeContentCardStyle('')).toBe('list')
    expect(normalizeContentCardStyle('cards')).toBe('list')
  })

  it('presents the persisted grid style as the card option', () => {
    expect(contentCardStyleOptions).toEqual([
      { value: 'list', label: '列表' },
      { value: 'grid', label: '卡片' },
    ])
    expect(contentCardStyleLabel('grid')).toBe('卡片')
  })
})
