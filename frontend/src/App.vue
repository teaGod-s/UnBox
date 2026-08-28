<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Events } from '@wailsio/runtime'
import { ShellService, type SourceInfo, type Section, type VodItem, type EpisodeInfo, type VodMedia, type SourceRecord, type VodHistoryInfo } from '../bindings/github.com/unbox/unbox/internal/shell'
import PlaybackView, { type PlaybackPlan } from './components/PlaybackView.vue'
import DOMPurify from 'dompurify'

interface ChannelInfo { ID: string; Name: string; Group: string; Logo: string; Favorited: boolean }
interface Progress { Stage: string; Message: string; Done: number; Total: number }

const platform = ref('…')
const playerReady = ref(false)
const groups = ref<string[]>([])
const channels = ref<ChannelInfo[]>([])
const favorites = ref<ChannelInfo[]>([])
const activeGroup = ref('*')
const query = ref('')
const nowPlaying = ref('')
const importSummary = ref('')
const errMsg = ref('')
const importProgress = ref<Progress | null>(null)
const mode = ref<'home' | 'vod' | 'live' | 'settings'>('home')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
const vodItems = ref<VodItem[]>([])
const vodDetail = ref<VodMedia | null>(null)
const vodQuery = ref('')
const vodPage = ref(0)
const playbackPlan = ref<PlaybackPlan | null>(null)
const mpvReady = ref(false)
const mpvInstallMode = ref('')
const installMessage = ref('')
const vodSources = ref<SourceRecord[]>([])
const liveSources = ref<SourceRecord[]>([])
const vodSourceUrl = ref('')
const liveSourceUrl = ref('')
const homeHistory = ref<VodHistoryInfo[]>([])
const currentVod = ref<{ site: string; vodID: string } | null>(null)
let lastProgressSave = 0

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
    await refreshMpvStatus()
    await refreshHome()
  } catch (e) { errMsg.value = String(e) }
}

async function refreshMpvStatus() {
  const s = await ShellService.MPVStatus()
  mpvReady.value = s.Available
  mpvInstallMode.value = s.InstallMode
}

async function installMpv() {
  errMsg.value = ''; installMessage.value = ''
  try {
    const r = await ShellService.InstallMPV()
    installMessage.value = r.Message
    if (r.Installed) { installMessage.value = ''; await refreshMpvStatus() }
  } catch (e) { errMsg.value = String(e) }
}

async function recheckMpv() {
  try {
    const s = await ShellService.RefreshMPV()
    mpvReady.value = s.Available
    if (s.Available) installMessage.value = ''
  } catch (e) { errMsg.value = String(e) }
}

async function switchMode(m: 'home' | 'vod' | 'live' | 'settings') {
  mode.value = m
  if (m === 'vod') await refreshVod()
  else if (m === 'live') await reloadGroups()
  else if (m === 'home') await refreshHome()
  else if (m === 'settings') await reloadSourceHistory()
}

async function refreshHome() {
  try {
    homeHistory.value = (await ShellService.ListVodHistory()) ?? []
  } catch (e) { errMsg.value = String(e) }
}

function fmtProgress(sec: number) {
  if (sec <= 0) return ''
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return m > 0 ? `${m}分${s}秒` : `${s}秒`
}

async function resumeVod(h: VodHistoryInfo) {
  errMsg.value = ''
  try {
    mode.value = 'vod'
    await loadSources()
    activeSite.value = h.Site
    vodDetail.value = await ShellService.VodDetail(h.Site, h.VodID)
  } catch (e) { errMsg.value = String(e) }
}

// ---- 设置：源管理 ----
async function reloadSourceHistory() {
  try {
    const all = (await ShellService.ListSources()) ?? []
    vodSources.value = all.filter(s => s.Kind === 'vod')
    liveSources.value = all.filter(s => s.Kind === 'live')
    vodSourceUrl.value = vodSources.value[0]?.Ref ?? ''
    liveSourceUrl.value = liveSources.value[0]?.Ref ?? ''
  } catch (e) { errMsg.value = String(e) }
}

async function importVodSource() {
  const ref = vodSourceUrl.value.trim()
  if (!ref) return
  errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.ImportVodSource(ref)
    importSummary.value = `点播源导入成功：${r.Sites} 个站点`
    await reloadSourceHistory()
    activeSite.value = ''
    await refreshVod()
  } catch (e) { errMsg.value = String(e) }
}

async function importLiveSource() {
  const ref = liveSourceUrl.value.trim()
  if (!ref) return
  errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.ImportLiveSource(ref)
    importSummary.value = r.Channels > 0 ? `直播源导入成功：${r.Channels} 频道` : `直播源导入成功：${r.LiveSources} 个源`
    await reloadSourceHistory()
  } catch (e) { errMsg.value = String(e) }
}

async function reimportSource(kind: string, ref: string) {
  if (kind === 'vod') { vodSourceUrl.value = ref; await importVodSource() }
  else { liveSourceUrl.value = ref; await importLiveSource() }
}

async function deleteSource(kind: string, ref: string) {
  try {
    await ShellService.DeleteSource(kind, ref)
    await reloadSourceHistory()
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

async function loadLive() {
  errMsg.value = ''; importProgress.value = null
  try {
    const n = await ShellService.LoadLive()
    if (n === 0) importSummary.value = '没有可用的直播频道'
    await reloadGroups()
  } catch (e) { errMsg.value = String(e) }
}

async function play(c: ChannelInfo) {
  errMsg.value = ''
  currentVod.value = null
  try {
    playbackPlan.value = await ShellService.PrepareChannel(c.ID) as unknown as PlaybackPlan
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

async function refreshVod() {
  await loadSources()
  if (activeSite.value) await reloadVodCategories()
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
    const d = await ShellService.VodDetail(activeSite.value, item.ID)
    d.Description = DOMPurify.sanitize(d.Description)
    vodDetail.value = d
  } catch (e) { errMsg.value = String(e) }
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  try {
    playbackPlan.value = await ShellService.PrepareVod(activeSite.value, ep.ID) as unknown as PlaybackPlan
    nowPlaying.value = ep.Name
    if (vodDetail.value) {
      currentVod.value = { site: activeSite.value, vodID: vodDetail.value.ID }
      await ShellService.RecordVodHistory(activeSite.value, vodDetail.value.ID, vodDetail.value.Title, vodDetail.value.Logo, ep.ID, ep.Name, ep.Source)
    }
  } catch (e) { errMsg.value = String(e) }
}

async function fallbackToMpv(id: string) {
  try { playbackPlan.value = await ShellService.FallbackToMPV(id) as unknown as PlaybackPlan }
  catch (e) { errMsg.value = String(e) }
}

async function onProgress(time: number, duration: number) {
  if (!currentVod.value) return
  const now = Date.now()
  if (now - lastProgressSave < 10000) return
  lastProgressSave = now
  try { await ShellService.UpdateVodProgress(currentVod.value.site, currentVod.value.vodID, time, duration) }
  catch { /* 进度保存失败不阻断 */ }
}

// imgError 隐藏加载失败的图片（部分源 vod_pic 为空/失效/被防盗链拦截）。
function imgError(e: Event) {
  ;(e.target as HTMLImageElement).style.display = 'none'
}

onMounted(() => {
  refresh()
  Events.On('import:progress', (ev: any) => { importProgress.value = ev.data as Progress })
})
</script>

<template>
  <main class="container">
    <header>
      <h1 class="title">Unbox</h1>
      <p class="subtitle">{{ platform }} · 播放器{{ playerReady ? '就绪' : '未就绪' }}</p>
    </header>

    <div v-if="!mpvReady" class="mpv-install">
      <span>mpv 插件未安装（HEVC / RTMP / 本地文件需要它）</span>
      <button @click="installMpv">{{ mpvInstallMode === 'download' ? '下载并安装 mpv' : '显示安装命令' }}</button>
      <button v-if="mpvInstallMode && mpvInstallMode !== 'download'" @click="recheckMpv">我已安装，重新检测</button>
      <p v-if="installMessage" class="install-cmd">{{ installMessage }}</p>
    </div>

    <nav class="tabs">
      <button :class="{ active: mode === 'home' }" @click="switchMode('home')">首页</button>
      <button :class="{ active: mode === 'vod' }" @click="switchMode('vod')">点播</button>
      <button :class="{ active: mode === 'live' }" @click="switchMode('live')">直播</button>
      <button :class="{ active: mode === 'settings' }" @click="switchMode('settings')">设置</button>
    </nav>

    <!-- 首页：观看历史 -->
    <section v-if="mode === 'home'" class="home">
      <h2>观看记录</h2>
      <p v-if="!homeHistory.length" class="home-empty">暂无观看记录，去「点播」看看吧</p>
      <ul v-else class="home-list">
        <li v-for="h in homeHistory" :key="h.Site + h.VodID" @click="resumeVod(h)">
          <img v-if="h.VodLogo" :src="h.VodLogo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
          <span class="home-info">
            <span class="name">{{ h.VodTitle }}</span>
            <span class="sub">{{ h.EpName }}{{ fmtProgress(h.Progress) ? ' · 看到 ' + fmtProgress(h.Progress) : '' }}</span>
          </span>
        </li>
      </ul>
    </section>

    <!-- 直播 -->
    <section v-if="mode === 'live'" class="layout">
      <aside class="groups">
        <button v-for="g in groups" :key="g" :class="{ active: g === activeGroup }" @click="activeGroup = g; reloadChannels()">
          {{ g === '*' ? '全部' : g }}
        </button>
        <hr />
        <button @click="loadFavorites">⭐ 收藏</button>
      </aside>

      <aside class="player">
        <p v-if="nowPlaying" class="now">正在播放：{{ nowPlaying }}</p>
        <PlaybackView :plan="playbackPlan" @fallback="fallbackToMpv" @progress="onProgress" />
        <div class="controls" v-if="nowPlaying && playbackPlan?.Backend === 'mpv'">
          <button @click="pause">暂停</button>
          <button @click="resume">继续</button>
          <input type="range" min="0" max="100" @input="setVolume" />
        </div>
        <p v-if="favorites.length" class="favhead">收藏</p>
        <ul class="favs">
          <li v-for="f in favorites" :key="f.ID" @click="play(f)">{{ f.Name }}</li>
        </ul>
      </aside>

      <section class="channels">
        <div v-if="groups.length <= 1" class="load-live">
          <p>直播源尚未加载</p>
          <button @click="loadLive">加载直播</button>
        </div>
        <div class="search"><input v-model="query" placeholder="搜索频道" @keyup.enter="doSearch" /><button @click="doSearch">搜索</button></div>
        <ul>
          <li v-for="c in channels" :key="c.ID" class="channel">
            <span class="name">{{ c.Name }}</span>
            <span class="group">{{ c.Group }}</span>
            <button @click="play(c)">▶ 播放</button>
            <button @click="toggleFav(c)">{{ c.Favorited ? '★' : '☆' }}</button>
          </li>
        </ul>
      </section>
    </section>

    <!-- 点播 -->
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
          <div class="search"><input v-model="vodQuery" placeholder="搜索影片" @keyup.enter="vodSearch" /><button @click="vodSearch">搜索</button></div>
          <ul v-if="!vodDetail">
            <li v-for="it in vodItems" :key="it.ID" class="channel" @click="openVodDetail(it)">
              <img v-if="it.Logo" :src="it.Logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
              <span class="name">{{ it.Title }}</span><span class="group">{{ it.Group }}</span>
            </li>
          </ul>
          <div v-else class="vod-detail">
            <button @click="vodDetail = null">← 返回</button>
            <div class="vod-detail-top">
              <div class="vod-player">
                <p v-if="nowPlaying" class="now">正在播放：{{ nowPlaying }}</p>
                <PlaybackView :plan="playbackPlan" @fallback="fallbackToMpv" @progress="onProgress" />
                <div class="controls" v-if="nowPlaying && playbackPlan?.Backend === 'mpv'">
                  <button @click="pause">暂停</button>
                  <button @click="resume">继续</button>
                  <input type="range" min="0" max="100" @input="setVolume" />
                </div>
              </div>
              <div class="vod-info">
                <img v-if="vodDetail.Logo" :src="vodDetail.Logo" class="poster" referrerpolicy="no-referrer" @error="imgError" />
                <h2>{{ vodDetail.Title }}</h2>
                <p class="meta">{{ vodDetail.Type }} · {{ vodDetail.Year }} · {{ vodDetail.Area }}</p>
                <div class="desc" v-html="vodDetail.Description"></div>
              </div>
            </div>
            <div v-for="src in (vodDetail.Sources ?? [])" :key="src" class="ep-src">
              <p class="ep-src-name">{{ src }}</p>
              <button v-for="ep in (vodDetail.Episodes ?? []).filter(e => e.Source === src)" :key="ep.ID"
                      @click="playEpisode(ep)">{{ ep.Name }}</button>
            </div>
          </div>
        </section>
      </div>
    </section>

    <!-- 设置 -->
    <section v-if="mode === 'settings'" class="settings-page">
      <p v-if="importProgress" class="progress">{{ importProgress.Message }}</p>
      <p v-if="importSummary" class="ok">{{ importSummary }}</p>

      <section class="src-section">
        <h3>点播源</h3>
        <div class="src-add">
          <input v-model="vodSourceUrl" placeholder="粘贴点播源地址" @keyup.enter="importVodSource" />
          <button @click="importVodSource">导入</button>
        </div>
        <ul v-if="vodSources.length" class="src-history">
          <li v-for="s in vodSources" :key="'vod-' + s.Ref">
            <span class="src-ref" :class="{ current: s.Ref === vodSources[0]?.Ref }" @click="reimportSource('vod', s.Ref)">{{ s.Ref }}</span>
            <button class="src-del" @click="deleteSource('vod', s.Ref)">删除</button>
          </li>
        </ul>
      </section>

      <section class="src-section">
        <h3>直播源</h3>
        <div class="src-add">
          <input v-model="liveSourceUrl" placeholder="粘贴直播源地址（M3U/TXT/订阅）" @keyup.enter="importLiveSource" />
          <button @click="importLiveSource">导入</button>
        </div>
        <ul v-if="liveSources.length" class="src-history">
          <li v-for="s in liveSources" :key="'live-' + s.Ref">
            <span class="src-ref" :class="{ current: s.Ref === liveSources[0]?.Ref }" @click="reimportSource('live', s.Ref)">{{ s.Ref }}</span>
            <button class="src-del" @click="deleteSource('live', s.Ref)">删除</button>
          </li>
        </ul>
      </section>
    </section>

    <p v-if="errMsg" class="error">{{ errMsg }}</p>
  </main>
</template>
