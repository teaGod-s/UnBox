// playbackSettings.ts — 设置页「播放设置」分类的加载、持久化与展示定义。
//
// 三个开关都由这里集中管理：读取时归一化（缺失/非法值按关闭），切换时先更新
// 运行时值再持久化，持久化失败保留运行时值并把错误交给调用方展示。
import { ref, type Ref } from 'vue'
import { normalizePlaybackSettings, type PlaybackSettings } from './playbackAutomation'

export type PlaybackSettingKey = keyof PlaybackSettings

/** 后端设置接口的最小形状（对应 ShellService 的两个绑定方法）。 */
export interface PlaybackSettingsApi {
  GetPlaybackSettings(): Promise<Partial<PlaybackSettings> | null | undefined>
  SetPlaybackSettings(settings: PlaybackSettings): Promise<unknown>
}

export interface PlaybackSettingItem {
  key: PlaybackSettingKey
  label: string
  hint: string
}

/** 设置页展示的三行开关，顺序即界面顺序。 */
export const PLAYBACK_SETTING_ITEMS: readonly PlaybackSettingItem[] = [
  { key: 'AutoNext', label: '自动切集', hint: '当前线路有下一集时自动播放下一集' },
  { key: 'AutoSwitchSource', label: '自动换源', hint: '播放出错或持续缓冲 30 秒时按线路顺序换源' },
  { key: 'PreloadNext', label: '预载下一集', hint: '提前准备下一集资源，失败不影响当前播放' },
]

export interface PlaybackSettingsStore {
  settings: Ref<PlaybackSettings>
  load: () => Promise<void>
  set: (key: PlaybackSettingKey, value: boolean) => Promise<void>
}

/**
 * createPlaybackSettings 管理三个开关的运行时状态与后端持久化。
 * onError 用于把持久化/读取错误交给现有错误提示。
 */
export function createPlaybackSettings(
  api: PlaybackSettingsApi,
  onError: (err: unknown) => void = () => {},
): PlaybackSettingsStore {
  const settings = ref<PlaybackSettings>(normalizePlaybackSettings(null))

  async function load(): Promise<void> {
    try {
      settings.value = normalizePlaybackSettings(await api.GetPlaybackSettings())
    } catch (err) {
      // 读取失败回退为全部关闭，但不改坏已有运行时值之外的任何状态。
      settings.value = normalizePlaybackSettings(null)
      onError(err)
    }
  }

  async function set(key: PlaybackSettingKey, value: boolean): Promise<void> {
    // 先更新界面值：持久化失败也保留用户看到的选择。
    settings.value = { ...settings.value, [key]: value === true }
    try {
      await api.SetPlaybackSettings({ ...settings.value })
    } catch (err) {
      onError(err)
    }
  }

  return { settings, load, set }
}
