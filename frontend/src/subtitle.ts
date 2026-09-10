// 把 srt 时间轴逗号换成点，前面加 WEBVTT 头，交给原生 <track> 解析。
export function srtToVtt(srt: string): string {
  const body = srt.replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, '$1.$2')
  return `WEBVTT\n\n${body}`
}

export function attachSubtitleTrack(video: HTMLVideoElement, vtt: string, label: string, lang?: string): HTMLTrackElement {
  const url = URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' }))
  const el = document.createElement('track')
  el.kind = 'subtitles'
  el.src = url
  el.label = label
  if (lang) el.srclang = lang
  el.default = true
  video.appendChild(el)
  return el
}

export function removeSubtitleTrack(el: HTMLTrackElement | null): void {
  if (!el) return
  if (el.src?.startsWith('blob:')) URL.revokeObjectURL(el.src)
  el.remove()
}

export async function loadSubtitleFile(video: HTMLVideoElement, file: File): Promise<HTMLTrackElement | null> {
  try {
    const text = await file.text()
    const vtt = file.name.toLowerCase().endsWith('.srt') ? srtToVtt(text) : text
    return attachSubtitleTrack(video, vtt, file.name)
  } catch (e) {
    console.warn('[subtitle] 读取字幕文件失败：', e)
    return null
  }
}
