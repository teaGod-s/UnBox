import { describe, expect, it } from 'vitest'
import { normalizeContentCardStyle, type ContentCardStyle } from './contentCardStyle'

describe('content card style', () => {
  it('accepts list and grid styles', () => {
    const styles: ContentCardStyle[] = ['list', 'grid']

    expect(styles.map(normalizeContentCardStyle)).toEqual(['list', 'grid'])
  })

  it('falls back to list for missing or invalid values', () => {
    expect(normalizeContentCardStyle('')).toBe('list')
    expect(normalizeContentCardStyle('cards')).toBe('list')
  })
})
