export interface LibraryThumbItem {
  Path: string
  MTime: number
  Poster: string
}

export interface LibraryThumbAPI {
  EnsureThumb(path: string, mtime: number): Promise<[videoURL: string, posterURL: string, cached: boolean]>
  SaveThumb(path: string, mtime: number, jpeg: Uint8Array): Promise<string>
  GenerateThumbMpv(path: string, mtime: number): Promise<string>
}

interface PipelineOptions {
  captureFrame?: (videoURL: string) => Promise<Uint8Array>
  maxConcurrent?: number
  onPoster?: (item: LibraryThumbItem) => void
}

function captureVideoFrame(videoURL: string): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video')
    const canvas = document.createElement('canvas')
    const timeout = window.setTimeout(() => finish(new Error('首帧生成超时')), 10_000)
    let done = false

    const cleanup = () => {
      window.clearTimeout(timeout)
      video.removeEventListener('loadeddata', onLoadedData)
      video.removeEventListener('seeked', onSeeked)
      video.removeEventListener('error', onError)
      video.remove()
      canvas.remove()
    }
    const finish = (err: Error | null, bytes?: Uint8Array) => {
      if (done) return
      done = true
      cleanup()
      if (err) reject(err)
      else resolve(bytes ?? new Uint8Array())
    }
    const onError = () => finish(new Error('浏览器无法解码视频'))
    const onLoadedData = () => {
      const duration = Number.isFinite(video.duration) ? video.duration : 0
      const seek = Math.min(Math.max(duration * 0.1, 0), 60)
      if (seek === 0) {
        onSeeked()
      } else {
        try {
          video.currentTime = seek
        } catch (err) {
          finish(err instanceof Error ? err : new Error(String(err)))
        }
      }
    }
    const onSeeked = () => {
      const width = video.videoWidth || 320
      const height = video.videoHeight || 180
      canvas.width = width
      canvas.height = height
      try {
        const context = canvas.getContext('2d')
        if (!context) throw new Error('无法创建首帧画布')
        context.drawImage(video, 0, 0, width, height)
        canvas.toBlob(async blob => {
          if (!blob) {
            finish(new Error('首帧图片为空'))
            return
          }
          try {
            finish(null, new Uint8Array(await blob.arrayBuffer()))
          } catch (err) {
            finish(err instanceof Error ? err : new Error(String(err)))
          }
        }, 'image/jpeg', 0.8)
      } catch (err) {
        finish(err instanceof Error ? err : new Error(String(err)))
      }
    }

    video.crossOrigin = 'anonymous'
    video.preload = 'auto'
    video.muted = true
    video.style.position = 'fixed'
    video.style.width = '1px'
    video.style.height = '1px'
    video.style.opacity = '0'
    video.style.pointerEvents = 'none'
    video.addEventListener('loadeddata', onLoadedData)
    video.addEventListener('seeked', onSeeked)
    video.addEventListener('error', onError)
    document.body.append(video)
    video.src = videoURL
    video.load()
  })
}

function createSemaphore(limit: number) {
  let active = 0
  const queue: Array<() => void> = []
  return {
    async run<T>(task: () => Promise<T>): Promise<T> {
      if (active >= limit) await new Promise<void>(resolve => queue.push(resolve))
      active++
      try {
        return await task()
      } finally {
        active--
        queue.shift()?.()
      }
    },
  }
}

export function createLibraryThumbPipeline(api: LibraryThumbAPI, options: PipelineOptions = {}) {
  const attempted = new Set<string>()
  const semaphore = createSemaphore(Math.max(1, options.maxConcurrent ?? 2))
  const capture = options.captureFrame ?? captureVideoFrame

  function applyPoster(item: LibraryThumbItem, poster: string) {
    item.Poster = poster
    if (poster) options.onPoster?.(item)
  }

  async function ensure(item: LibraryThumbItem): Promise<void> {
    if (item.Poster || attempted.has(item.Path)) return
    let videoURL = ''
    try {
      const ensured = await api.EnsureThumb(item.Path, item.MTime)
      videoURL = ensured[0]
      if (ensured[2]) {
        applyPoster(item, ensured[1])
        return
      }
      await semaphore.run(async () => {
        try {
          const jpeg = await capture(videoURL)
          applyPoster(item, await api.SaveThumb(item.Path, item.MTime, jpeg))
        } catch {
          try {
            applyPoster(item, await api.GenerateThumbMpv(item.Path, item.MTime))
          } catch {
            attempted.add(item.Path)
          }
        }
      })
    } catch {
      attempted.add(item.Path)
    }
  }

  return {
    attempted,
    ensure,
    ensureAll(items: LibraryThumbItem[]) {
      return Promise.all(items.filter(item => !item.Poster).map(item => ensure(item)))
    },
  }
}
