<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShellService, type SourceInfo, type Section, type VodItem, type EpisodeInfo, type VodMedia } from '../bindings/github.com/unbox/unbox/internal/shell'

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
const mode = ref<'live' | 'vod'>('live')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
const vodItems = ref<VodItem[]>([])
const vodDetail = ref<VodMedia | null>(null)
const vodQuery = ref('')
const vodPage = ref(0)

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

async function loadSources() {
  sources.value = (await ShellService.Sources()) ?? []
  if (!activeSite.value) {
    activeSite.value = sources.value.find(s => s.Kind === 'vod')?.ID ?? ''
  }
}

async function switchMode(m: 'live' | 'vod') {
  mode.value = m
  if (m === 'vod') await loadSources()
}

async function selectSite(id: string) {
  activeSite.value = id
  vodDetail.value = null
  vodPage.value = 0
  await reloadVodCategories()
}

async function reloadVodCategories() {
  vodCategories.value = (await ShellService.VodCategories(activeSite.value)) ?? []
  vodActiveCat.value = vodCategories.value[0]?.ID ?? ''
  await reloadVodList()
}

async function reloadVodList() {
  vodItems.value = (await ShellService.VodList(activeSite.value, vodActiveCat.value, vodPage.value)) ?? []
}

async function vodSearch() {
  if (!vodQuery.value) { await reloadVodList(); return }
  vodItems.value = (await ShellService.VodSearch(activeSite.value, vodQuery.value)) ?? []
}

async function openVodDetail(item: VodItem) {
  errMsg.value = ''
  try {
    vodDetail.value = await ShellService.VodDetail(activeSite.value, item.ID)
  } catch (e) { errMsg.value = String(e) }
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  try {
    await ShellService.PlayVod(activeSite.value, ep.ID)
    nowPlaying.value = ep.Name
  } catch (e) { errMsg.value = String(e) }
}

onMounted(refresh)
</script>

<template>
  <main class="container">
    <header>
      <h1 class="title">Unbox</h1>
      <p class="subtitle">{{ platform }} · 播放器{{ playerReady ? '就绪' : '未就绪' }}</p>
    </header>

    <nav class="tabs">
      <button :class="{ active: mode === 'live' }" @click="switchMode('live')">直播</button>
      <button :class="{ active: mode === 'vod' }" @click="switchMode('vod')">点播</button>
    </nav>

    <section class="import">
      <input v-model="importUrl" placeholder="粘贴订阅链接或本地路径" />
      <button :disabled="loading" @click="doImport">{{ loading ? '导入中…' : '导入订阅' }}</button>
      <span v-if="importSummary" class="ok">{{ importSummary }}</span>
    </section>

    <section v-if="mode === 'live'" class="layout">
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

    <section v-if="mode === 'vod'" class="vod">
      <div class="vod-toolbar">
        <select v-model="activeSite" @change="selectSite(activeSite)">
          <option v-for="s in sources.filter(x => x.Kind === 'vod')" :key="s.ID" :value="s.ID">{{ s.Name }}</option>
        </select>
      </div>

      <div class="vod-layout">
        <aside class="vod-cats">
          <button v-for="c in vodCategories" :key="c.ID" :class="{ active: c.ID === vodActiveCat }"
                  @click="vodActiveCat = c.ID; vodPage = 0; reloadVodList()">{{ c.Title }}</button>
        </aside>

        <section class="vod-main">
          <div class="search"><input v-model="vodQuery" placeholder="搜索影片" @input="vodSearch" /></div>
          <ul v-if="!vodDetail">
            <li v-for="it in vodItems" :key="it.ID" class="channel" @click="openVodDetail(it)">
              <span class="name">{{ it.Title }}</span><span class="group">{{ it.Group }}</span>
            </li>
          </ul>
          <div v-else class="vod-detail">
            <button @click="vodDetail = null">← 返回</button>
            <h2>{{ vodDetail.Title }}</h2>
            <p class="meta">{{ vodDetail.Type }} · {{ vodDetail.Year }} · {{ vodDetail.Area }}</p>
            <p>{{ vodDetail.Description }}</p>
            <div v-for="src in (vodDetail.Sources ?? [])" :key="src" class="ep-src">
              <p class="ep-src-name">{{ src }}</p>
              <button v-for="ep in (vodDetail.Episodes ?? []).filter(e => e.Source === src)" :key="ep.ID"
                      @click="playEpisode(ep)">{{ ep.Name }}</button>
            </div>
          </div>
        </section>
      </div>
    </section>

    <p v-if="errMsg" class="error">{{ errMsg }}</p>
  </main>
</template>
