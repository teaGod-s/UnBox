export type ContentCardStyle = 'list' | 'grid'

export function normalizeContentCardStyle(value: string | null | undefined): ContentCardStyle {
  return value === 'grid' ? 'grid' : 'list'
}
