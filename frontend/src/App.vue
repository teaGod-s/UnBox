<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Events, Browser } from '@wailsio/runtime'
import { ShellService, type SourceInfo, type Section, type VodItem, type EpisodeInfo, type VodMedia, type SourceRecord, type VodHistoryInfo, type VodFavoriteInfo, type UpdateInfo } from '../bindings/github.com/unbox/unbox/internal/shell'
import PlaybackView, { type PlaybackPlan } from './components/PlaybackView.vue'
import VodDetailHeader from './components/VodDetailHeader.vue'
import { clampEpisodePage, episodePageIndex, episodePageRanges, paginateEpisodes } from './episodes'
import { playbackPlanForMode, resolvePlaybackFallback, shouldPauseStalePlayback, shouldRecordVodProgress, shouldShowMpvInstallPrompt, type ActivePlaybackSession, type PlaybackScope, type PlaybackStatus } from './playbackScope'
import { createVodSearchCache, isCurrentVodCategoryRequest, isVodSearchCacheValid, nextVodCategoryRequest, nextVodSearchRequest, removeVodFavorite, removeVodHistory, removeVodSearchHistory, resolveVodSelection, shouldShowVodNoResults, upsertVodSearchHistory, vodBackTarget, vodSearchQueryForReturn, type VodDetailOrigin, type VodSearchCache, type VodView } from './vodNavigation'
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
const mode = ref<'home' | 'vod' | 'live' | 'search' | 'favorites' | 'settings'>('home')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const activeLine = ref('')
const activeSource = ref('')
const detailSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
const vodListItems = ref<VodItem[]>([])
const vodSearchItems = ref<VodItem[]>([])
const vodDetail = ref<VodMedia | null>(null)
const vodView = ref<VodView>('list')
const vodDetailOrigin = ref<VodDetailOrigin>('list')
const vodSearchCache = ref<VodSearchCache<VodItem> | null>(null)
const episodePage = ref(0)
const currentEpisodeID = ref('')
const episodePagination = ref<HTMLElement | null>(null)
const vodQuery = ref('')
const vodSearchHistory = ref<string[]>([])
const vodFavorites = ref<VodFavoriteInfo[]>([])
const vodFavorited = ref(false)
const vodPage = ref(0)
let vodCategoryRequest = 0
const vodCategoryLoading = ref(false)
const vodCategoryError = ref('')
const livePlaybackPlan = ref<PlaybackPlan | null>(null)
const vodPlaybackPlan = ref<PlaybackPlan | null>(null)
const playbackOwner = ref<'live' | 'vod' | null>(null)
const activePlayback = ref<ActivePlaybackSession | null>(null)
const livePlaybackToken = ref(0)
const vodPlaybackToken = ref(0)
const livePlaybackStatus = ref<PlaybackStatus>('idle')
const vodPlaybackStatus = ref<PlaybackStatus>('idle')
const livePlaybackError = ref('')
const vodPlaybackError = ref('')
let nextPlaybackToken = 0
const mpvReady = ref(false)
const mpvFallbackRequested = ref(false)
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
const searchRequest = ref(0)
const activeSearchQuery = ref('')
const completedSearchQuery = ref('')
const activeSearchID = ref(0)
const searchFloorID = ref(0)
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

const showMpvInstallPrompt = computed(() => shouldShowMpvInstallPrompt(platform.value, mpvReady.value, mpvFallbackRequested.value))

const activeEpisodes = computed(() => (vodDetail.value?.Episodes ?? []).filter(ep => ep.Source === activeSource.value))
const episodePages = computed(() => paginateEpisodes(activeEpisodes.value))
const episodeRanges = computed(() => episodePageRanges(activeEpisodes.value.length))
const visibleEpisodes = computed(() => episodePages.value[episodePage.value] ?? [])
const visibleVodItems = computed(() => vodView.value === 'search' ? vodSearchItems.value : vodListItems.value)
const activePlaybackPlan = computed(() => playbackPlanForMode(mode.value, {
  live: livePlaybackPlan.value,
  vod: vodPlaybackPlan.value,
}, playbackOwner.value ?? undefined))
const livePagePlaybackPlan = computed(() => mode.value === 'live' && playbackOwner.value === 'live' ? livePlaybackPlan.value : null)
const vodPagePlaybackPlan = computed(() => mode.value === 'vod' && vodView.value === 'detail' && playbackOwner.value === 'vod' ? vodPlaybackPlan.value : null)
const showVodNoResults = computed(() => shouldShowVodNoResults(vodQuery.value, completedSearchQuery.value, searching.value, vodSearchItems.value.length))

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
  } catch (e) {
    handleError(e)
  }
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
  if (s.Available) mpvFallbackRequested.value = false
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

async function switchMode(m: 'home' | 'vod' | 'live' | 'search' | 'favorites' | 'settings') {
  if (m !== 'search' && searching.value) await invalidateSearch()
  if (m !== 'live' && activePlayback.value?.scope === 'live') await stopPlayback('live')
  if (m !== 'vod' && activePlayback.value?.scope === 'vod') await stopPlayback('vod')
  if (vodView.value === 'detail') currentVod.value = null
  mode.value = m
  if (m === 'vod') {
    vodView.value = 'list'
    vodDetail.value = null
    await refreshVod()
  }
  else if (m === 'live') await reloadGroups()
  else if (m === 'search') {
    vodView.value = 'search'
    vodDetail.value = null
    await loadVodSearchHistory()
  }
  else if (m === 'favorites') {
    vodView.value = 'list'
    vodDetail.value = null
    await loadVodFavorites()
  }
  else if (m === 'home') await refreshHome()
  else if (m === 'settings') { await reloadSourceHistory(); await refreshLogs(); await loadSearchThreads() }
}

async function refreshHome() {
  try {
    homeHistory.value = (await ShellService.ListVodHistory()) ?? []
  } catch (e) { handleError(e) }
}

async function loadVodSearchHistory() {
  try { vodSearchHistory.value = (await ShellService.ListVodSearchHistory()) ?? [] } catch (e) { handleError(e) }
}

async function loadVodFavorites() {
  try { vodFavorites.value = (await ShellService.ListVodFavorites()) ?? [] } catch (e) { handleError(e) }
}

async function deleteHomeHistory(h: VodHistoryInfo) {
  try {
    await ShellService.DeleteVodHistory(h.Site, h.VodID)
    homeHistory.value = removeVodHistory(homeHistory.value, h.Site, h.VodID)
  } catch (e) { handleError(e) }
}

async function deleteVodSearchHistory(query: string) {
  try {
    await ShellService.DeleteVodSearchHistory(query)
    vodSearchHistory.value = removeVodSearchHistory(vodSearchHistory.value, query)
  } catch (e) { handleError(e) }
}

async function useVodSearchHistory(query: string) {
  vodQuery.value = query
  await vodSearch()
}

async function deleteVodFavorite(favorite: VodFavoriteInfo) {
  try {
    await ShellService.RemoveVodFavorite(favorite.Site, favorite.VodID)
    vodFavorites.value = removeVodFavorite(vodFavorites.value, favorite.Site, favorite.VodID)
    if (currentVod.value?.site === favorite.Site && currentVod.value.vodID === favorite.VodID) vodFavorited.value = false
  } catch (e) { handleError(e) }
}

async function toggleVodFavorite() {
  const detail = vodDetail.value
  const site = detailSite.value || activeSite.value
  if (!detail || !site) return
  try {
    if (vodFavorited.value) {
      await ShellService.RemoveVodFavorite(site, detail.ID)
      vodFavorited.value = false
      vodFavorites.value = removeVodFavorite(vodFavorites.value, site, detail.ID)
    } else {
      await ShellService.AddVodFavorite(site, detail.ID, detail.Title, detail.Logo, detail.Group)
      vodFavorited.value = true
      await loadVodFavorites()
    }
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
    await stopPlayback('live')
    mode.value = 'vod'
    await loadSources()
    activeSite.value = h.Site
    detailSite.value = h.Site
    vodDetailOrigin.value = 'home'
    vodView.value = 'detail'
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
  const token = beginPlayback('live')
  livePlaybackPlan.value = null
  livePlaybackToken.value = 0
  livePlaybackStatus.value = 'preparing'
  livePlaybackError.value = ''
  liveNowPlaying.value = c.Name
  try {
    const plan = await ShellService.PrepareChannelWithToken(c.ID, token) as unknown as PlaybackPlan
    if (!isCurrentPlayback('live', token)) { await pauseStalePlayback(token); return }
    livePlaybackPlan.value = plan
    livePlaybackToken.value = token
    livePlaybackStatus.value = 'playing'
  } catch (e) {
    if (isCurrentPlayback('live', token)) {
      livePlaybackPlan.value = null
      livePlaybackToken.value = 0
      livePlaybackStatus.value = 'error'
      livePlaybackError.value = String(e)
      handleError(e)
    }
  }
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
  const last = activeSite.value || await ShellService.LastVodSite()
  const selection = resolveVodSelection(vodSites.value, last)
  activeSite.value = selection.site
  activeLine.value = selection.line
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
    vodView.value = 'list'
    vodPage.value = 0
    await reloadVodCategories()
    await ShellService.SetLastVodSite(first.ID)
  }
}

async function selectSite(id: string) {
  activeSite.value = id
  vodDetail.value = null
  vodView.value = 'list'
  vodPage.value = 0
  await reloadVodCategories()
  await ShellService.SetLastVodSite(id)
}

async function reloadVodCategories() {
  const request = nextVodCategoryRequest(vodCategoryRequest)
  vodCategoryRequest = request
  vodCategoryLoading.value = true
  vodCategoryError.value = ''
  vodCategories.value = []
  vodActiveCat.value = ''
  vodListItems.value = []
  try {
    const categories = (await ShellService.VodCategories(activeSite.value)) ?? []
    if (!isCurrentVodCategoryRequest(request, vodCategoryRequest)) return
    vodCategories.value = categories
    vodActiveCat.value = categories[0]?.ID ?? ''
    if (vodActiveCat.value) await reloadVodList()
  } catch (e) {
    if (isCurrentVodCategoryRequest(request, vodCategoryRequest)) {
      vodCategoryError.value = String(e)
      handleError(e)
    }
  } finally {
    if (isCurrentVodCategoryRequest(request, vodCategoryRequest)) vodCategoryLoading.value = false
  }
}

async function reloadVodList() {
  vodListItems.value = (await ShellService.VodList(activeSite.value, vodActiveCat.value, vodPage.value)) ?? []
}

async function vodSearch() {
  const searchQuery = vodQuery.value.trim()
  const request = nextVodSearchRequest(searchRequest.value)
  searchRequest.value = request
  activeSearchQuery.value = searchQuery
  completedSearchQuery.value = ''
  searchFloorID.value = activeSearchID.value
  activeSearchID.value = 0
  if (vodView.value === 'detail') await stopPlayback('vod')
  searchProgress.value = null
  if (!searchQuery) {
    searching.value = false
    try { await ShellService.CancelSearch() } catch { /* 忽略 */ }
    vodSearchItems.value = []
    vodSearchCache.value = null
    return
  }
  searching.value = true
  currentVod.value = null
  mode.value = 'search'
  vodView.value = 'search'
  vodDetail.value = null
  vodSearchItems.value = []
  void ShellService.RecordVodSearch(searchQuery).then(() => {
    vodSearchHistory.value = upsertVodSearchHistory(vodSearchHistory.value, searchQuery)
  }).catch(() => {})
  try {
    const items = (await ShellService.VodSearchAllWithToken(searchQuery, request)) ?? []
    if (searchRequest.value !== request || activeSearchQuery.value !== searchQuery) return
    vodSearchItems.value = items
    vodSearchCache.value = createVodSearchCache(searchQuery, items)
    completedSearchQuery.value = searchQuery
  } catch (e) {
    if (searchRequest.value === request) handleError(e)
  }
  finally {
    if (searchRequest.value === request) searching.value = false
  }
}

async function cancelSearch() {
  await invalidateSearch()
  vodSearchCache.value = createVodSearchCache(activeSearchQuery.value, vodSearchItems.value)
}

async function invalidateSearch() {
  searchRequest.value = nextVodSearchRequest(searchRequest.value)
  activeSearchID.value = 0
  searching.value = false
  try { await ShellService.CancelSearch() } catch { /* 忽略 */ }
}

async function openVodDetail(item: VodItem) {
  errMsg.value = ''
  try {
    if (searching.value) await cancelSearch()
    const site = item.Site || activeSite.value
    vodDetailOrigin.value = mode.value === 'favorites'
      ? 'favorites'
      : mode.value === 'search' || vodView.value === 'search' ? 'search' : 'list'
    detailSite.value = site
    const d = await ShellService.VodDetail(site, item.ID)
    d.Description = DOMPurify.sanitize(d.Description)
    vodDetail.value = d
    mode.value = 'vod'
    vodView.value = 'detail'
    activeSource.value = d.Sources?.[0] ?? ''
    resetEpisodePage()
    vodFavorited.value = await ShellService.IsVodFavorite(site, d.ID)
  } catch (e) { handleError(e) }
}

async function openVodFavorite(favorite: VodFavoriteInfo) {
  await openVodDetail({
    ID: favorite.VodID,
    Title: favorite.Title,
    Logo: favorite.Logo,
    Group: favorite.Group,
    Site: favorite.Site,
  })
}

async function backFromVodDetail() {
  const target = vodBackTarget(vodDetailOrigin.value)
  await stopPlayback('vod')
  currentVod.value = null
  vodDetail.value = null
  if (target === 'home') {
    mode.value = 'home'
    await refreshHome()
    return
  }
  if (target === 'search') {
    if (isVodSearchCacheValid(vodSearchCache.value)) {
      vodQuery.value = vodSearchCache.value!.query
      vodSearchItems.value = [...vodSearchCache.value!.items]
      mode.value = 'search'
      vodView.value = 'search'
      return
    }
    vodQuery.value = vodSearchQueryForReturn(vodSearchCache.value, vodQuery.value.trim())
    await vodSearch()
    return
  }
  if (target === 'favorites') {
    mode.value = 'favorites'
    await loadVodFavorites()
    return
  }
  vodView.value = 'list'
}

async function backFromVodSearch() {
  if (searching.value) {
    await invalidateSearch()
  }
  vodDetail.value = null
  await switchMode('vod')
}

async function doPlayEpisode(site: string, epID: string, epName: string, source: string) {
  await stopPlayback('live')
  const token = beginPlayback('vod')
  vodPlaybackPlan.value = null
  vodPlaybackToken.value = 0
  vodPlaybackStatus.value = 'preparing'
  vodPlaybackError.value = ''
  vodNowPlaying.value = epName
  let plan: PlaybackPlan
  try {
    plan = await ShellService.PrepareVodWithToken(site, epID, token) as unknown as PlaybackPlan
  } catch (e) {
    if (isCurrentPlayback('vod', token)) {
      vodPlaybackPlan.value = null
      vodPlaybackToken.value = 0
      vodPlaybackStatus.value = 'error'
      vodPlaybackError.value = String(e)
      throw e
    }
    return false
  }
  if (!isCurrentPlayback('vod', token)) { await pauseStalePlayback(token); return false }
  vodPlaybackPlan.value = plan
  vodPlaybackToken.value = token
  vodPlaybackStatus.value = 'playing'
  currentEpisodeID.value = epID
  if (vodDetail.value) {
    currentVod.value = { site, vodID: vodDetail.value.ID }
    await ShellService.RecordVodHistory(site, vodDetail.value.ID, vodDetail.value.Title, vodDetail.value.Logo, epID, epName, source)
  }
  return true
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  pendingSeek.value = 0
  try {
    await doPlayEpisode(detailSite.value || activeSite.value, ep.ID, ep.Name, ep.Source)
  } catch (e) {
    if (isCurrentPlayback('vod', vodPlaybackToken.value)) handleError(e)
  }
}

async function fallbackToMpv(scope: PlaybackScope, id: string, token: number) {
  mpvFallbackRequested.value = true
  if (scope === 'live') {
    livePlaybackStatus.value = 'preparing'
    livePlaybackError.value = ''
  } else {
    vodPlaybackStatus.value = 'preparing'
    vodPlaybackError.value = ''
  }
  try {
    const applied = await resolvePlaybackFallback(
      scope,
      async () => await ShellService.FallbackToMPVWithToken(id, token) as unknown as PlaybackPlan,
      () => isCurrentPlayback(scope, token) &&
        (scope === 'live' ? mode.value === 'live' : mode.value === 'vod' && vodView.value === 'detail'),
      (target, plan) => {
        if (target === 'live') {
          livePlaybackPlan.value = plan
          livePlaybackStatus.value = 'playing'
        } else {
          vodPlaybackPlan.value = plan
          vodPlaybackStatus.value = 'playing'
        }
      },
    )
    if (!applied) await pauseStalePlayback(token)
    else {
      try { await refreshMpvStatus() } catch { /* 播放已回退，状态检测失败不影响播放 */ }
    }
  }
  catch (e) {
    if (isCurrentPlayback(scope, token)) {
      if (scope === 'live') {
        livePlaybackPlan.value = null
        livePlaybackToken.value = 0
        livePlaybackStatus.value = 'error'
        livePlaybackError.value = String(e)
      } else {
        vodPlaybackPlan.value = null
        vodPlaybackToken.value = 0
        vodPlaybackStatus.value = 'error'
        vodPlaybackError.value = String(e)
      }
      handleError(e)
    }
  }
}

async function stopPlayback(scope: PlaybackScope) {
  const ownsPlayer = activePlayback.value?.scope === scope
  const token = ownsPlayer ? activePlayback.value!.token : 0
  if (scope === 'live') {
    livePlaybackPlan.value = null
    livePlaybackToken.value = 0
    liveNowPlaying.value = ''
    livePlaybackStatus.value = 'idle'
    livePlaybackError.value = ''
  } else {
    vodPlaybackPlan.value = null
    vodPlaybackToken.value = 0
    vodNowPlaying.value = ''
    currentVod.value = null
    vodPlaybackStatus.value = 'idle'
    vodPlaybackError.value = ''
  }
  if (ownsPlayer) {
    activePlayback.value = null
    playbackOwner.value = null
    try { await ShellService.StopPlayback(token) } catch { /* 播放器未就绪时无需处理 */ }
  }
}

async function pauseStalePlayback(token: number) {
  if (!shouldPauseStalePlayback(activePlayback.value)) return
  try { await ShellService.StopPlayback(token) } catch { /* 过期播放尚未建立时无需处理 */ }
}

function beginPlayback(scope: PlaybackScope): number {
  const token = ++nextPlaybackToken
  activePlayback.value = { scope, token }
  playbackOwner.value = scope
  return token
}

function isCurrentPlayback(scope: PlaybackScope, token: number): boolean {
  const active = activePlayback.value
  return !!active && active.scope === scope && active.token === token
}

async function onVodProgress(token: number, time: number, duration: number) {
  if (!isCurrentPlayback('vod', token) || !shouldRecordVodProgress(mode.value, vodView.value) || !currentVod.value) return
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
  const token = vodPlaybackToken.value
  if (!isCurrentPlayback('vod', token) || !shouldRecordVodProgress(mode.value, vodView.value) || !currentVod.value || activePlaybackPlan.value?.Backend !== 'mpv') return
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
  Events.On('search:start', (ev: any) => {
    const data = ev.data as { ID: number; Token: number; Query: string }
    if (searching.value && data.Token === searchRequest.value && data.Query === activeSearchQuery.value && data.ID > searchFloorID.value) activeSearchID.value = data.ID
  })
  Events.On('search:progress', (ev: any) => {
    const data = ev.data as Progress & { ID?: number; Token?: number; Query?: string }
    if (searching.value && data.Token === searchRequest.value && data.ID !== undefined && data.ID > searchFloorID.value && data.Query === activeSearchQuery.value) {
      if (activeSearchID.value === 0) activeSearchID.value = data.ID
      if (data.ID === activeSearchID.value) searchProgress.value = data
    }
  })
  Events.On('search:result', (ev: any) => {
    const data = ev.data as { ID: number; Token: number; Query: string; Items: VodItem[] }
    if (searching.value && data.Token === searchRequest.value && data.ID > searchFloorID.value && data.Query === activeSearchQuery.value) {
      if (activeSearchID.value === 0) activeSearchID.value = data.ID
      if (data.ID !== activeSearchID.value) return
      vodSearchItems.value = [...vodSearchItems.value, ...(data.Items ?? [])]
      vodSearchCache.value = createVodSearchCache(activeSearchQuery.value, vodSearchItems.value)
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

    <div v-if="showMpvInstallPrompt" class="mpv-install" aria-live="polite">
      <span>mpv 插件未安装（HEVC / RTMP / 本地文件需要它）</span>
      <button @click="installMpv">{{ mpvInstallMode === 'download' ? '下载并安装 mpv' : '显示安装命令' }}</button>
      <button v-if="mpvInstallMode && mpvInstallMode !== 'download'" @click="recheckMpv">我已安装，重新检测</button>
      <p v-if="installMessage" class="install-cmd">{{ installMessage }}</p>
    </div>

    <nav class="tabs">
      <button :class="{ active: mode === 'home' }" @click="switchMode('home')">首页</button>
      <button :class="{ active: mode === 'vod' }" @click="switchMode('vod')">点播</button>
      <button :class="{ active: mode === 'live' }" @click="switchMode('live')">直播</button>
      <button :class="{ active: mode === 'search' }" @click="switchMode('search')">搜索</button>
      <button :class="{ active: mode === 'favorites' }" @click="switchMode('favorites')">收藏</button>
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
          <button class="row-delete" type="button" title="删除观看记录" @click.stop="deleteHomeHistory(h)">删除</button>
        </li>
      </ul>
    </section>

    <!-- 点播搜索 -->
    <section v-if="mode === 'search'" class="search-page">
      <div class="search-page-head">
        <button class="vod-back" type="button" @click="backFromVodSearch">← 点播</button>
        <form class="search" @submit.prevent="vodSearch">
          <input v-model="vodQuery" placeholder="搜索影片" />
          <button type="submit">搜索</button>
          <button v-if="searching" type="button" @click="cancelSearch">取消</button>
          <span v-if="searchProgress" class="progress">{{ searchProgress.Message }}</span>
        </form>
      </div>
      <div v-if="vodSearchHistory.length" class="search-history">
        <span class="search-history-title">历史搜索</span>
        <span v-for="term in vodSearchHistory" :key="term" class="search-history-item">
          <button type="button" @click="useVodSearchHistory(term)">{{ term }}</button>
          <button type="button" class="row-delete" title="删除搜索词" @click="deleteVodSearchHistory(term)">×</button>
        </span>
      </div>
      <p v-if="showVodNoResults" class="home-empty">暂无搜索结果</p>
      <section class="vod-main search-results">
        <ul>
          <li v-for="it in vodSearchItems" :key="it.ID + (it.Site || '')" class="channel" @click="openVodDetail(it)">
            <img v-if="it.Logo" :src="it.Logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
            <span class="name">{{ it.Title }}</span><span class="group">{{ vodItemSub(it) }}</span>
          </li>
        </ul>
      </section>
    </section>

    <!-- 点播收藏 -->
    <section v-if="mode === 'favorites'" class="favorites-page">
      <h2>点播收藏</h2>
      <p v-if="!vodFavorites.length" class="home-empty">暂无点播收藏</p>
      <ul v-else class="favorites-list">
        <li v-for="favorite in vodFavorites" :key="favorite.Site + favorite.VodID" @click="openVodFavorite(favorite)">
          <img v-if="favorite.Logo" :src="favorite.Logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
          <span class="home-info">
            <span class="name">{{ favorite.Title }}</span>
            <span class="sub">{{ siteName(favorite.Site) || favorite.Site }}{{ favorite.Group ? ' · ' + favorite.Group : '' }}</span>
          </span>
          <button class="row-delete" type="button" title="删除收藏" @click.stop="deleteVodFavorite(favorite)">删除</button>
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
        <p v-if="livePlaybackStatus === 'preparing'" class="playback-status" aria-live="polite">正在切换频道…</p>
        <p v-if="livePlaybackStatus === 'error'" class="playback-error" aria-live="assertive">频道播放失败：{{ livePlaybackError }}</p>
        <PlaybackView :plan="livePagePlaybackPlan" @fallback="id => fallbackToMpv('live', id, livePlaybackToken)" />
        <div class="controls" v-if="liveNowPlaying && livePagePlaybackPlan?.Backend === 'mpv'">
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
            <span class="name" :title="c.Name"><span class="name-text">{{ c.Name }}</span></span>
            <span class="group">{{ c.Group }}</span>
            <button @click="play(c)">▶ 播放</button>
            <button @click="toggleFav(c)">{{ c.Favorited ? '★' : '☆' }}</button>
          </li>
        </ul>
      </section>
    </section>

    <!-- 点播 -->
    <section v-if="mode === 'vod'" class="vod">
      <div v-if="vodView === 'list'" class="vod-toolbar">
        <select v-if="vodLines.length > 1" v-model="activeLine" @change="selectLine(activeLine)">
          <option v-for="l in vodLines" :key="l" :value="l">{{ l || '默认线路' }}</option>
        </select>
        <select v-model="activeSite" @change="selectSite(activeSite)">
          <option v-for="s in sitesOfLine(activeLine)" :key="s.ID" :value="s.ID">{{ s.Name }}</option>
        </select>
      </div>

      <p v-if="vodCategoryLoading" class="vod-state" aria-live="polite">正在加载分类…</p>
      <p v-else-if="vodCategoryError" class="vod-state vod-state-error" aria-live="assertive">分类加载失败：{{ vodCategoryError }}</p>

      <div class="vod-layout" :class="{ 'cats-collapsed': catsCollapsed, 'without-cats': vodView !== 'list' }">
        <aside v-if="vodView === 'list'" class="vod-cats" :class="{ collapsed: catsCollapsed }">
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
          <VodDetailHeader v-if="vodView === 'detail'" :now-playing="vodNowPlaying" @back="backFromVodDetail" />
          <ul v-if="vodView !== 'detail'">
            <li v-for="it in visibleVodItems" :key="it.ID + (it.Site || '')" class="channel" @click="openVodDetail(it)">
              <img v-if="it.Logo" :src="it.Logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
              <span class="name">{{ it.Title }}</span><span class="group">{{ vodItemSub(it) }}</span>
            </li>
          </ul>
          <div v-else-if="vodView === 'detail' && vodDetail" class="vod-detail">
            <div class="vod-detail-top" :class="{ 'info-collapsed': infoCollapsed }">
              <div class="vod-player">
                <p v-if="vodPlaybackStatus === 'preparing'" class="playback-status" aria-live="polite">正在加载剧集…</p>
                <p v-if="vodPlaybackStatus === 'error'" class="playback-error" aria-live="assertive">剧集播放失败：{{ vodPlaybackError }}</p>
                <PlaybackView :plan="vodPagePlaybackPlan" :seek-to="pendingSeek" @fallback="id => fallbackToMpv('vod', id, vodPlaybackToken)" @progress="(time, duration) => onVodProgress(vodPlaybackToken, time, duration)" />
                <div class="controls" v-if="vodNowPlaying && vodPagePlaybackPlan?.Backend === 'mpv'">
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
                  <button class="favorite-toggle" type="button" :aria-pressed="vodFavorited" @click="toggleVodFavorite">{{ vodFavorited ? '取消收藏' : '收藏' }}</button>
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
          <li><a href="https://github.com/wailsapp/wails" target="_blank" rel="noopener">Wails</a><span class="oss-ver">v3.0.0-beta.9</span><span class="oss-lic">MIT</span> — 桌面应用框架</li>
          <li><a href="https://gitlab.com/cznic/sqlite" target="_blank" rel="noopener">modernc.org/sqlite</a><span class="oss-ver">v1.56.0</span><span class="oss-lic">BSD-3</span> — 纯 Go SQLite</li>
          <li><a href="https://github.com/PuerkitoBio/goquery" target="_blank" rel="noopener">goquery</a><span class="oss-ver">v1.13.0</span><span class="oss-lic">BSD-3</span> — HTML 文档解析</li>
          <li><a href="https://github.com/dop251/goja" target="_blank" rel="noopener">goja</a><span class="oss-ver">2026-08-26</span><span class="oss-lic">MIT</span> — Go JavaScript 引擎</li>
          <li><a href="https://pkg.go.dev/golang.org/x/text" target="_blank" rel="noopener">golang.org/x/text</a><span class="oss-ver">v0.41.0</span><span class="oss-lic">BSD-3</span> — 文本编码与语言支持</li>
          <li><a href="https://github.com/vuejs/core" target="_blank" rel="noopener">Vue</a><span class="oss-ver">v3.5.41</span><span class="oss-lic">MIT</span> — 前端框架</li>
          <li><a href="https://github.com/wailsapp/wails/tree/master/v3/pkg/runtime" target="_blank" rel="noopener">@wailsio/runtime</a><span class="oss-ver">v3.0.0-beta.9</span><span class="oss-lic">MIT</span> — Wails 前端运行时</li>
          <li><a href="https://github.com/video-dev/hls.js" target="_blank" rel="noopener">hls.js</a><span class="oss-ver">v1.7.1</span><span class="oss-lic">Apache-2.0</span> — HLS 播放</li>
          <li><a href="https://github.com/xqq/mpegts.js" target="_blank" rel="noopener">mpegts.js</a><span class="oss-ver">v1.8.2</span><span class="oss-lic">MIT</span> — MPEG-TS / FLV 播放</li>
          <li><a href="https://github.com/cure53/DOMPurify" target="_blank" rel="noopener">DOMPurify</a><span class="oss-ver">v3.4.14</span><span class="oss-lic">Apache-2.0</span> — HTML 清洗</li>
          <li><a href="https://github.com/vitejs/vite" target="_blank" rel="noopener">Vite</a><span class="oss-ver">v8.2.1</span><span class="oss-lic">MIT</span> — 构建工具</li>
          <li><a href="https://github.com/vitejs/vite-plugin-vue" target="_blank" rel="noopener">@vitejs/plugin-vue</a><span class="oss-ver">v6.0.8</span><span class="oss-lic">MIT</span> — Vue Vite 插件</li>
          <li><a href="https://github.com/microsoft/TypeScript" target="_blank" rel="noopener">TypeScript</a><span class="oss-ver">v4.9.5</span><span class="oss-lic">Apache-2.0</span> — 类型系统</li>
          <li><a href="https://github.com/vuejs/test-utils" target="_blank" rel="noopener">@vue/test-utils</a><span class="oss-ver">v2.5.0</span><span class="oss-lic">MIT</span> — Vue 测试工具</li>
          <li><a href="https://github.com/jsdom/jsdom" target="_blank" rel="noopener">jsdom</a><span class="oss-ver">v30.0.1</span><span class="oss-lic">MIT</span> — DOM 测试环境</li>
          <li><a href="https://github.com/vitest-dev/vitest" target="_blank" rel="noopener">Vitest</a><span class="oss-ver">v4.1.11</span><span class="oss-lic">MIT</span> — 单元测试框架</li>
          <li><a href="https://github.com/vuejs/language-tools" target="_blank" rel="noopener">vue-tsc</a><span class="oss-ver">v1.8.27</span><span class="oss-lic">MIT</span> — Vue 类型检查</li>
        </ul>
      </div>
    </div>

    <div v-if="errMsg" class="error-box">
      <span class="error-text">{{ errMsg }}</span>
      <button class="error-close" title="关闭" @click="errMsg = ''">✕</button>
    </div>
  </main>
</template>
