<script setup lang="ts">
import type { TrackItem } from '../useHlsTracks'

defineProps<{
  levels: TrackItem[]
  currentLevel: number
  audioTracks: TrackItem[]
  currentAudio: number
  subtitleTracks: TrackItem[]
  currentSubtitle: number
  selectLevel(i: number): void
  selectAudio(i: number): void
  selectSubtitle(i: number): void
}>()
</script>

<template>
  <div class="track-menu">
    <section v-if="levels.length" class="track-sec">
      <h4>清晰度</h4>
      <ul>
        <li v-for="level in levels" :key="level.index" :aria-current="level.index === currentLevel ? 'true' : undefined"
            :class="{ active: level.index === currentLevel }" @click="selectLevel(level.index)">
          {{ level.label }}
        </li>
      </ul>
    </section>
    <section v-if="audioTracks.length" class="track-sec">
      <h4>音轨</h4>
      <ul>
        <li v-for="track in audioTracks" :key="track.index" :aria-current="track.index === currentAudio ? 'true' : undefined"
            :class="{ active: track.index === currentAudio }" @click="selectAudio(track.index)">
          {{ track.label }}
        </li>
      </ul>
    </section>
    <section v-if="subtitleTracks.length" class="track-sec">
      <h4>字幕</h4>
      <ul>
        <li v-for="track in subtitleTracks" :key="track.index" :aria-current="track.index === currentSubtitle ? 'true' : undefined"
            :class="{ active: track.index === currentSubtitle }" @click="selectSubtitle(track.index)">
          {{ track.label }}
        </li>
      </ul>
    </section>
  </div>
</template>
