<script setup lang="ts">
import { ref } from 'vue'
import type { TrackItem } from '../useHlsTracks'

const props = defineProps<{
  levels: TrackItem[]
  currentLevel: number
  audioTracks: TrackItem[]
  currentAudio: number
  subtitleTracks: TrackItem[]
  currentSubtitle: number
  selectLevel(i: number): void
  selectAudio(i: number): void
  selectSubtitle(i: number): void
  onLoadSubtitle(file: File): void
}>()

const fileInput = ref<HTMLInputElement | null>(null)

function pickSubtitle() {
  fileInput.value?.click()
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) props.onLoadSubtitle(file)
  input.value = ''
}
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
    <button class="load-subtitle" type="button" @click="pickSubtitle">加载字幕…</button>
    <input ref="fileInput" type="file" accept=".srt,.vtt" hidden @change="onFileChange" />
  </div>
</template>
