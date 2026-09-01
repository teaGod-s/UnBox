<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Events, Browser } from '@wailsio/runtime'
import { ShellService, type SourceInfo, type Section, type VodItem, type EpisodeInfo, type VodMedia, type SourceRecord, type VodHistoryInfo, type UpdateInfo } from '../bindings/github.com/unbox/unbox/internal/shell'
import PlaybackView, { type PlaybackPlan } from './components/PlaybackView.vue'
import { clampEpisodePage, episodePageIndex, episodePageRanges, paginateEpisodes } from './episodes'
import { playbackPlanForMode } from './playbackScope'
import DOMPurify from 'dompurify'

// 爱发电赞助主页
const DONATE_URL = 'https://afdian.com/a/teaGod'

interface ChannelInfo { ID: string; Name: string; Group: string; Logo: string; Favorited: boolean }
interface Progress { Stage: string; Message: string; Done: number; Total: number }

const platform = ref('…')
const playerReady = ref(false)
const groups = ref<string[]>([])
const channels = ref<ChannelInfo[]>([])
const favorites = ref<ChannelInfo[]>([])
const activeGroup = ref('*')
const query = ref('')
const liveNowPlaying = ref('')
const vodNowPlaying = ref('')
const importSummary = ref('')
const errMsg = ref('')
const importProgress = ref<Progress | null>(null)
const mode = ref<'home' | 'vod' | 'live' | 'settings'>('home')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const activeLine = ref('')
const activeSource = ref('')
const detailSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
const vodItems = ref<VodItem[]>([])
const vodDetail = ref<VodMedia | null>(null)
const episodePage = ref(0)
const currentEpisodeID = ref('')
const episodePagination = ref<HTMLElement | null>(null)
const vodQuery = ref('')
const vodPage = ref(0)
const livePlaybackPlan = ref<PlaybackPlan | null>(null)
const vodPlaybackPlan = ref<PlaybackPlan | null>(null)
const mpvReady = ref(false)
const mpvInstallMode = ref('')
const installMessage = ref('')
const vodSources = ref<SourceRecord[]>([])
const liveSources = ref<SourceRecord[]>([])
const vodSourceUrl = ref('')
const liveSourceUrl = ref('')
const showVodHistory = ref(false)
const showLiveHistory = ref(false)
const homeHistory = ref<VodHistoryInfo[]>([])
const currentVod = ref<{ site: string; vodID: string } | null>(null)
const pendingSeek = ref(0)
const logs = ref('')
const showLogs = ref(false)
const copyMsg = ref('')
const searchProgress = ref<Progress | null>(null)
const searchThreads = ref(1)
const showThreads = ref(false)
const searching = ref(false)
const updateInfo = ref<UpdateInfo | null>(null)
const updateMsg = ref('')
const currentVersion = ref('')
const internalVersion = ref('')
const showAbout = ref(false)
const showDisclaimer = ref(false)
const showOpenSource = ref(false)
const catsCollapsed = ref(false)
const infoCollapsed = ref(false)
let lastProgressSave = 0

const activeEpisodes = computed(() => (vodDetail.value?.Episodes ?? []).filter(ep => ep.Source === activeSource.value))
const episodePages = computed(() => paginateEpisodes(activeEpisodes.value))
const episodeRanges = computed(() => episodePageRanges(activeEpisodes.value.length))
const visibleEpisodes = computed(() => episodePages.value[episodePage.value] ?? [])
const activePlaybackPlan = computed(() => playbackPlanForMode(mode.value, {
  live: livePlaybackPlan.value,
  vod: vodPlaybackPlan.value,
}))

function resetEpisodePage() {
  episodePage.value = 0
  currentEpisodeID.value = ''
}

function selectEpisodePage(page: number) {
  episodePage.value = clampEpisodePage(page, activeEpisodes.value.length)
}

function selectEpisodeSource(source: string) {
  activeSource.value = source
  resetEpisodePage()
}

function scrollEpisodePages(direction: number) {
  const element = episodePagination.value
  if (!element) return
  element.scrollBy({ left: direction * Math.max(element.clientWidth * 0.75, 160), behavior: 'smooth' })
}

const vodSites = computed(() => sources.value.filter(s => s.Kind === 'vod'))
const vodLines = computed(() => {
  const names: string[] = []
  for (const s of vodSites.value) {
    const ln = s.Line || ''
    if (!names.includes(ln)) names.push(ln)
  }
  return names
})
function sitesOfLine(line: string) {
  return vodSites.value.filter(s => (s.Line || '') === line)
}
function siteName(site: string) {
  if (!site) return ''
  return sources.value.find(s => s.ID === site)?.Name ?? site
}
function vodItemSub(it: VodItem) {
  const parts = [it.Site ? siteName(it.Site) : '', it.Group].filter(Boolean)
  return parts.join(' · ')
}

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
    currentVersion.value = await ShellService.CurrentVersion()
    internalVersion.value = await ShellService.InternalVersion()
    await refreshMpvStatus()
    await refreshHome()
  } catch (e) { handleError(e) }
}

// handleError 统一处理前端错误：回显 + 写入后端日志（使 RuntimeError 进入「查看日志」）。
function handleError(e: unknown) {
  const msg = String(e)
  errMsg.value = msg
  ShellService.LogError(msg).catch(() => {})
}

async function openAbout() {
  try { internalVersion.value = await ShellService.InternalVersion() } catch { /* 忽略 */ }
  showAbout.value = true
}

// openURL 用系统默认浏览器打开外部链接（源码 / 捐助 / 下载新版本）。
function openURL(url: string) {
  Browser.OpenURL(url).catch(() => {})
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
  } catch (e) { handleError(e) }
}

async function recheckMpv() {
  try {
    const s = await ShellService.RefreshMPV()
    mpvReady.value = s.Available
    if (s.Available) installMessage.value = ''
  } catch (e) { handleError(e) }
}

async function switchMode(m: 'home' | 'vod' | 'live' | 'settings') {
  mode.value = m
  if (m === 'vod') await refreshVod()
  else if (m === 'live') await reloadGroups()
  else if (m === 'home') await refreshHome()
  else if (m === 'settings') { await reloadSourceHistory(); await refreshLogs(); await loadSearchThreads() }
}

async function refreshHome() {
  try {
    homeHistory.value = (await ShellService.ListVodHistory()) ?? []
  } catch (e) { handleError(e) }
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
    detailSite.value = h.Site
    vodDetail.value = await ShellService.VodDetail(h.Site, h.VodID)
    const resumeSource = h.Source && vodDetail.value.Sources?.includes(h.Source)
      ? h.Source
      : vodDetail.value.Sources?.[0] ?? ''
    activeSource.value = resumeSource
    resetEpisodePage()
    if (h.EpID) {
      pendingSeek.value = h.Progress
      await doPlayEpisode(h.Site, h.EpID, h.EpName, resumeSource)
      episodePage.value = episodePageIndex(activeEpisodes.value, h.EpID)
      if (h.Progress > 0 && vodPlaybackPlan.value?.Backend === 'mpv') {
        await ShellService.Seek(h.Progress)
      }
    }
  } catch (e) { handleError(e) }
}

// ---- 设置：源管理 ----
async function reloadSourceHistory() {
  try {
    const all = (await ShellService.ListSources()) ?? []
    vodSources.value = all.filter(s => s.Kind === 'vod')
    liveSources.value = all.filter(s => s.Kind === 'live')
    vodSourceUrl.value = vodSources.value[0]?.Ref ?? ''
    liveSourceUrl.value = liveSources.value[0]?.Ref ?? ''
  } catch (e) { handleError(e) }
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
  } catch (e) { handleError(e) }
}

async function importLiveSource() {
  const ref = liveSourceUrl.value.trim()
  if (!ref) return
  errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.ImportLiveSource(ref)
    importSummary.value = r.Channels > 0 ? `直播源导入成功：${r.Channels} 频道` : `直播源导入成功：${r.LiveSources} 个源`
    await reloadSourceHistory()
  } catch (e) { handleError(e) }
}

async function reimportSource(kind: string, ref: string) {
  if (kind === 'vod') { vodSourceUrl.value = ref; await importVodSource() }
  else { liveSourceUrl.value = ref; await importLiveSource() }
}

async function deleteSource(kind: string, ref: string) {
  try {
    await ShellService.DeleteSource(kind, ref)
    await reloadSourceHistory()
  } catch (e) { handleError(e) }
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
  } catch (e) { handleError(e) }
}

async function play(c: ChannelInfo) {
  errMsg.value = ''
  currentVod.value = null
  try {
    livePlaybackPlan.value = await ShellService.PrepareChannel(c.ID) as unknown as PlaybackPlan
    liveNowPlaying.value = c.Name
  } catch (e) { handleError(e) }
}

async function toggleFav(c: ChannelInfo) {
  try {
    if (c.Favorited) await ShellService.RemoveFavorite(c.ID)
    else await ShellService.AddFavorite(c.ID)
    c.Favorited = !c.Favorited
    favorites.value = (await ShellService.ListFavorites()) ?? []
  } catch (e) { handleError(e) }
}

async function loadFavorites() { favorites.value = (await ShellService.ListFavorites()) ?? [] }

async function pause() { await ShellService.Pause() }
async function resume() { await ShellService.Resume() }
async function setVolume(e: Event) { await ShellService.SetVolume(Number((e.target as HTMLInputElement).value)) }

async function loadSources() {
  sources.value = (await ShellService.Sources()) ?? []
  if (!activeSite.value) {
    const last = await ShellService.LastVodSite()
    const target = vodSites.value.find(s => s.ID === last) ?? vodSites.value[0]
    if (target) {
      activeSite.value = target.ID
      activeLine.value = target.Line ?? ''
    }
  }
}

async function refreshVod() {
  await loadSources()
  if (activeSite.value) await reloadVodCategories()
}

async function selectLine(line: string) {
  activeLine.value = line
  const first = sitesOfLine(line)[0]
  if (first) {
    activeSite.value = first.ID
    vodDetail.value = null
    vodPage.value = 0
    await reloadVodCategories()
    await ShellService.SetLastVodSite(first.ID)
  }
}

async function selectSite(id: string) {
  activeSite.value = id
  vodDetail.value = null
  vodPage.value = 0
  await reloadVodCategories()
  await ShellService.SetLastVodSite(id)
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
  searching.value = true
  vodItems.value = []
  try {
    vodItems.value = (await ShellService.VodSearchAll(vodQuery.value)) ?? []
  } catch (e) { handleError(e) }
  finally { searching.value = false }
}

async function cancelSearch() {
  searching.value = false
  try { await ShellService.CancelSearch() } catch { /* 忽略 */ }
}

async function openVodDetail(item: VodItem) {
  errMsg.value = ''
  try {
    const site = item.Site || activeSite.value
    detailSite.value = site
    const d = await ShellService.VodDetail(site, item.ID)
    d.Description = DOMPurify.sanitize(d.Description)
    vodDetail.value = d
    activeSource.value = d.Sources?.[0] ?? ''
    resetEpisodePage()
  } catch (e) { handleError(e) }
}

async function doPlayEpisode(site: string, epID: string, epName: string, source: string) {
  vodPlaybackPlan.value = await ShellService.PrepareVod(site, epID) as unknown as PlaybackPlan
  vodNowPlaying.value = epName
  currentEpisodeID.value = epID
  if (vodDetail.value) {
    currentVod.value = { site, vodID: vodDetail.value.ID }
    await ShellService.RecordVodHistory(site, vodDetail.value.ID, vodDetail.value.Title, vodDetail.value.Logo, epID, epName, source)
  }
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  pendingSeek.value = 0
  try {
    await doPlayEpisode(detailSite.value || activeSite.value, ep.ID, ep.Name, ep.Source)
  } catch (e) { handleError(e) }
}

async function fallbackToMpv(id: string) {
  try {
    const plan = await ShellService.FallbackToMPV(id) as unknown as PlaybackPlan
    if (mode.value === 'live') livePlaybackPlan.value = plan
    else if (mode.value === 'vod') vodPlaybackPlan.value = plan
  }
  catch (e) { handleError(e) }
}

async function onProgress(time: number, duration: number) {
  if (!currentVod.value) return
  const now = Date.now()
  if (now - lastProgressSave < 10000) return
  lastProgressSave = now
  try { await ShellService.UpdateVodProgress(currentVod.value.site, currentVod.value.vodID, time, duration) }
  catch { /* 进度保存失败不阻断 */ }
}

async function refreshLogs() {
  try { logs.value = await ShellService.GetLogs() } catch (e) { handleError(e) }
}

async function openLogs() {
  copyMsg.value = ''
  await refreshLogs()
  showLogs.value = true
}

async function copyLogs() {
  const text = logs.value
  if (!text) return
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    copyMsg.value = '已复制到剪贴板'
  } catch (e) {
    errMsg.value = '复制失败：' + String(e)
  }
}

async function pollMpvProgress() {
  if (!currentVod.value || activePlaybackPlan.value?.Backend !== 'mpv') return
  try {
    const pos = await ShellService.Position()
    await ShellService.UpdateVodProgress(currentVod.value.site, currentVod.value.vodID, pos, 0)
  } catch { /* 忽略 */ }
}

async function loadSearchThreads() {
  try { searchThreads.value = await ShellService.SearchThreads() } catch (e) { handleError(e) }
}

function openThreads() {
  showThreads.value = true
}

async function chooseThreads(n: number) {
  searchThreads.value = n
  try { await ShellService.SetSearchThreads(n) } catch (e) { handleError(e) }
  showThreads.value = false
}

async function checkUpdate() {
  updateMsg.value = '检查中…'
  try {
    updateInfo.value = await ShellService.CheckUpdate()
    updateMsg.value = updateInfo.value.HasUpdate ? `发现新版本 ${updateInfo.value.LatestVersion}` : '已是最新版本'
  } catch (e) {
    updateMsg.value = '检查更新失败'
    handleError(e)
  }
}

// imgError 隐藏加载失败的图片（部分源 vod_pic 为空/失效/被防盗链拦截）。
function imgError(e: Event) {
  ;(e.target as HTMLImageElement).style.display = 'none'
}

onMounted(() => {
  refresh()
  Events.On('import:progress', (ev: any) => { importProgress.value = ev.data as Progress })
  Events.On('search:progress', (ev: any) => { searchProgress.value = ev.data as Progress })
  Events.On('search:result', (ev: any) => {
    if (searching.value) {
      vodItems.value = [...vodItems.value, ...(ev.data as VodItem[])]
    }
  })
  setInterval(pollMpvProgress, 10000)
})

</script>

<template>
  <main class="container">
    <header>
      <h1 class="title">UnBox</h1>
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
      <button class="settings-btn" :class="{ active: mode === 'settings' }" @click="switchMode('settings')">设置</button>
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
            <span class="sub">{{ h.SiteName || h.Site }} · {{ h.EpName }}{{ fmtProgress(h.Progress) ? ' · 看到 ' + fmtProgress(h.Progress) : '' }}</span>
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
        <p v-if="liveNowPlaying" class="now">正在播放：{{ liveNowPlaying }}</p>
        <PlaybackView :plan="livePlaybackPlan" @fallback="fallbackToMpv" />
        <div class="controls" v-if="liveNowPlaying && livePlaybackPlan?.Backend === 'mpv'">
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
          <span v-if="importProgress && importProgress.Stage === 'live'" class="progress">{{ importProgress.Message }}</span>
        </div>
        <form class="search" @submit.prevent="doSearch"><input v-model="query" placeholder="搜索频道" /><button type="submit">搜索</button></form>
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
        <select v-if="vodLines.length > 1" v-model="activeLine" @change="selectLine(activeLine)">
          <option v-for="l in vodLines" :key="l" :value="l">{{ l || '默认线路' }}</option>
        </select>
        <select v-model="activeSite" @change="selectSite(activeSite)">
          <option v-for="s in sitesOfLine(activeLine)" :key="s.ID" :value="s.ID">{{ s.Name }}</option>
        </select>
      </div>

      <div class="vod-layout" :class="{ 'cats-collapsed': catsCollapsed }">
        <aside class="vod-cats" :class="{ collapsed: catsCollapsed }">
          <div class="cats-head">
            <span class="cats-title">类目</span>
            <button class="cats-toggle" :title="catsCollapsed ? '展开类目' : '收起类目'" @click="catsCollapsed = !catsCollapsed">{{ catsCollapsed ? '»' : '«' }}</button>
          </div>
          <div class="cats-list">
            <button v-for="c in vodCategories" :key="c.ID" :class="{ active: c.ID === vodActiveCat }"
                    @click="vodActiveCat = c.ID; vodPage = 0; reloadVodList()">{{ c.Title }}</button>
          </div>
        </aside>

        <section class="vod-main">
          <div class="vod-search-row">
            <button v-if="vodDetail" class="vod-back" type="button" @click="vodDetail = null">← 返回</button>
            <form class="search" @submit.prevent="vodSearch"><input v-model="vodQuery" placeholder="搜索影片" /><button type="submit">搜索</button><button v-if="searching" type="button" @click="cancelSearch">取消</button><span v-if="searchProgress" class="progress">{{ searchProgress.Message }}</span></form>
          </div>
          <ul v-if="!vodDetail">
            <li v-for="it in vodItems" :key="it.ID" class="channel" @click="openVodDetail(it)">
              <img v-if="it.Logo" :src="it.Logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
              <span class="name">{{ it.Title }}</span><span class="group">{{ vodItemSub(it) }}</span>
            </li>
          </ul>
          <div v-else class="vod-detail">
            <div class="vod-detail-top" :class="{ 'info-collapsed': infoCollapsed }">
              <div class="vod-player">
                <p v-if="vodNowPlaying" class="now">正在播放：{{ vodNowPlaying }}</p>
                <PlaybackView :plan="vodPlaybackPlan" :seek-to="pendingSeek" @fallback="fallbackToMpv" @progress="onProgress" />
                <div class="controls" v-if="vodNowPlaying && vodPlaybackPlan?.Backend === 'mpv'">
                  <button @click="pause">暂停</button>
                  <button @click="resume">继续</button>
                  <input type="range" min="0" max="100" @input="setVolume" />
                </div>
                <div v-if="(vodDetail.Sources ?? []).length" class="ep-src-tabs">
                  <button v-for="src in vodDetail.Sources" :key="src" :class="{ active: src === activeSource }" @click="selectEpisodeSource(src)">{{ src }}</button>
                </div>
                <div v-if="episodeRanges.length > 1" class="ep-pagination-wrap" role="navigation" aria-label="剧集分页">
                  <button class="ep-pagination-arrow" type="button" aria-label="向左滚动剧集分页" title="向左滚动" @click="scrollEpisodePages(-1)">‹</button>
                  <div ref="episodePagination" class="ep-pagination">
                    <button v-for="(range, index) in episodeRanges" :key="range.start" type="button"
                            :class="{ active: index === episodePage }" :aria-current="index === episodePage ? 'page' : undefined"
                            @click="selectEpisodePage(index)">{{ range.start }}~{{ range.end }}集</button>
                  </div>
                  <button class="ep-pagination-arrow" type="button" aria-label="向右滚动剧集分页" title="向右滚动" @click="scrollEpisodePages(1)">›</button>
                </div>
                <div class="ep-list">
                  <button v-for="ep in visibleEpisodes" :key="ep.ID" :class="{ active: ep.ID === currentEpisodeID }"
                          @click="playEpisode(ep)">{{ ep.Name }}</button>
                </div>
              </div>
              <div class="vod-info" :class="{ collapsed: infoCollapsed }">
                <button class="info-toggle" :title="infoCollapsed ? '展开详情' : '收起详情'" @click="infoCollapsed = !infoCollapsed">{{ infoCollapsed ? '«' : '»' }}</button>
                <div class="info-body">
                  <img v-if="vodDetail.Logo" :src="vodDetail.Logo" class="poster" referrerpolicy="no-referrer" @error="imgError" />
                  <h2>{{ vodDetail.Title }}</h2>
                  <p class="meta">{{ vodDetail.Type }} · {{ vodDetail.Year }} · {{ vodDetail.Area }}</p>
                  <div class="desc" v-html="vodDetail.Description"></div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </section>

    <!-- 设置 -->
    <section v-if="mode === 'settings'" class="settings-page">
      <p v-if="importProgress && importProgress.Stage !== 'live'" class="progress">{{ importProgress.Message }}</p>
      <p v-if="importSummary" class="ok">{{ importSummary }}</p>

      <section class="src-section">
        <h3>点播源</h3>
        <form class="src-add" @submit.prevent="importVodSource">
          <input v-model="vodSourceUrl" placeholder="粘贴点播源地址" />
          <button type="submit">导入</button>
          <button type="button" @click="showVodHistory = !showVodHistory">{{ showVodHistory ? '收起历史' : '历史配置' }}</button>
        </form>
        <ul v-if="showVodHistory && vodSources.length" class="src-history">
          <li v-for="s in vodSources" :key="'vod-' + s.Ref">
            <span class="src-ref" :class="{ current: s.Ref === vodSources[0]?.Ref }" @click="reimportSource('vod', s.Ref)">{{ s.Ref }}</span>
            <button class="src-del" @click="deleteSource('vod', s.Ref)">删除</button>
          </li>
        </ul>
      </section>

      <section class="src-section">
        <h3>直播源</h3>
        <form class="src-add" @submit.prevent="importLiveSource">
          <input v-model="liveSourceUrl" placeholder="粘贴直播源地址（M3U/TXT/订阅）" />
          <button type="submit">导入</button>
          <button type="button" @click="showLiveHistory = !showLiveHistory">{{ showLiveHistory ? '收起历史' : '历史配置' }}</button>
        </form>
        <ul v-if="showLiveHistory && liveSources.length" class="src-history">
          <li v-for="s in liveSources" :key="'live-' + s.Ref">
            <span class="src-ref" :class="{ current: s.Ref === liveSources[0]?.Ref }" @click="reimportSource('live', s.Ref)">{{ s.Ref }}</span>
            <button class="src-del" @click="deleteSource('live', s.Ref)">删除</button>
          </li>
        </ul>
      </section>

      <section class="src-section">
        <h3>搜索</h3>
        <div class="src-add">
          <button @click="openThreads">搜索线程：{{ searchThreads }} 个</button>
        </div>
      </section>

      <section class="src-section">
        <h3>关于</h3>
        <div class="src-add about-actions">
          <button @click="checkUpdate">检查更新</button>
          <button @click="openAbout">关于我们</button>
          <button @click="showDisclaimer = true">免责条款</button>
          <button @click="showOpenSource = true">开源库</button>
          <button @click="openURL('https://github.com/teaGod-s/UnBox')">源码</button>
          <button @click="openURL(DONATE_URL)">捐助</button>
        </div>
        <p v-if="updateMsg" class="home-empty">{{ updateMsg }}</p>
        <div v-if="updateInfo?.HasUpdate && updateInfo.URL" class="src-add">
          <button @click="openURL(updateInfo.URL)">点击下载新版本</button>
        </div>
      </section>

      <section class="src-section">
        <h3>日志</h3>
        <div class="src-add">
          <button @click="openLogs">查看日志</button>
        </div>
      </section>
    </section>

    <div v-if="showThreads" class="settings-overlay" @click.self="showThreads = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>搜索线程</h2>
          <button @click="showThreads = false">✕</button>
        </div>
        <p class="threads-hint">全站搜索时同时请求的站点数，站点多时调大可加速</p>
        <div class="threads-options">
          <button v-for="n in [1, 4, 8, 16]" :key="n" :class="{ active: searchThreads === n }" @click="chooseThreads(n)">{{ n }}{{ n === 1 ? '（默认）' : '' }}</button>
        </div>
      </div>
    </div>

    <div v-if="showLogs" class="settings-overlay" @click.self="showLogs = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>日志</h2>
          <button @click="showLogs = false">✕</button>
        </div>
        <div class="log-toolbar">
          <button @click="refreshLogs">刷新</button>
          <button @click="copyLogs">复制</button>
          <span v-if="copyMsg" class="ok">{{ copyMsg }}</span>
        </div>
        <pre v-if="logs" class="log-view">{{ logs }}</pre>
        <p v-else class="home-empty">暂无日志</p>
      </div>
    </div>

    <div v-if="showAbout" class="settings-overlay" @click.self="showAbout = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>关于 UnBox</h2>
          <button @click="showAbout = false">✕</button>
        </div>
        <div class="about-body">
          <img class="about-logo" src="/appicon.png" alt="UnBox logo" />
          <p>UnBox 是一个跨平台（Windows / macOS / Linux）的 TVBox 兼容桌面播放器，支持 IPTV 直播与视频点播，一个安装包装好即用。</p>
          <p class="about-ver">当前版本：{{ currentVersion }}</p>
          <p class="about-ver">内部版本：{{ internalVersion }}</p>
          <p>技术栈：Go 1.26 + Wails v3 + Vue 3 + TypeScript。完全开源，源码见「源码」按钮。</p>
        </div>
      </div>
    </div>

    <div v-if="showDisclaimer" class="settings-overlay" @click.self="showDisclaimer = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>免责条款</h2>
          <button @click="showDisclaimer = false">✕</button>
        </div>
        <div class="disclaimer">
          <p>UnBox 是一个开源的「空壳」播放器，本身不提供、不存储、不制作任何影视内容，也不内置任何内容源。</p>
          <p>本软件仅为用户提供技术性的播放能力。用户自行导入的内容源（订阅、M3U、TVBox 源等）及其所指向的资源，均由第三方提供，与本软件无关。</p>
          <p>请遵守当地法律法规，仅将本软件用于访问您拥有合法权利或已获授权的内容。用户因使用本软件而产生的任何法律后果由用户自行承担，本软件及作者不承担任何责任。</p>
        </div>
      </div>
    </div>

    <div v-if="showOpenSource" class="settings-overlay" @click.self="showOpenSource = false">
      <div class="settings-panel">
        <div class="settings-head">
          <h2>开源库</h2>
          <button @click="showOpenSource = false">✕</button>
        </div>
        <ul class="oss-list">
          <li><a href="https://github.com/wailsapp/wails" target="_blank" rel="noopener">Wails v3</a><span class="oss-lic">MIT</span> — 桌面应用框架</li>
          <li><a href="https://gitlab.com/cznic/sqlite" target="_blank" rel="noopener">modernc.org/sqlite</a><span class="oss-lic">BSD-3</span> — 纯 Go SQLite</li>
          <li><a href="https://github.com/vuejs/core" target="_blank" rel="noopener">Vue 3</a><span class="oss-lic">MIT</span> — 前端框架</li>
          <li><a href="https://github.com/video-dev/hls.js" target="_blank" rel="noopener">hls.js</a><span class="oss-lic">Apache-2.0</span> — HLS 播放</li>
          <li><a href="https://github.com/xqq/mpegts.js" target="_blank" rel="noopener">mpegts.js</a><span class="oss-lic">MIT</span> — MPEG-TS / FLV 播放</li>
          <li><a href="https://github.com/cure53/DOMPurify" target="_blank" rel="noopener">DOMPurify</a><span class="oss-lic">Apache-2.0</span> — HTML 清洗</li>
          <li><a href="https://github.com/vitejs/vite" target="_blank" rel="noopener">Vite</a><span class="oss-lic">MIT</span> — 构建工具</li>
          <li><a href="https://github.com/microsoft/TypeScript" target="_blank" rel="noopener">TypeScript</a><span class="oss-lic">Apache-2.0</span> — 类型系统</li>
        </ul>
      </div>
    </div>

    <div v-if="errMsg" class="error-box">
      <span class="error-text">{{ errMsg }}</span>
      <button class="error-close" title="关闭" @click="errMsg = ''">✕</button>
    </div>
  </main>
</template>
