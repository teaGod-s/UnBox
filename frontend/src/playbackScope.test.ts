import { describe, expect, it } from 'vitest'
import { playbackPlanForMode, resolvePlaybackFallback, shouldPauseStalePlayback, shouldRecordVodProgress } from './playbackScope'

describe('playback scope', () => {
  it('only exposes the plan belonging to the active page', () => {
    const plans = { live: { ID: 'live-1' }, vod: { ID: 'vod-1' } }
    expect(playbackPlanForMode('live', plans)).toEqual(plans.live)
    expect(playbackPlanForMode('vod', plans)).toEqual(plans.vod)
    expect(playbackPlanForMode('home', plans)).toBeNull()
    expect(playbackPlanForMode('live', plans, 'vod')).toBeNull()
  })

  it('keeps the captured fallback scope when the request resolves after a page switch', async () => {
    let resolve!: (plan: { ID: string }) => void
    const request = new Promise<{ ID: string }>(done => { resolve = done })
    const applied: Array<{ scope: string; ID: string }> = []
    let current = true
    const pending = resolvePlaybackFallback('vod', () => request, () => current, (scope, plan) => applied.push({ scope, ID: plan.ID }))

    current = false
    resolve({ ID: 'vod-fallback' })
    await expect(pending).resolves.toBe(false)

    expect(applied).toEqual([])
  })

  it('only pauses a stale request when no newer session owns the player', () => {
    expect(shouldPauseStalePlayback(null)).toBe(true)
    expect(shouldPauseStalePlayback({ scope: 'live', token: 2 })).toBe(false)
  })

  it('rejects late vod progress outside the vod detail page', () => {
    expect(shouldRecordVodProgress('vod', 'detail')).toBe(true)
    expect(shouldRecordVodProgress('live', 'detail')).toBe(false)
    expect(shouldRecordVodProgress('vod', 'list')).toBe(false)
  })
})
