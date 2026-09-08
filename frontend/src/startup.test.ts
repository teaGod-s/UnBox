import { describe, expect, it } from 'vitest'
import { initializeHomeState } from './startup'

describe('startup initialization', () => {
  it('loads local library items before refreshing home history', async () => {
    const events: string[] = []

    await initializeHomeState(
      async () => { events.push('library') },
      async () => { events.push('home') },
    )

    expect(events).toEqual(['library', 'home'])
  })
})
