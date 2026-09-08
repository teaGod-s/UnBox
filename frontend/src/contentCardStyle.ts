export type ContentCardStyle = 'list' | 'grid'

export const contentCardStyleOptions = [
  { value: 'list', label: '列表' },
  { value: 'grid', label: '卡片' },
] as const

export function contentCardStyleLabel(value: ContentCardStyle): string {
  return value === 'grid' ? '卡片' : '列表'
}

export function normalizeContentCardStyle(value: string | null | undefined): ContentCardStyle {
  return value === 'grid' ? 'grid' : 'list'
}
