<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Hls from 'hls.js'
import mpegts from 'mpegts.js'

export interface PlaybackPlan {
  ID: string
  Backend: 'web' | 'mpv'
  URL: string
  Kind: string
  CanFallback: boolean
}

const props = defineProps<{ plan: PlaybackPlan | null }>()
const emit = defineEmits<{ fallback: [id: string]; progress: [time: number, duration: number] }>()
const video = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null
let flv: ReturnType<typeof mpegts.createPlayer> | null = null
let fallbackSent = false

function cleanup() {
  hls?.destroy(); hls = null
  flv?.destroy(); flv = null
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
}

function requestFallback() {
  if (!fallbackSent && props.plan?.CanFallback && props.plan.Backend === 'web') {
    fallbackSent = true
    emit('fallback', props.plan.ID)
  }
}

function onTimeUpdate() {
  if (video.value) {
    emit('progress', video.value.currentTime, video.value.duration || 0)
  }
}

async function attach(plan: PlaybackPlan | null) {
  cleanup(); fallbackSent = false
  if (!plan || plan.Backend !== 'web') return
  await nextTick()
  const element = video.value
  if (!element) return
  if (plan.Kind === 'hls' && Hls.isSupported()) {
    hls = new Hls({ enableWorker: false })
    hls.on(Hls.Events.ERROR, (_event, data) => { if (data.fatal) requestFallback() })
    hls.loadSource(plan.URL); hls.attachMedia(element); return
  }
  if ((plan.Kind === 'flv' || plan.Kind === 'ts') && mpegts.getFeatureList().mseLivePlayback) {
    flv = mpegts.createPlayer({ type: plan.Kind === 'flv' ? 'flv' : 'mpegts', url: plan.URL })
    flv.on(mpegts.Events.ERROR, requestFallback); flv.attachMediaElement(element); flv.load(); return
  }
  element.src = plan.URL
  element.addEventListener('error', requestFallback, { once: true })
}

watch(() => props.plan, attach, { immediate: true })
onBeforeUnmount(cleanup)
</script>

<template>
  <div class="playback-view">
    <video v-if="plan?.Backend === 'web'" ref="video" controls playsinline preload="metadata" @timeupdate="onTimeUpdate" />
    <div v-else-if="plan?.Backend === 'mpv'" class="mpv-status">正在使用 mpv 播放</div>
    <div v-else class="playback-empty">选择频道或剧集开始播放</div>
  </div>
</template>
