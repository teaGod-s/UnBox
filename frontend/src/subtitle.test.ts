import { beforeEach, describe, expect, it, vi } from 'vitest'
import { attachSubtitleTrack, loadSubtitleFile, removeSubtitleTrack, srtToVtt } from './subtitle'

beforeEach(() => {
  vi.stubGlobal('URL', { ...URL, createObjectURL: vi.fn(() => 'blob:fake'), revokeObjectURL: vi.fn() })
})

describe('subtitle helpers', () => {
  it('srtToVtt 时间轴逗号换点、前缀 WEBVTT', () => {
    const out = srtToVtt('1\n00:00:01,000 --> 00:00:04,000\n你好\n')
    expect(out.startsWith('WEBVTT\n\n')).toBe(true)
    expect(out).toContain('00:00:01.000 --> 00:00:04.000')
    expect(out).not.toContain(',000 -->')
  })

  it('attachSubtitleTrack 注入 <track kind=subtitles> 并返回元素', () => {
    const video = { appendChild: vi.fn() } as any
    const el = attachSubtitleTrack(video, 'WEBVTT\n\n', '外挂', 'zh')
    expect(video.appendChild).toHaveBeenCalledTimes(1)
    const track = video.appendChild.mock.calls[0][0]
    expect(track.kind).toBe('subtitles')
    expect(track.src).toBe('blob:fake')
    expect(track.label).toBe('外挂')
    expect(track.srclang).toBe('zh')
    expect(el).toBe(track)
  })

  it('removeSubtitleTrack 移除元素并 revoke blob URL', () => {
    const video = { appendChild: vi.fn() } as any
    const el = attachSubtitleTrack(video, 'WEBVTT\n\n', '外挂')
    ;(el as any).remove = vi.fn()
    removeSubtitleTrack(el)
    expect((el as any).remove).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fake')
  })

  it('loadSubtitleFile：.srt 转 vtt、.vtt 直用', async () => {
    const video = { appendChild: vi.fn() } as any
    const mkFile = (name: string, text: string) => ({ name, text: () => Promise.resolve(text) }) as unknown as File
    const srtEl = await loadSubtitleFile(video, mkFile('a.srt', '1\n00:00:01,000 --> 00:00:02,000\nx\n'))
    expect(srtEl).not.toBeNull()
    const vttEl = await loadSubtitleFile(video, mkFile('b.vtt', 'WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nx\n'))
    expect(vttEl).not.toBeNull()
  })
})
