<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShellService } from '../bindings/github.com/unbox/unbox/internal/shell'

const platform = ref('…')
const playerReady = ref(false)
const loading = ref(false)
const loadError = ref('')

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
  } catch (e) {
    console.error('调用 ShellService 失败：', e)
  }
}

async function loadTestStream() {
  loading.value = true
  loadError.value = ''
  try {
    await ShellService.LoadTestStream()
  } catch (e) {
    loadError.value = String(e)
    console.error('加载测试流失败：', e)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
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
        <dd>{{ playerReady ? '是' : '否' }}</dd>
      </div>
    </dl>
    <p class="actions">
      <button class="load" :disabled="!playerReady || loading" @click="loadTestStream">
        {{ loading ? '加载中…' : '加载示例流' }}
      </button>
    </p>
    <p v-if="loadError" class="error">{{ loadError }}</p>
  </main>
</template>
