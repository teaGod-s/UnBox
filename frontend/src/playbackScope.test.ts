import { describe, expect, it } from 'vitest'
import { playbackPlanForMode } from './playbackScope'

describe('playback scope', () => {
  it('only exposes the plan belonging to the active page', () => {
    const plans = { live: { ID: 'live-1' }, vod: { ID: 'vod-1' } }
    expect(playbackPlanForMode('live', plans)).toEqual(plans.live)
    expect(playbackPlanForMode('vod', plans)).toEqual(plans.vod)
    expect(playbackPlanForMode('home', plans)).toBeNull()
  })
})
