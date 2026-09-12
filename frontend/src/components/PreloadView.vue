<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ShellService } from '../../bindings/github.com/unbox/unbox/internal/shell'
import type { PlaybackPlan } from './PlaybackView.vue'

// PreloadView 只负责 Web 下一集的隐藏预载：用独立媒体元素拉取资源，
// 不接管当前播放元素，也不决定切集或换源。
const props = defineProps<{ plan: PlaybackPlan | null }>()
const emit = defineEmits<{ ready: [id: string]; error: [id: string] }>()

const video = ref<HTMLVideoElement | null>(null)
let generation = 0

// releasePreload 尽力释放后端预载会话；失败只影响预载，不影响当前播放。
function releasePreload(id: string) {
  if (!id) return
  try {
    void Promise.resolve(ShellService.ReleasePreload(id)).catch(() => {})
  } catch {
    // 预载释放失败无需上报
  }
}

function detach() {
  const element = video.value
  if (!element) return
  element.pause()
  element.removeAttribute('src')
  element.load()
}

async function attach(plan: PlaybackPlan | null) {
  const current = ++generation
  detach()
  if (!plan || plan.Backend !== 'web' || !plan.URL) return
  await nextTick()
  // 快速切换计划时只保留最新一次挂载。
  if (current !== generation || plan !== props.plan) return
  const element = video.value
  if (!element) return
  element.src = plan.URL
  element.load()
}

watch(
  () => props.plan,
  (plan, previous) => {
    if (previous?.ID && previous.ID !== plan?.ID) releasePreload(previous.ID)
    void attach(plan)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  generation++
  detach()
  releasePreload(props.plan?.ID ?? '')
})

function onReady() {
  if (props.plan?.ID) emit('ready', props.plan.ID)
}

function onError() {
  if (props.plan?.ID) emit('error', props.plan.ID)
}
</script>

<template>
  <video
    v-if="plan?.Backend === 'web' && plan?.URL"
    ref="video"
    class="preload-video"
    muted
    playsinline
    preload="auto"
    @canplay="onReady"
    @error="onError"
  />
</template>
