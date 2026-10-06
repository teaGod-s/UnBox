// contentCardItem.ts — 「内容展示样式」（列表 / 卡片）共用的条目模型。
//
// 首页观看记录、点播收藏、点播列表、搜索结果四处数据结构不同，但渲染完全一致。
// 各自归一成 ContentCardItem 后交给 ContentCardList 一份模板渲染，避免四处复制
// 卡片标记与列表行标记。

/** 卡片模式的进度徽标：集数与进度分开渲染，保留原有的分栏间距。 */
export interface ContentCardProgress {
  /** 集数，如「第 3 集」。 */
  ep: string
  /** 播放进度，如「看到 12分34秒」。 */
  text: string
}

export interface ContentCardItem {
  /** 列表去重键，同时用于标记「当前展开删除遮罩」的条目。 */
  key: string
  logo: string
  title: string
  /** 站点显示名：卡片模式下作为站点徽标，列表模式下作为副标题首段。 */
  site: string
  /** 副标题补充（分类 / 集数 / 进度），与 site 用「 · 」连接。 */
  detail: string
  /** 卡片模式的进度徽标；为空时整块不渲染（收藏与点播列表都没有进度）。 */
  progress?: ContentCardProgress
}

/** contentCardSubtitle 拼出列表模式第二行的文本。 */
export function contentCardSubtitle(item: ContentCardItem): string {
  return [item.site, item.detail].filter(Boolean).join(' · ')
}

/**
 * historyCardItem 归一「首页观看记录」。
 *
 * 单独拎出来是因为它的 site / detail / progress 三者必须保持一致：列表第二行是
 * 「站点 · 集数 · 看到 X」，卡片进度徽标是「集数 + 看到 X」，两处共用同一次格式化
 * 结果，避免只改一处导致两种展示对不上。
 */
export function historyCardItem(
  key: string,
  logo: string,
  title: string,
  site: string,
  episode: string,
  progressText: string,
): ContentCardItem {
  const seen = progressText ? `看到 ${progressText}` : ''
  return {
    key,
    logo,
    title,
    site,
    detail: [episode, seen].filter(Boolean).join(' · '),
    progress: episode || seen ? { ep: episode, text: seen } : undefined,
  }
}
