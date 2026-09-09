import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const stylesheet = readFileSync('public/style.css', 'utf8')

describe('live layout contract', () => {
  it('keeps a usable minimum width for the live player column', () => {
    expect(stylesheet).toContain('minmax(280px, 1fr)')
    expect(stylesheet).toMatch(/\.player \.playback-view\s*\{[^}]*aspect-ratio:\s*16 \/ 9/s)
  })

  it('uses a breakpoint for a stacked player layout on narrow windows', () => {
    expect(stylesheet).toMatch(/@media \(max-width: 900px\)[\s\S]*grid-template-areas:[\s\S]*"player"[\s\S]*"groups"[\s\S]*"channels"/)
  })
})

describe('vod layout contract', () => {
  it('shares one rhythm between the card grid and the category rail', () => {
    expect(stylesheet).toMatch(/--content-col:\s*148px;/)
    expect(stylesheet).toMatch(/--content-gap:\s*0\.75rem;/)
    expect(stylesheet).toMatch(/--vod-rail:\s*128px;/)

    // 卡片列宽、卡片间距、卡片内边距统一走 token，不再散落字面量。
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill, minmax\(min\(100%, var\(--content-col\)\), 1fr\)\);/s)
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*gap:\s*var\(--content-gap\);/s)
    expect(stylesheet).toMatch(/\.content-card-grid\s*\{[^}]*padding:\s*var\(--content-gap\);/s)

    // 点播页必须重新声明 padding：.vod-main ul { padding: 0 } 的特异性更高，
    // 不重新声明会让卡片贴着面板边框，和侧栏的留白对不上。
    expect(stylesheet).toMatch(/\.vod-main ul\.content-card-grid\s*\{[^}]*padding:\s*var\(--content-gap\);/s)

    // 侧栏宽度与内边距同样走 token，两个面板的留白节奏一致。
    expect(stylesheet).toMatch(/\.vod-layout\s*\{[^}]*grid-template-columns:\s*var\(--vod-rail\) minmax\(0, 1fr\);[^}]*gap:\s*var\(--content-gap\);/s)
    expect(stylesheet).toMatch(/\.vod-cats\s*\{[^}]*padding:\s*var\(--content-gap\);/s)
  })

  it('keeps the category rail narrower than a single card column', () => {
    const px = (re: RegExp) => Number(stylesheet.match(re)![1])
    expect(px(/--vod-rail:\s*(\d+)px/)).toBeLessThan(px(/--content-col:\s*(\d+)px/))
  })

  it('sizes each poster row to the full poster height', () => {
    // 海报卡靠 aspect-ratio 定高，行高由自动网格行推出来。overflow: hidden 会把卡片变成
    // 滚动容器，浏览器就不拿 aspect-ratio 去撑那一行：1280×800 下行只有 168px、海报有
    // 255px，整列海报互相压在一起。overflow: clip 同样裁掉圆角外的溢出，但不是滚动
    // 容器，行高回到海报的真实高度，row-gap 才真的生效。
    expect(stylesheet).toMatch(/\.content-card-grid \.content-card\s*\{[^}]*overflow:\s*clip;/s)
  })

  it('leaves a gap between the progress mask and the title mask', () => {
    // 两个蒙层都绝对钉在卡底：标题在 bottom:0.35rem，整高约 1.67rem（0.98rem×1.25 行高
    // + 0.44rem 上下内边距），顶边离卡底约 2.02rem。进度蒙层的 bottom 必须比标题的 bottom
    // 高出至少 2rem（标题整高 + 一点空隙），否则两块黑底蒙层会像素级黏在一起。
    const rem = (sel: RegExp) => Number(stylesheet.match(sel)![1])
    const titleBottom = rem(/\.content-card-title\s*\{[^}]*bottom:\s*([\d.]+)rem/s)
    const progBottom = rem(/\.content-card-progress\s*\{[^}]*bottom:\s*([\d.]+)rem/s)
    expect(progBottom - titleBottom).toBeGreaterThanOrEqual(2)
  })
})

describe('library playlist layout contract', () => {
  it('keeps symmetric card padding and a narrow vertical scrollbar', () => {
    expect(stylesheet).toMatch(/\.library-body\s*\{[^}]*gap:\s*6px;/s)
    expect(stylesheet).toMatch(/\.library-side\s*\{[^}]*padding-left:\s*0;[^}]*padding-right:\s*12px;[^}]*scrollbar-width:\s*thin;/s)
    expect(stylesheet).toMatch(/\.library-side::?-webkit-scrollbar\s*\{[^}]*width:\s*6px;/s)
  })
})
