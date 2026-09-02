export type PlaybackMode = 'home' | 'vod' | 'live' | 'search' | 'favorites' | 'settings'
export type PlaybackScope = 'live' | 'vod'
export type PlaybackStatus = 'idle' | 'preparing' | 'playing' | 'error'

export interface ActivePlaybackSession {
  scope: PlaybackScope
  token: number
}

export function shouldPauseStalePlayback(active: ActivePlaybackSession | null): boolean {
  return active === null
}

export function playbackPlanForMode<T>(mode: PlaybackMode, plans: { live: T | null; vod: T | null }, owner?: PlaybackScope): T | null {
  if (owner && owner !== mode) return null
  if (mode === 'live') return plans.live
  if (mode === 'vod') return plans.vod
  return null
}

export async function resolvePlaybackFallback<T>(
  scope: PlaybackScope,
  request: () => Promise<T>,
  isCurrent: () => boolean,
  apply: (scope: PlaybackScope, plan: T) => void,
): Promise<boolean> {
  const plan = await request()
  if (!isCurrent()) return false
  apply(scope, plan)
  return true
}

export function shouldRecordVodProgress(mode: PlaybackMode, vodView: string): boolean {
  return mode === 'vod' && vodView === 'detail'
}
