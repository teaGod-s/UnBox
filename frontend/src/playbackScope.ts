export type PlaybackMode = 'home' | 'vod' | 'live' | 'library' | 'search' | 'favorites' | 'settings'
export type PlaybackScope = 'live' | 'vod'
export type PlaybackStatus = 'idle' | 'preparing' | 'playing' | 'error'

export function shouldShowMpvInstallPrompt(platform: string, mpvReady: boolean, mpvNeeded: boolean): boolean {
  if (mpvReady) return false
  if (platform === 'windows' || platform === 'darwin') return mpvNeeded
  return platform === 'linux' || mpvNeeded
}

// MPV_UNAVAILABLE_ERROR 与后端 playback.ErrMPVUnavailable 的文案一一对应。
// 后端在「内容需要 mpv 但播放器不可用」时原样返回它（controller.go 的 loadMPV），
// 前端据此判断这次播放失败是不是缺 mpv。任一边改了文案都会被
// App.playback.test.ts 的配对断言拦住。
export const MPV_UNAVAILABLE_ERROR = 'mpv 插件未安装'

// isMpvUnavailableError 判定播放失败是否由 mpv 缺失引起。
//
// 不能只靠 Web→mpv 降级来置位安装提示：本地媒体库、HEVC 的 HLS、RTMP 会**直接**
// 路由到 mpv，根本不经过 Web，那条路径永远不会触发降级回调。
export function isMpvUnavailableError(message: string): boolean {
  return message.includes(MPV_UNAVAILABLE_ERROR)
}

// mpvInstallHint 生成 mpv 缺失时的说明文案。
//
// Windows 上便携版是单个 exe、不含 mpv，用户往往不知道自己缺了什么，
// 所以要点明便携版的限制以及哪些内容会因此播不了。
export function mpvInstallHint(platform: string): string {
  const need = '未检测到 mpv，HEVC / RTMP / 本地视频需要它。'
  if (platform === 'windows') {
    return `${need}便携版不包含 mpv，可点下方按钮自动下载安装。`
  }
  return need
}

export interface ActivePlaybackSession {
  scope: PlaybackScope
  token: number
}

export function shouldPauseStalePlayback(active: ActivePlaybackSession | null): boolean {
  return active === null
}

export function playbackPlanForMode<T>(mode: PlaybackMode, plans: { live: T | null; vod: T | null }, owner?: PlaybackScope): T | null {
  if (owner && owner !== mode && !(mode === 'library' && owner === 'vod')) return null
  if (mode === 'live') return plans.live
  if (mode === 'vod' || mode === 'library') return plans.vod
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
  return (mode === 'vod' && vodView === 'detail') || mode === 'library'
}
