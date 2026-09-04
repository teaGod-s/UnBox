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
