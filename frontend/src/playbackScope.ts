export type PlaybackMode = 'home' | 'vod' | 'live' | 'settings'

export function playbackPlanForMode<T>(mode: PlaybackMode, plans: { live: T | null; vod: T | null }): T | null {
  if (mode === 'live') return plans.live
  if (mode === 'vod') return plans.vod
  return null
}
