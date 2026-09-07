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

describe('library playlist layout contract', () => {
  it('keeps symmetric card padding and a narrow vertical scrollbar', () => {
    expect(stylesheet).toMatch(/\.library-body\s*\{[^}]*gap:\s*6px;/s)
    expect(stylesheet).toMatch(/\.library-side\s*\{[^}]*padding-left:\s*0;[^}]*padding-right:\s*12px;[^}]*scrollbar-width:\s*thin;/s)
    expect(stylesheet).toMatch(/\.library-side::?-webkit-scrollbar\s*\{[^}]*width:\s*6px;/s)
  })
})
