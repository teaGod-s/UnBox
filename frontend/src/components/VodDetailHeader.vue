<script setup lang="ts">
defineProps<{ nowPlaying: string }>()
defineEmits<{ back: [] }>()
</script>

<template>
  <div class="vod-detail-head">
    <button class="vod-back" type="button" @click="$emit('back')">← 返回</button>
    <p v-if="nowPlaying" class="now">正在播放：{{ nowPlaying }}</p>
  </div>
</template>
