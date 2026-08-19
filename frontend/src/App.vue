<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShellService } from '../bindings/github.com/unbox/unbox/internal/shell'

interface ChannelInfo { ID: string; Name: string; Group: string; Logo: string; Favorited: boolean }

const platform = ref('…')
const playerReady = ref(false)
const groups = ref<string[]>([])
const channels = ref<ChannelInfo[]>([])
const favorites = ref<ChannelInfo[]>([])
const activeGroup = ref('*')
const query = ref('')
const nowPlaying = ref('')
const importUrl = ref('')
const importSummary = ref('')
const errMsg = ref('')
const loading = ref(false)

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
    await reloadGroups()
  } catch (e) { errMsg.value = String(e) }
}

async function reloadGroups() {
  groups.value = ['*', ...((await ShellService.Groups()) ?? [])]
  await reloadChannels()
}

async function reloadChannels() {
  channels.value = (await ShellService.Channels(activeGroup.value === '*' ? '' : activeGroup.value, 0)) ?? []
}

async function doSearch() {
  if (!query.value) { await reloadChannels(); return }
  channels.value = (await ShellService.Search(query.value)) ?? []
}

async function doImport() {
  loading.value = true; errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.ImportSubscription(importUrl.value)
    importSummary.value = `导入成功：${r.Groups} 组 / ${r.Channels} 频道`
    await reloadGroups()
  } catch (e) { errMsg.value = String(e) }
  finally { loading.value = false }
}

async function play(c: ChannelInfo) {
  errMsg.value = ''
  try {
    await ShellService.PlayChannel(c.ID)
    nowPlaying.value = c.Name
  } catch (e) { errMsg.value = String(e) }
}

async function toggleFav(c: ChannelInfo) {
  try {
    if (c.Favorited) await ShellService.RemoveFavorite(c.ID)
    else await ShellService.AddFavorite(c.ID)
    c.Favorited = !c.Favorited
    favorites.value = (await ShellService.ListFavorites()) ?? []
  } catch (e) { errMsg.value = String(e) }
}

async function loadFavorites() { favorites.value = (await ShellService.ListFavorites()) ?? [] }

async function pause() { await ShellService.Pause() }
async function resume() { await ShellService.Resume() }
async function setVolume(e: Event) { await ShellService.SetVolume(Number((e.target as HTMLInputElement).value)) }

onMounted(refresh)
</script>

<template>
  <main class="container">
    <header>
      <h1 class="title">Unbox</h1>
      <p class="subtitle">{{ platform }} · 播放器{{ playerReady ? '就绪' : '未就绪' }}</p>
    </header>

    <section class="import">
      <input v-model="importUrl" placeholder="粘贴订阅链接或本地路径" />
      <button :disabled="loading" @click="doImport">{{ loading ? '导入中…' : '导入订阅' }}</button>
      <span v-if="importSummary" class="ok">{{ importSummary }}</span>
    </section>

    <section class="layout">
      <aside class="groups">
        <button v-for="g in groups" :key="g" :class="{ active: g === activeGroup }" @click="activeGroup = g; reloadChannels()">
          {{ g === '*' ? '全部' : g }}
        </button>
        <hr />
        <button @click="loadFavorites">⭐ 收藏</button>
      </aside>

      <section class="channels">
        <div class="search"><input v-model="query" placeholder="搜索频道" @input="doSearch" /></div>
        <ul>
          <li v-for="c in channels" :key="c.ID" class="channel">
            <span class="name">{{ c.Name }}</span>
            <span class="group">{{ c.Group }}</span>
            <button @click="play(c)">▶ 播放</button>
            <button @click="toggleFav(c)">{{ c.Favorited ? '★' : '☆' }}</button>
          </li>
        </ul>
      </section>

      <aside class="player">
        <p v-if="nowPlaying" class="now">正在播放：{{ nowPlaying }}</p>
        <div class="controls" v-if="nowPlaying">
          <button @click="pause">暂停</button>
          <button @click="resume">继续</button>
          <input type="range" min="0" max="100" @input="setVolume" />
        </div>
        <p v-if="favorites.length" class="favhead">收藏</p>
        <ul class="favs">
          <li v-for="f in favorites" :key="f.ID" @click="play(f)">{{ f.Name }}</li>
        </ul>
      </aside>
    </section>

    <p v-if="errMsg" class="error">{{ errMsg }}</p>
  </main>
</template>
