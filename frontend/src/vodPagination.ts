export interface VodPageMeta {
  Page: number
  PageCount: number
  HasMore: boolean
}

export function nextVodPage(page: number): number {
  return Math.max(1, page + 1)
}

export function hasNextVodPage(page: VodPageMeta, itemCount: number): boolean {
  if (page.PageCount > 0) return page.Page < page.PageCount
  return page.HasMore && itemCount > 0
}

export function appendVodItems<T extends { ID: string; Site?: string }>(existing: T[], incoming: T[]): T[] {
  const seen = new Set(existing.map(item => `${item.Site ?? ''}\u0000${item.ID}`))
  const appended = incoming.filter(item => {
    const key = `${item.Site ?? ''}\u0000${item.ID}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
  return [...existing, ...appended]
}
