import { describe, expect, it, vi } from 'vitest'
import { createLibraryThumbPipeline, type LibraryThumbAPI, type LibraryThumbItem } from './libraryThumb'

function item(path: string, poster = ''): LibraryThumbItem {
  return { Path: path, MTime: 1, Poster: poster }
}

function apiFor(overrides: Partial<LibraryThumbAPI> = {}): LibraryThumbAPI {
  return {
    EnsureThumb: vi.fn(async (path: string) => [`video:${path}`, `poster:${path}`, false] as [string, string, boolean]),
    SaveThumb: vi.fn(async (path: string) => `saved:${path}`),
    GenerateThumbMpv: vi.fn(async (path: string) => `mpv:${path}`),
    ...overrides,
  }
}

describe('library thumbnail pipeline', () => {
  it('uses a cached poster without creating a video', async () => {
    const api = apiFor({ EnsureThumb: vi.fn(async () => ['video', 'cached', true] as [string, string, boolean]) })
    const target = item('/a.mp4')
    const pipeline = createLibraryThumbPipeline(api)

    await pipeline.ensure(target)

    expect(target.Poster).toBe('cached')
    expect(api.GenerateThumbMpv).not.toHaveBeenCalled()
  })

  it('saves a poster from the web capture path', async () => {
    const api = apiFor()
    const target = item('/a.mp4')
    const capture = vi.fn(async () => new Uint8Array([1, 2, 3]))
    const onPoster = vi.fn()
    const pipeline = createLibraryThumbPipeline(api, { captureFrame: capture, onPoster })

    await pipeline.ensure(target)

    expect(capture).toHaveBeenCalledWith('video:/a.mp4')
    expect(api.SaveThumb).toHaveBeenCalledWith('/a.mp4', 1, new Uint8Array([1, 2, 3]))
    expect(target.Poster).toBe('saved:/a.mp4')
    expect(onPoster).toHaveBeenCalledWith(target)
    expect(api.GenerateThumbMpv).not.toHaveBeenCalled()
  })

  it('falls back to mpv after web capture fails', async () => {
    const api = apiFor()
    const target = item('/a.mkv')
    const pipeline = createLibraryThumbPipeline(api, {
      captureFrame: vi.fn(async () => { throw new Error('decode failed') }),
    })

    await pipeline.ensure(target)

    expect(api.GenerateThumbMpv).toHaveBeenCalledWith('/a.mkv', 1)
    expect(target.Poster).toBe('mpv:/a.mkv')
  })

  it('remembers a failed item and does not retry it', async () => {
    const api = apiFor({ GenerateThumbMpv: vi.fn(async () => { throw new Error('no mpv') }) })
    const target = item('/a.mkv')
    const capture = vi.fn(async () => { throw new Error('decode failed') })
    const pipeline = createLibraryThumbPipeline(api, { captureFrame: capture })

    await pipeline.ensure(target)
    await pipeline.ensure(target)

    expect(pipeline.attempted.has('/a.mkv')).toBe(true)
    expect(capture).toHaveBeenCalledTimes(1)
    expect(api.GenerateThumbMpv).toHaveBeenCalledTimes(1)
  })

  it('limits only the generation stage to two concurrent captures', async () => {
    const api = apiFor()
    let running = 0
    let peak = 0
    const capture = vi.fn(async () => {
      running++
      peak = Math.max(peak, running)
      await new Promise(resolve => setTimeout(resolve, 5))
      running--
      return new Uint8Array([1])
    })
    const targets = ['/a.mp4', '/b.mp4', '/c.mp4', '/d.mp4'].map(path => item(path))
    const pipeline = createLibraryThumbPipeline(api, { captureFrame: capture, maxConcurrent: 2 })

    await pipeline.ensureAll(targets)

    expect(peak).toBe(2)
    expect(capture).toHaveBeenCalledTimes(4)
    expect(targets.every(target => target.Poster)).toBe(true)
  })
})
