<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Hls, { type ErrorData, type Events } from 'hls.js'
import mpegts from 'mpegts.js'

export interface PlaybackPlan {
  ID: string
  Backend: 'web' | 'mpv'
  URL: string
  Kind: string
  CanFallback: boolean
}

const props = defineProps<{ plan: PlaybackPlan | null; seekTo?: number }>()
const emit = defineEmits<{ fallback: [id: string, position: number]; progress: [time: number, duration: number] }>()
const video = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null
let flv: ReturnType<typeof mpegts.createPlayer> | null = null
let fallbackSent = false
let networkRestarts = 0
let mediaRecoveries = 0

// 传输抖动（签名过期、CDN 限流）与「后端确实解不了」必须区别对待：前者原地重试，
// 后者直接换 mpv。预算内的重试只影响这一条播放，不重置后端选择。
const MAX_NETWORK_RESTARTS = 3
const MAX_MEDIA_RECOVERIES = 2

function cleanup() {
  hls?.destroy(); hls = null
  flv?.destroy(); flv = null
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
}

function requestFallback() {
  if (!fallbackSent && props.plan?.CanFallback && props.plan.Backend === 'web') {
    fallbackSent = true
    // 带上已看位置：mpv 据此续播，否则降级等于从头重播。
    emit('fallback', props.plan.ID, video.value?.currentTime || 0)
  }
}

// hls.js 的 fatal 只表示它自己的重试策略用尽，不等于后端能力不足。
// 1.7.1 里 attachMediaError 是 details 而非 type，挂载失败归入 MEDIA_ERROR。
function onHlsError(_event: Events, data: ErrorData) {
  if (!data.fatal) return
  console.warn('[hls] fatal', data.type, data.details)
  if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
    mediaRecoveries++
    if (mediaRecoveries <= MAX_MEDIA_RECOVERIES && hls) hls.recoverMediaError()
    else requestFallback()
    return
  }
  if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
    networkRestarts++
    // 无参调用：hls.js 会回到当前播放位置；传 undefined 会被算成 NaN。
    if (networkRestarts <= MAX_NETWORK_RESTARTS && hls) hls.startLoad()
    else requestFallback()
    return
  }
  requestFallback()
}

// mpegts 没有内部重试，也没有 recoverMediaError：传输错误只能靠 unload()+load() 重连，
// 编解码/能力错误（MEDIA_ERROR）重连也不会好，直接换 mpv。
function onMpegtsError(errType: string, errDetail: string, info: unknown) {
  console.warn('[mpegts] error', errType, errDetail, info)
  if (errType === mpegts.ErrorTypes.MEDIA_ERROR) {
    requestFallback()
    return
  }
  networkRestarts++
  if (networkRestarts <= MAX_NETWORK_RESTARTS && flv) {
    flv.unload(); flv.load()
  } else requestFallback()
}

function onTimeUpdate() {
  if (video.value) {
    emit('progress', video.value.currentTime, video.value.duration || 0)
  }
}

function applySeek() {
  const pos = props.seekTo
  if (pos && pos > 0 && video.value) {
    video.value.currentTime = pos
  }
}

function onLoadedMetadata() {
  applySeek()
}

async function attach(plan: PlaybackPlan | null) {
  cleanup()
  fallbackSent = false
  networkRestarts = 0
  mediaRecoveries = 0
  if (!plan || plan.Backend !== 'web') return
  await nextTick()
  const element = video.value
  if (!element) return
  if (plan.Kind === 'hls' && Hls.isSupported()) {
    hls = new Hls({ enableWorker: false })
    hls.on(Hls.Events.ERROR, onHlsError)
    hls.loadSource(plan.URL); hls.attachMedia(element); return
  }
  if ((plan.Kind === 'flv' || plan.Kind === 'ts') && mpegts.getFeatureList().mseLivePlayback) {
    flv = mpegts.createPlayer({ type: plan.Kind === 'flv' ? 'flv' : 'mpegts', url: plan.URL })
    flv.on(mpegts.Events.ERROR, onMpegtsError); flv.attachMediaElement(element); flv.load(); return
  }
  element.src = plan.URL
  element.addEventListener('error', requestFallback, { once: true })
}

watch(() => props.plan, attach, { immediate: true })
watch(() => props.seekTo, applySeek)
onBeforeUnmount(cleanup)
</script>

<template>
  <div class="playback-view">
    <video v-if="plan?.Backend === 'web'" ref="video" controls playsinline preload="metadata" @timeupdate="onTimeUpdate" @loadedmetadata="onLoadedMetadata" />
    <div v-else-if="plan?.Backend === 'mpv'" class="mpv-status">正在使用 mpv 播放</div>
    <div v-else class="playback-empty">选择频道或剧集开始播放</div>
  </div>
</template>
