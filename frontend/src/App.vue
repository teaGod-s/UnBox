<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShellService } from '../bindings/github.com/unbox/unbox/internal/shell'

const platform = ref('…')
const playerReady = ref(false)

onMounted(async () => {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
  } catch (e) {
    console.error('调用 ShellService 失败：', e)
  }
})
</script>

<template>
  <main class="container">
    <h1 class="title">Unbox</h1>
    <p class="subtitle">IPTV 播放器</p>
    <dl class="status">
      <div class="row">
        <dt>当前平台</dt>
        <dd>{{ platform }}</dd>
      </div>
      <div class="row">
        <dt>播放器就绪</dt>
        <dd>{{ playerReady ? '是' : '否（Task 5 接线）' }}</dd>
      </div>
    </dl>
  </main>
</template>
