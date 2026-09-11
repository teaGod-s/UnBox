import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

const app = readFileSync('src/App.vue', 'utf8')

describe('mpv standalone controls', () => {
  it('delegates basic playback controls to the mpv OSC', () => {
    // mpv 已在独立窗口展示 OSC；主窗口不能再展示暂停、继续、音量这套重复控件。
    expect(app).not.toMatch(/<div class="controls" v-if="[^"]*Backend === 'mpv'/)
    expect(app).not.toContain('@click="pause"')
    expect(app).not.toContain('@click="resume"')
    expect(app).not.toContain('@input="setVolume"')
  })

  it('keeps VOD business controls outside the player controls', () => {
    expect(app).toContain('class="ep-src-tabs"')
    expect(app).toContain('class="ep-list"')
  })
})
