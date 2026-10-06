// contentListDom.ts — 内容列表共用的 DOM 交互。
// App.vue 与 ContentCardList.vue 都要用，故从 App.vue 抽出。

/** imgError 隐藏加载失败的图片，避免浏览器碎图占位。 */
export function imgError(e: Event) {
  ;(e.target as HTMLImageElement).style.display = 'none'
}

// scrollxEnter/Leave 让过长文本在悬停时自动左滑揭示隐藏部分（无手动滚动条）。
// 纯 CSS 做不到：translateX 百分比相对自身宽度，无法得知溢出量，故用 JS 量取。
export function scrollxEnter(e: MouseEvent) {
  const box = e.currentTarget as HTMLElement
  const inner = box.firstElementChild as HTMLElement | null
  if (!inner) return
  const overflow = inner.scrollWidth - box.clientWidth
  if (overflow <= 0) return
  inner.style.transition = `transform ${Math.min(3000, 600 + overflow * 4)}ms ease-out`
  inner.style.transform = `translateX(${-overflow}px)`
}

export function scrollxLeave(e: MouseEvent) {
  const box = e.currentTarget as HTMLElement
  const inner = box.firstElementChild as HTMLElement | null
  if (!inner) return
  inner.style.transition = 'transform 300ms ease-in-out'
  inner.style.transform = ''
}
