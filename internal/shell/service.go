package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/library"
	"github.com/unbox/unbox/internal/library/thumb"
	"github.com/unbox/unbox/internal/playback"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvplugin"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/provider/tvbox"
	"github.com/unbox/unbox/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"os"
	"runtime"
)

// ImportResult 是导入订阅的摘要。
// 播放列表导入时 Channels 立即可用；FongMi多线路 配置导入时 Sites/LiveSources 非空，
// 直播源需前端调用 LoadLive 按需加载。
type ImportResult struct {
	Sites       int // 点播站点数（配置导入）
	LiveSources int // 直播源数（配置导入，待按需加载）
	Channels    int // 频道数（播放列表导入，立即可用）
}

// persistedSubscription 是存进 store 的订阅快照，重启后据此无网络重建状态。
// 点播源与直播源分开存储，便于在设置里分别配置。
type persistedSubscription struct {
	VodRef   string           `json:"vodRef,omitempty"`   // 当前点播源地址
	VodCFGs  []*config.Config `json:"vodCfgs,omitempty"`  // 点播源：解析后的终端配置
	LiveRef  string           `json:"liveRef,omitempty"`  // 当前直播源地址
	LiveCFGs []*config.Config `json:"liveCfgs,omitempty"` // 直播源：解析后的终端配置
	Channels []config.Channel `json:"channels,omitempty"` // 直播源（播放列表）：已组装频道
	// 旧版字段（向后兼容早期单源快照）
	Ref  string           `json:"ref,omitempty"`
	CFGs []*config.Config `json:"cfgs,omitempty"`
}

// SourceRecord 是一条导入源的历史记录（供设置页展示当前源与历史源）。
type SourceRecord struct {
	Kind string
	Ref  string
	At   int64
}

// UpdateInfo 是「检查更新」的结果。
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	URL            string
}

// VodHistoryInfo 是「首页」展示的点播观看记录。
type VodHistoryInfo struct {
	Site     string
	SiteName string // 站点显示名
	VodID    string
	VodTitle string
	VodLogo  string
	EpID     string
	EpName   string
	Source   string
	Progress int // 秒
	Duration int // 秒
	At       int64
}

// subscriptionKey 是 store.kv 里订阅快照的键。
const subscriptionKey = "subscription"

// searchThreadsKey 是 store.kv 里全站搜索并发线程数的键。
const searchThreadsKey = "searchThreads"

// maxSearchThreads 是全站搜索并发度上限（与设置页选项一致）。
const maxSearchThreads = 16

// searchDeadSitesKey 是 store.kv 里「超时站点跳过名单」的键。
const searchDeadSitesKey = "searchDeadSites"

// searchCacheTTLKey 是 store.kv 里搜索结果缓存 TTL（分钟）的键。
const searchCacheTTLKey = "searchCacheTTLMinutes"

const contentCardStyleKey = "contentCardStyle"

const (
	playbackAutoNextKey         = "playback.autoNext"
	playbackAutoSwitchSourceKey = "playback.autoSwitchSource"
	playbackPreloadNextKey      = "playback.preloadNext"
)

// PlaybackSettings 是点播播放自动化选项。
type PlaybackSettings struct {
	AutoNext         bool
	AutoSwitchSource bool
	PreloadNext      bool
}

// appVersion 是当前应用版本（与 GitHub release tag 对齐）。
// 本地/开发构建默认为 0.0.1；发布构建通过 -ldflags
// "-X github.com/unbox/unbox/internal/shell.appVersion=<version>" 注入真实版本。
var appVersion = "0.0.1"

// updateRepo 是发布仓库（改名后改这里）。
const updateRepo = "teaGod-s/UnBox"

// updatePageURL 是最新 release 的网页入口：GitHub 会 302 到 /releases/tag/<tag>，
// 顺着重定向读 tag 即可，不占 API 配额。
const updatePageURL = "https://github.com/" + updateRepo + "/releases/latest"

// updateURL 是 GitHub 最新 release 的 API 地址，只作网页入口的备用。
// 未登录的 API 每小时只有 60 次配额，被限流时一律 403。
const updateURL = "https://api.github.com/repos/" + updateRepo + "/releases/latest"

// ChannelInfo 是前端展示用的频道信息。
type ChannelInfo struct {
	ID        string
	Name      string
	Group     string
	Logo      string
	Favorited bool
}

// SourceInfo 是顶层来源（直播或某个点播站）。
type SourceInfo struct {
	ID   string
	Name string
	Kind string // "live" | "vod"
	Line string // 所属线路名（单线路源为空）
}

// Section 是点播分类。
type Section struct {
	ID      string
	Title   string
	Filters []provider.FilterGroup `json:"filters,omitempty"` // 该分类的筛选定义；无筛选为空
}

type vodCategoryCacheEntry struct {
	sections  []Section
	expiresAt time.Time
}

const vodCategoryCacheTTL = 5 * time.Second

// VodFavoriteInfo 是前端展示用的点播收藏。
type VodFavoriteInfo struct {
	Site  string
	VodID string
	Title string
	Logo  string
	Group string
	At    int64
}

// VodItem 是点播影片列表项。
type VodItem struct {
	ID    string
	Title string
	Logo  string
	Group string
	Site  string // 所属站点 key（全站搜索时非空）
}

// VodListPage 是点播分类列表的一页，包含接口返回的分页信息。
type VodListPage struct {
	Items     []VodItem
	Page      int
	PageCount int
	Total     int
	HasMore   bool
}

// EpisodeInfo 是点播剧集（不向前端暴露 URL）。
type EpisodeInfo struct {
	ID     string
	Source string
	Name   string
}

// VodMedia 是点播详情。
type VodMedia struct {
	ID          string
	Title       string
	Logo        string
	Group       string
	Description string
	Year        string
	Area        string
	Type        string
	Remarks     string
	Sources     []string
	Episodes    []EpisodeInfo
}

// Progress 是导入进度快照，经 Wails 事件 "import:progress" 推给前端。
// Done/Total 为 -1 表示该阶段无法给出精确进度（如 resolve 阶段节点数是
// 边解析边发现的）。
type Progress struct {
	Stage   string // "resolve" | "live" | "done" | "error"
	Message string
	Done    int
	Total   int
}

// SearchStartEvent 标识一次全站搜索，前端据此丢弃过期搜索事件。
type SearchStartEvent struct {
	ID    uint64
	Token uint64
	Query string
}

// SearchProgressEvent 是带请求身份的全站搜索进度。
type SearchProgressEvent struct {
	ID    uint64
	Token uint64
	Query string
	Progress
}

// SearchResultEvent 是带请求身份的渐进搜索结果。
type SearchResultEvent struct {
	ID    uint64
	Token uint64
	Query string
	Items []VodItem
}

// newShellService 组装壳层服务但不启动播放器事件桥接。
// 便于测试在桥接启动前注入事件出口，避免与桥接 goroutine 争用字段。
// pv 为直播 Provider（可为 nil）；p 可为 nil（播放器未就绪）。
func newShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	root, _ := os.UserConfigDir()
	manager := mpvplugin.New(runtime.GOOS, root)
	controller := playback.NewController(playback.NewResolver(nil), playback.NewProxy(nil, 0), p)
	// Linux WebKitGTK 无 MSE 也不原生支持 HLS：hls.js/mpegts.js 用不了，
	// HLS/FLV/TS 必须走 mpv，只有 MP4 能由原生 <video> 播。其余平台 Web 能力默认为真。
	if runtime.GOOS == "linux" {
		controller.SetWebMSE(false)
	}
	svc := &ShellService{
		live:             pv,
		player:           p,
		store:            st,
		library:          library.New(st, filepath.Join(root, "unbox", "posters"), nil),
		vods:             map[string]provider.Provider{},
		vodNames:         map[string]string{},
		playback:         controller,
		mpvPlugin:        manager,
		vodCategoryCache: make(map[string]vodCategoryCacheEntry),
		vodCategoryNow:   time.Now,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		now:              time.Now,
	}
	svc.loadInsecureTLSHosts()
	svc.loadSearchDeadSites()
	svc.loadSearchThreads()
	return svc
}

// NewShellService 组装壳层服务并启动播放器事件桥接。
// pv 为直播 Provider（可为 nil）；p 可为 nil（播放器未就绪）。
func NewShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	svc := newShellService(pv, p, st)
	svc.playbackEventEmitter = svc.emitPlaybackEvent
	svc.startPlaybackBridge()
	return svc
}

// ServiceShutdown 在 Wails 退出服务阶段释放媒体库 HTTP 服务和播放资源。
func (s *ShellService) ServiceShutdown() error {
	s.stopPlaybackBridge()
	var firstErr error
	if s.library != nil {
		firstErr = s.library.Close()
	}
	if s.playback != nil {
		if err := s.playback.Close(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// AddLibraryDir 注册一个本地媒体库目录。
func (s *ShellService) AddLibraryDir(path string) error {
	return s.library.AddDir(path)
}

// RemoveLibraryDir 移除一个本地媒体库目录及其扫描条目。
func (s *ShellService) RemoveLibraryDir(path string) error {
	return s.library.RemoveDir(path)
}

// ListLibraryDirs 返回已注册的本地媒体库目录。
func (s *ShellService) ListLibraryDirs() ([]library.LibraryDir, error) {
	return s.library.ListDirs()
}

// ScanLibrary 扫描所有已注册的本地媒体库目录。
func (s *ShellService) ScanLibrary() (library.ScanResult, error) {
	return s.library.Scan()
}

// RescanLibrary 是 ScanLibrary 的前端友好别名。
func (s *ShellService) RescanLibrary() (library.ScanResult, error) {
	return s.ScanLibrary()
}

// ListLibrary 返回本地媒体库扫描出的条目。
func (s *ShellService) ListLibrary() ([]library.LibraryItem, error) {
	return s.library.List()
}

// EnsureThumb 注册文件 URL 与缓存 URL，返回是否已缓存。
func (s *ShellService) EnsureThumb(path string, mtime int64) (videoURL, posterURL string, cached bool, err error) {
	if s.library == nil {
		return "", "", false, errors.New("媒体库未就绪")
	}
	return s.library.EnsureThumb(path, mtime)
}

// SaveThumb 写入前端 web-canvas 抓到的 JPEG 字节到缓存。
func (s *ShellService) SaveThumb(path string, mtime int64, jpeg []byte) (posterURL string, err error) {
	if s.library == nil {
		return "", errors.New("媒体库未就绪")
	}
	return s.library.SaveThumb(path, mtime, jpeg)
}

// GenerateThumbMpv 使用当前可用的 mpv 抓取本地视频首帧。
func (s *ShellService) GenerateThumbMpv(path string, mtime int64) (posterURL string, err error) {
	if s.library == nil {
		return "", errors.New("媒体库未就绪")
	}
	mpvPath := ""
	if s.mpvPlugin != nil {
		mpvPath = s.mpvPlugin.Status().Path
	}
	return s.library.GenerateThumbMpv(path, mtime, thumb.NewMpvGenerator(mpvPath, 15*time.Second))
}

// PrepareLibrary 按本地媒体扩展名选择 Web 或 mpv 播放路由。
func (s *ShellService) PrepareLibrary(path string) (playback.Plan, error) {
	stream, err := s.library.StreamFor(path)
	if err != nil {
		return playback.Plan{}, err
	}
	return s.playback.Prepare(context.Background(), stream)
}

// RecordLibraryProgress 记录本地媒体的观看进度，供首页继续观看使用。
func (s *ShellService) RecordLibraryProgress(path string, progress, duration float64) error {
	return s.library.RecordProgress(path, int(progress), int(duration))
}

// progressMu 串行化进度事件的上报（collectChannels 的并发回调会并发触发 emit）。
var progressMu sync.Mutex

// emitProgress 把进度经 Wails 事件推给前端。并发调用安全。
func (s *ShellService) emitProgress(p Progress) {
	app := application.Get()
	if app == nil {
		return
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	app.Event.Emit("import:progress", p)
}

// emitSearchProgress 把全站搜索进度经 Wails 事件推给前端。
func (s *ShellService) emitSearchProgress(id, token uint64, q string, p Progress) {
	app := application.Get()
	if app == nil {
		return
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	app.Event.Emit("search:progress", SearchProgressEvent{ID: id, Token: token, Query: q, Progress: p})
}

// ImportSubscription 拉取并解析订阅，重建 Provider。支持两类输入：
//   - FongMi多线路 订阅配置（JSON，含 lives/storeHouse/urls，可能多仓）
//   - 独立 M3U/TXT 播放列表（#EXTM3U 或「名称,URL」行）
func (s *ShellService) ImportSubscription(ref string) (ImportResult, error) {
	s.emitProgress(Progress{Stage: "resolve", Message: "正在解析订阅…", Done: -1, Total: -1})
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		log.Printf("导入订阅拉取失败 %s: %v", ref, err)
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if err := checkSubscriptionContent(raw, ref); err != nil {
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, err
	}
	if isPlaylist(raw) {
		return s.importPlaylist(ref, raw, true)
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, err
	}

	// 直播源不在此拉取（m3u 数量多、耗时），仅收集定义，前端经 LoadLive 按需加载。
	liveList := flattenLives(cfgs)
	vods, vodNames, siteLines := collectVodSites(cfgs)
	s.mu.Lock()
	s.live = nil
	s.liveSources = liveList
	s.liveCount = 0
	s.vods = vods
	s.vodNames = vodNames
	s.vodSiteLines = siteLines
	s.vodCFGs = cfgs
	s.liveCFGs = cfgs
	s.liveChannels = nil
	s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	s.vodRef = ref
	s.liveRef = ref
	s.mu.Unlock()

	s.addSource("vod", ref)
	s.addSource("live", ref)
	s.saveSubscription()

	s.emitProgress(Progress{Stage: "done", Message: "导入完成", Done: -1, Total: -1})
	return ImportResult{Sites: len(vods), LiveSources: len(liveList)}, nil
}

// ImportVodSource 仅导入点播源（站点），不动现有直播源。
func (s *ShellService) ImportVodSource(ref string) (ImportResult, error) {
	s.emitProgress(Progress{Stage: "resolve", Message: "正在解析点播源…", Done: -1, Total: -1})
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		log.Printf("导入点播源拉取失败 %s: %v", ref, err)
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if err := checkSubscriptionContent(raw, ref); err != nil {
		return ImportResult{}, err
	}
	if isPlaylist(raw) {
		return ImportResult{}, errors.New("该源是直播播放列表，不含点播站点")
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		return ImportResult{}, err
	}
	vods, vodNames, siteLines := collectVodSites(cfgs)
	s.mu.Lock()
	s.vods = vods
	s.vodNames = vodNames
	s.vodSiteLines = siteLines
	s.vodCFGs = cfgs
	s.vodRef = ref
	s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	s.mu.Unlock()
	s.addSource("vod", ref)
	s.saveSubscription()
	s.emitProgress(Progress{Stage: "done", Message: "点播源导入完成", Done: -1, Total: -1})
	return ImportResult{Sites: len(vods)}, nil
}

// ImportLiveSource 仅导入直播源（频道），不动现有点播源。
func (s *ShellService) ImportLiveSource(ref string) (ImportResult, error) {
	s.emitProgress(Progress{Stage: "resolve", Message: "正在解析直播源…", Done: -1, Total: -1})
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		log.Printf("导入直播源拉取失败 %s: %v", ref, err)
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if err := checkSubscriptionContent(raw, ref); err != nil {
		return ImportResult{}, err
	}
	if isPlaylist(raw) {
		return s.importPlaylist(ref, raw, false)
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		return ImportResult{}, err
	}
	liveList := flattenLives(cfgs)
	s.mu.Lock()
	s.live = nil
	s.liveSources = liveList
	s.liveCount = 0
	s.liveCFGs = cfgs
	s.liveChannels = nil
	s.liveRef = ref
	s.mu.Unlock()
	s.addSource("live", ref)
	s.saveSubscription()
	s.emitProgress(Progress{Stage: "done", Message: "直播源导入完成", Done: -1, Total: -1})
	return ImportResult{LiveSources: len(liveList)}, nil
}

// addSource 记录一条导入源（按 kind）。
func (s *ShellService) addSource(kind, ref string) {
	if s.store != nil {
		_ = s.store.AddSource(kind, ref)
	}
}

// ListSources 返回导入源历史（最近在前），设置页据此展示当前源与历史源。
func (s *ShellService) ListSources() ([]SourceRecord, error) {
	if s.store == nil {
		return nil, nil
	}
	recs, err := s.store.ListSources()
	if err != nil {
		return nil, err
	}
	out := make([]SourceRecord, len(recs))
	for i, r := range recs {
		out[i] = SourceRecord{Kind: r.Kind, Ref: r.Ref, At: r.At}
	}
	return out, nil
}

// DeleteSource 删除一条导入源历史。
func (s *ShellService) DeleteSource(kind, ref string) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.DeleteSource(kind, ref); err != nil {
		return err
	}
	s.clearVodCategoryCache()
	return nil
}

// RecordVodHistory 记录一条点播观看记录（开始播放某集时）。
func (s *ShellService) RecordVodHistory(site, vodID, title, logo, epID, epName, source string) error {
	if s.store == nil {
		return nil
	}
	return s.store.UpsertVodHistory(store.VodHistory{
		Site: site, VodID: vodID, VodTitle: title, VodLogo: logo,
		EpID: epID, EpName: epName, Source: source,
	})
}

// UpdateVodProgress 更新点播观看进度（秒）。
func (s *ShellService) UpdateVodProgress(site, vodID string, progress, duration float64) error {
	if s.store == nil {
		return nil
	}
	return s.store.UpdateVodProgress(site, vodID, int(progress), int(duration))
}

// ListVodHistory 返回点播观看历史（最近在前）。
func (s *ShellService) ListVodHistory() ([]VodHistoryInfo, error) {
	if s.store == nil {
		return nil, nil
	}
	recs, err := s.store.ListVodHistory(100)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	siteNames := make(map[string]string, len(s.vodNames))
	for k, v := range s.vodNames {
		siteNames[k] = v
	}
	s.mu.RUnlock()
	out := make([]VodHistoryInfo, len(recs))
	for i, r := range recs {
		out[i] = VodHistoryInfo{
			Site: r.Site, SiteName: siteNames[r.Site], VodID: r.VodID, VodTitle: r.VodTitle, VodLogo: r.VodLogo,
			EpID: r.EpID, EpName: r.EpName, Source: r.Source,
			Progress: r.Progress, Duration: r.Duration, At: r.UpdatedAt,
		}
	}
	return out, nil
}

// GetVodHistory 返回指定点播的单条观看记录；无记录时返回零值。
func (s *ShellService) GetVodHistory(site, vodID string) (VodHistoryInfo, error) {
	if s.store == nil {
		return VodHistoryInfo{}, nil
	}
	rec, ok, err := s.store.GetVodHistory(site, vodID)
	if err != nil {
		return VodHistoryInfo{}, err
	}
	if !ok {
		return VodHistoryInfo{}, nil
	}
	s.mu.RLock()
	siteName := s.vodNames[rec.Site]
	s.mu.RUnlock()
	return VodHistoryInfo{
		Site: rec.Site, SiteName: siteName, VodID: rec.VodID, VodTitle: rec.VodTitle, VodLogo: rec.VodLogo,
		EpID: rec.EpID, EpName: rec.EpName, Source: rec.Source,
		Progress: rec.Progress, Duration: rec.Duration, At: rec.UpdatedAt,
	}, nil
}

// DeleteVodHistory 删除首页的一条点播观看记录。
func (s *ShellService) DeleteVodHistory(site, vodID string) error {
	if s.store == nil {
		return nil
	}
	return s.store.DeleteVodHistory(site, vodID)
}

// RecordVodSearch 保存一条点播搜索词。
func (s *ShellService) RecordVodSearch(query string) error {
	if s.store == nil {
		return nil
	}
	return s.store.RecordVodSearch(query)
}

// ListVodSearchHistory 返回最近点播搜索词。
func (s *ShellService) ListVodSearchHistory() ([]string, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ListVodSearchHistory(30)
}

// DeleteVodSearchHistory 删除一条点播搜索词。
func (s *ShellService) DeleteVodSearchHistory(query string) error {
	if s.store == nil {
		return nil
	}
	return s.store.DeleteVodSearch(query)
}

// AddVodFavorite 新增或更新一条点播收藏。
func (s *ShellService) AddVodFavorite(site, vodID, title, logo, group string) error {
	if s.store == nil {
		return nil
	}
	return s.store.AddVodFavorite(site, vodID, title, logo, group)
}

// RemoveVodFavorite 删除一条点播收藏。
func (s *ShellService) RemoveVodFavorite(site, vodID string) error {
	if s.store == nil {
		return nil
	}
	return s.store.RemoveVodFavorite(site, vodID)
}

// IsVodFavorite 判断一条点播是否已收藏。
func (s *ShellService) IsVodFavorite(site, vodID string) (bool, error) {
	if s.store == nil {
		return false, nil
	}
	return s.store.IsVodFavorite(site, vodID)
}

// ListVodFavorites 返回全部点播收藏。
func (s *ShellService) ListVodFavorites() ([]VodFavoriteInfo, error) {
	if s.store == nil {
		return nil, nil
	}
	favs, err := s.store.ListVodFavorites()
	if err != nil {
		return nil, err
	}
	out := make([]VodFavoriteInfo, len(favs))
	for i, f := range favs {
		out[i] = VodFavoriteInfo{Site: f.Site, VodID: f.VodID, Title: f.Title, Logo: f.Logo, Group: f.Group, At: f.AddedAt}
	}
	return out, nil
}

// saveSubscription 把当前点播/直播源快照写进 store（无网络）。store 为 nil 时静默跳过。
func (s *ShellService) saveSubscription() {
	if s.store == nil {
		return
	}
	s.mu.RLock()
	sub := persistedSubscription{
		VodRef: s.vodRef, VodCFGs: s.vodCFGs,
		LiveRef: s.liveRef, LiveCFGs: s.liveCFGs,
		Channels: s.liveChannels,
	}
	s.mu.RUnlock()
	b, err := json.Marshal(sub)
	if err != nil {
		return
	}
	_ = s.store.SetKV(subscriptionKey, string(b))
}

// RestoreSubscription 从 store 恢复上次导入的订阅快照并重建状态。
// 无快照或 store 为 nil 时返回零值 ImportResult 且不报错（视为「尚未导入」）。
// 恢复只做纯内存重建（flattenLives/collectVodSites/live.New），不触发网络。
func (s *ShellService) RestoreSubscription() (ImportResult, error) {
	if s.store == nil {
		return ImportResult{}, nil
	}
	raw, ok, err := s.store.GetKV(subscriptionKey)
	if err != nil {
		return ImportResult{}, err
	}
	if !ok {
		return ImportResult{}, nil
	}
	var sub persistedSubscription
	if err := json.Unmarshal([]byte(raw), &sub); err != nil {
		return ImportResult{}, err
	}

	// 向后兼容旧快照（单 ref/cfgs），并重新登记到 sources 表保证设置页回显。
	vodRef := sub.VodRef
	if vodRef == "" {
		vodRef = sub.Ref
	}
	vodCFGs := sub.VodCFGs
	if len(vodCFGs) == 0 {
		vodCFGs = sub.CFGs
	}
	liveRef := sub.LiveRef
	if liveRef == "" {
		liveRef = sub.Ref
	}
	liveCFGs := sub.LiveCFGs
	if len(liveCFGs) == 0 {
		liveCFGs = sub.CFGs
	}
	if vodRef != "" {
		_ = s.store.AddSource("vod", vodRef)
	}
	if liveRef != "" {
		_ = s.store.AddSource("live", liveRef)
	}

	// 恢复后重新解析相对 api/ext。对已解析过的绝对地址是幂等的
	// （resolveRelativeURL 只改写相对地址），而含相对路径的新快照则要在这一步
	// 补上解析，否则站点会在运行时才报「爬虫地址不是完整 URL（./lib/xxx.js）」。
	// 注意：不能用 vodRef/liveRef 兜底 —— 它们是顶层索引地址，而站点来自展开后的
	// 子配置，用错 base 只会把相对路径解析成 404 地址，比直接报错更难排查。
	resolveRelativeURLs(vodCFGs)
	resolveRelativeURLs(liveCFGs)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.vodRef = vodRef
	s.vodCFGs = vodCFGs
	s.liveRef = liveRef
	s.liveCFGs = liveCFGs
	s.liveChannels = sub.Channels

	var channels int
	if len(sub.Channels) > 0 {
		s.live = live.New(sub.Channels)
		s.liveSources = nil
		s.liveCount = len(sub.Channels)
		channels = len(sub.Channels)
	} else {
		s.live = nil
		s.liveSources = flattenLives(liveCFGs)
		s.liveCount = 0
	}
	s.vods, s.vodNames, s.vodSiteLines = collectVodSites(vodCFGs)
	s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	return ImportResult{Sites: len(s.vods), LiveSources: len(s.liveSources), Channels: channels}, nil
}

// LoadLive 按需拉取全部直播源并构建直播 provider。已加载则直接返回频道数。
func (s *ShellService) LoadLive() (int, error) {
	s.mu.RLock()
	alreadyLoaded := s.live != nil
	lives := s.liveSources
	count := s.liveCount
	s.mu.RUnlock()
	if alreadyLoaded {
		return count, nil
	}

	s.emitProgress(Progress{Stage: "live", Message: "正在拉取直播源…", Done: 0, Total: len(lives)})
	channels := collectChannels(context.Background(), lives, func(done, total int) {
		s.emitProgress(Progress{Stage: "live", Message: fmt.Sprintf("拉取直播源 %d/%d", done, total), Done: done, Total: total})
	})
	lp := live.New(channels)
	s.mu.Lock()
	s.live = lp
	s.liveCount = len(channels)
	s.mu.Unlock()
	s.emitProgress(Progress{Stage: "done", Message: "直播加载完成", Done: -1, Total: -1})
	return len(channels), nil
}

// flattenLives 把多份配置的直播源拍平成一个切片。
func flattenLives(cfgs []*config.Config) []config.Live {
	var lives []config.Live
	for _, cfg := range cfgs {
		lives = append(lives, cfg.Lives...)
	}
	return lives
}

// collectVodSites 从多份配置收集点播站点：type=1（CMS）→ tvbox.Provider，
// type=3 http（drpy2/drpyS 爬虫服务）→ tvbox.Drpy，.js → tvbox.Spider。
func collectVodSites(cfgs []*config.Config) (map[string]provider.Provider, map[string]string, map[string]string) {
	vods := make(map[string]provider.Provider)
	names := make(map[string]string)
	lines := make(map[string]string)
	for _, cfg := range cfgs {
		line := cfg.SourceName
		for _, site := range cfg.Sites {
			kind, _ := config.Classify(site)
			switch kind {
			case "cms":
				if site.API == "" {
					continue
				}
				vods[site.Key] = tvbox.New(site)
				names[site.Key] = site.Name
				lines[site.Key] = line
			case "http":
				vods[site.Key] = tvbox.NewDrpy(site)
				names[site.Key] = site.Name
				lines[site.Key] = line
			case "js":
				spider, err := tvbox.NewSpider(site)
				if err != nil {
					continue
				}
				vods[site.Key] = spider
				names[site.Key] = site.Name
				lines[site.Key] = line
			case "jar":
				jar, err := tvbox.NewJar(site)
				if err != nil {
					continue
				}
				vods[site.Key] = jar
				names[site.Key] = site.Name
				lines[site.Key] = line
			}
		}
	}
	return vods, names, lines
}

// isPlaylist 探测内容是否为独立播放列表而非 JSON 配置。
func isPlaylist(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return false
	}
	if bytes.HasPrefix(t, []byte("#EXTM3U")) {
		return true
	}
	// JSON 配置以 { 或 [ 开头；其余（如「名称,URL」行）按 TXT 播放列表处理。
	return t[0] != '{' && t[0] != '['
}

// looksLikeHTML 判断拉取到的内容是不是网页。
// 网页既不是 JSON 配置，也不是播放列表；不拦下来的话会被 isPlaylist 当成 TXT，
// 于是把 HTML 里含逗号的行解析成一堆垃圾频道。
func looksLikeHTML(raw []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))))
	if len(head) > htmlSniffBytes {
		head = head[:htmlSniffBytes]
	}
	if len(head) == 0 {
		return false
	}
	return bytes.HasPrefix(head, []byte("<!doctype html")) ||
		bytes.HasPrefix(head, []byte("<html")) ||
		bytes.Contains(head, []byte("<head")) ||
		bytes.Contains(head, []byte("<body"))
}

// htmlSniffBytes 是判定网页时检查的字节数。
const htmlSniffBytes = 1024

// binaryMagic 是常见的错误粘贴产物开头：图片（JPEG/PNG/BMP/GIF）、压缩包、
// PDF 等。这些既不是 HTML、也不是 JSON/播放列表，必须拦在 isPlaylist 之前，
// 否则会被当成 TXT 播放列表，报出「直播播放列表 / 没有可用频道」这类误导错误。
var binaryMagic = [][]byte{
	{0xFF, 0xD8, 0xFF},                // JPEG
	{0x89, 'P', 'N', 'G', '\r', '\n'}, // PNG
	{'B', 'M'},                        // BMP
	{'G', 'I', 'F', '8'},              // GIF87a / GIF89a
	{'P', 'K', 0x03, 0x04},            // ZIP（含 docx/xlsx/apk）
	{0x1F, 0x8B, 0x08},                // gzip
	{'%', 'P', 'D', 'F'},              // PDF
}

// looksLikeBinary 判断拉取到的内容是不是图片/压缩包等二进制文件。
// 命中已知魔数，或头部出现 NUL 字节（文本配置/播放列表不会带 NUL）都算。
func looksLikeBinary(raw []byte) bool {
	head := bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")))
	if len(head) > htmlSniffBytes {
		head = head[:htmlSniffBytes]
	}
	if len(head) == 0 {
		return false
	}
	for _, magic := range binaryMagic {
		if bytes.HasPrefix(head, magic) {
			return true
		}
	}
	return bytes.IndexByte(head, 0) >= 0
}

// checkSubscriptionContent 在导入前拦下「填了网页/图片地址」这类误操作，给出明确提示。
func checkSubscriptionContent(raw []byte, ref string) error {
	if looksLikeHTML(raw) {
		return fmt.Errorf("该地址返回的是网页而不是订阅配置：%s（请填写 JSON 接口或 M3U/TXT 播放列表地址）", ref)
	}
	if looksLikeBinary(raw) {
		return fmt.Errorf("该地址返回的是图片或二进制文件而不是订阅配置：%s（请填写 JSON 接口或 M3U/TXT 播放列表地址）", ref)
	}
	return nil
}

// importPlaylist 解析独立 M3U/TXT 播放列表并重建直播 Provider。
// clearVod 为 true 时同时清空点播站点（合并导入语义）；false 时保留（仅导直播源）。
func (s *ShellService) importPlaylist(ref string, raw []byte, clearVod bool) (ImportResult, error) {
	entries := live.ParseM3U(raw)
	if len(entries) == 0 {
		entries = live.ParseTXT(raw) // m3u 解析为空则按 TXT 回退
	}
	channels := live.Assemble(entries)
	if len(channels) == 0 {
		return ImportResult{}, errors.New("播放列表中没有可用频道")
	}
	lp := live.New(channels)
	s.mu.Lock()
	s.live = lp
	s.liveSources = nil
	s.liveCount = len(channels)
	s.liveCFGs = nil
	s.liveChannels = channels
	s.liveRef = ref
	if clearVod {
		s.vods = map[string]provider.Provider{}
		s.vodNames = map[string]string{}
		s.vodCFGs = nil
		s.vodRef = ""
		s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	}
	s.mu.Unlock()
	s.addSource("live", ref)
	s.saveSubscription()
	return ImportResult{Channels: len(channels)}, nil
}

// resolveConfigs 解析 FongMi多线路 配置；索引节点（storeHouse/urls 非空）交给 Resolver 展开。
func resolveConfigs(ctx context.Context, ref string, raw []byte) ([]*config.Config, error) {
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.SourceURL = ref
	if len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0 {
		cfgs, rerr := config.NewResolver().Resolve(ctx, ref)
		if rerr != nil && len(cfgs) == 0 {
			return nil, fmt.Errorf("展开订阅失败: %w", rerr)
		}
		resolveRelativeURLs(cfgs)
		return cfgs, nil
	}
	config.ResolveRelativeURLs(cfg)
	return []*config.Config{cfg}, nil
}

// resolveRelativeURLs 把每份配置里站点/直播的相对地址按各自的来源地址解析成绝对地址。
func resolveRelativeURLs(cfgs []*config.Config) {
	for _, cfg := range cfgs {
		config.ResolveRelativeURLs(cfg)
	}
}

// liveFetchTimeout 是直播源 m3u 的拉取超时。直播源数量多、死源占比高，
// 用比配置拉取（15s）更短的超时让死源快速失败，避免拖慢整次导入。
const liveFetchTimeout = 8 * time.Second

// collectChannels 从多份配置收集直播频道：Channels 内嵌的直接用，URL 指向的
// 并发拉取解析（按 URL 去重，同一 m3u 只拉一次）。并发时用 slot 记录原始
// 顺序，结果按原下标回填，保证频道顺序与串行版本一致（live.New 只对 group
// 排序，组内频道顺序依赖输入顺序）。progress（可为 nil）在每个直播源拉取
// 完成后回调 (done, total)。
func collectChannels(ctx context.Context, lives []config.Live, progress func(done, total int)) []config.Channel {
	fetcher := config.NewFetcherWithTimeout(liveFetchTimeout)

	// 第一遍：把每个直播源归为「内嵌频道」或「需拉取」，按 URL 去重并记录顺序。
	type slot struct {
		embedded []config.Channel
		job      int // 索引进 jobs；-1 表示内嵌
	}
	var slots []slot
	var jobs []config.Live
	seenURL := map[string]bool{}
	for _, lv := range lives {
		switch {
		case len(lv.Channels) > 0:
			slots = append(slots, slot{embedded: lv.Channels, job: -1})
		case lv.URL != "" && !seenURL[lv.URL]:
			seenURL[lv.URL] = true
			jobs = append(jobs, lv)
			slots = append(slots, slot{job: len(jobs) - 1})
		}
	}

	// 并发拉取，结果写进与 job 下标对应的位置（每个位置只被一个 goroutine 写）。
	results := make([][]config.Channel, len(jobs))
	const maxConcurrency = 16
	sem := make(chan struct{}, maxConcurrency)
	var done atomic.Int64
	var wg sync.WaitGroup
	for i, lv := range jobs {
		wg.Add(1)
		go func(i int, lv config.Live) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if chs, err := live.FetchLive(ctx, lv, fetcher); err == nil {
				results[i] = chs
			}
			if progress != nil {
				progress(int(done.Add(1)), len(jobs))
			}
		}(i, lv)
	}
	wg.Wait()

	// 按原始顺序组装：内嵌频道与拉取结果各自归位。
	var channels []config.Channel
	for _, s := range slots {
		if s.job >= 0 {
			channels = append(channels, results[s.job]...)
		} else {
			channels = append(channels, s.embedded...)
		}
	}
	return channels
}

func (s *ShellService) Groups() ([]string, error) {
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	secs, err := pv.Home(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]string, len(secs))
	for i, sec := range secs {
		out[i] = sec.Title
	}
	return out, nil
}

func (s *ShellService) Channels(group string, page int) ([]ChannelInfo, error) {
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	pg, err := pv.Browse(context.Background(), group, page)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(pg.Items), nil
}

func (s *ShellService) Search(q string) ([]ChannelInfo, error) {
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	items, err := pv.Search(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(items), nil
}

func (s *ShellService) toChannelInfo(items []provider.Item) []ChannelInfo {
	out := make([]ChannelInfo, len(items))
	for i, it := range items {
		var fav bool
		if s.store != nil {
			fav, _ = s.store.IsFavorite(it.ID)
		}
		out[i] = ChannelInfo{ID: it.ID, Name: it.Title, Group: it.Group, Logo: it.Logo, Favorited: fav}
	}
	return out
}

func (s *ShellService) PlayChannel(id string) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return errors.New("未导入订阅")
	}
	st, err := pv.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.player.Load(context.Background(), st); err != nil {
		return err
	}
	if s.store != nil {
		if m, err := pv.Detail(context.Background(), id); err == nil {
			_ = s.store.AddRecent(id, m.Title, m.Group, st.URL) // 尽力而为，失败不阻断播放
		}
	}
	return s.player.Play()
}

func (s *ShellService) PrepareChannel(id string) (playback.Plan, error) {
	return s.prepareChannel(id, 0)
}

// PrepareChannelWithToken 准备直播播放，并让后端丢弃已过期的前端请求。
func (s *ShellService) PrepareChannelWithToken(id string, token uint64) (playback.Plan, error) {
	return s.prepareChannel(id, token)
}

func (s *ShellService) prepareChannel(id string, token uint64) (playback.Plan, error) {
	requestID, accepted := s.claimPlayback(token)
	if !accepted {
		return playback.Plan{}, nil
	}
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return playback.Plan{}, errors.New("未导入订阅")
	}
	st, err := pv.Resolve(context.Background(), id)
	if err != nil {
		log.Printf("解析直播频道失败 id=%s: %v", id, err)
		return playback.Plan{}, err
	}
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if !s.playbackCurrent(token, requestID) {
		return playback.Plan{}, nil
	}
	plan, err := s.playback.Prepare(context.Background(), st)
	if err != nil {
		log.Printf("准备直播播放失败 id=%s: %v", id, err)
	}
	return plan, err
}

func (s *ShellService) Pause() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Pause()
}

func (s *ShellService) Resume() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Play()
}

func (s *ShellService) SetVolume(v int) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.SetVolume(v)
}

// Position 返回当前播放位置（秒），供前端轮询 mpv 进度。
func (s *ShellService) Position() float64 {
	if s.player == nil {
		return 0
	}
	return s.player.State().Position
}

// Seek 跳转到指定秒（mpv 后端断点续播用）。
func (s *ShellService) Seek(sec float64) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Seek(sec)
}

func (s *ShellService) AddFavorite(id string) error {
	s.mu.RLock()
	pv := s.live
	s.mu.RUnlock()
	if pv == nil {
		return errors.New("未导入订阅")
	}
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	m, err := pv.Detail(context.Background(), id)
	if err != nil {
		return err
	}
	st, err := pv.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	return s.store.AddFavorite(id, m.Title, m.Group, st.URL)
}

func (s *ShellService) RemoveFavorite(id string) error {
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	return s.store.RemoveFavorite(id)
}

func (s *ShellService) ListFavorites() ([]ChannelInfo, error) {
	if s.store == nil {
		return nil, errors.New("数据库未就绪，收藏不可用")
	}
	favs, err := s.store.ListFavorites()
	if err != nil {
		return nil, err
	}
	out := make([]ChannelInfo, len(favs))
	for i, f := range favs {
		out[i] = ChannelInfo{ID: f.ID, Name: f.Name, Group: f.Group, Favorited: true}
	}
	return out, nil
}

func (s *ShellService) AddGroup(name string) error {
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	return s.store.AddGroup(name)
}

func (s *ShellService) ListGroups() ([]string, error) {
	if s.store == nil {
		return nil, errors.New("数据库未就绪，收藏不可用")
	}
	gs, err := s.store.ListGroups()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Name
	}
	return out, nil
}

// Sources 返回顶层来源（直播 + 各点播站，按 key 排序稳定）。
func (s *ShellService) Sources() []SourceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []SourceInfo{{ID: "live", Name: "直播", Kind: "live"}}
	keys := make([]string, 0, len(s.vods))
	for k := range s.vods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, SourceInfo{ID: k, Name: s.vodNames[k], Kind: "vod", Line: s.vodSiteLines[k]})
	}
	return out
}

// vodOf 按站点 key 取点播 Provider。
func (s *ShellService) vodOf(site string) (provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pv, ok := s.vods[site]
	if !ok {
		return nil, fmt.Errorf("点播站点不存在: %s", site)
	}
	return pv, nil
}

func (s *ShellService) VodCategories(site string) ([]Section, error) {
	now := time.Now
	s.mu.RLock()
	if s.vodCategoryNow != nil {
		now = s.vodCategoryNow
	}
	entry, cached := s.vodCategoryCache[site]
	if cached && now().Before(entry.expiresAt) {
		out := append([]Section(nil), entry.sections...)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()

	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	secs, err := pv.Home(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Section, len(secs))
	for i, sc := range secs {
		out[i] = Section{ID: sc.ID, Title: sc.Title, Filters: sc.Filters}
	}
	s.mu.Lock()
	if s.vodCategoryCache == nil {
		s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	}
	s.vodCategoryCache[site] = vodCategoryCacheEntry{sections: append([]Section(nil), out...), expiresAt: now().Add(vodCategoryCacheTTL)}
	s.mu.Unlock()
	return out, nil
}

func (s *ShellService) clearVodCategoryCache() {
	s.mu.Lock()
	s.vodCategoryCache = make(map[string]vodCategoryCacheEntry)
	s.mu.Unlock()
}

func (s *ShellService) VodList(site, cat string, page int) (VodListPage, error) {
	return s.VodListFiltered(site, cat, page, nil)
}

// VodListFiltered 带筛选值浏览分类；filters 可为 nil（等价 VodList）。
func (s *ShellService) VodListFiltered(site, cat string, page int, filters map[string]string) (VodListPage, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return VodListPage{}, err
	}
	pg, err := pv.BrowseFiltered(context.Background(), cat, page, filters)
	if err != nil {
		return VodListPage{}, err
	}
	return VodListPage{
		Items:     toVodItems(pg.Items),
		Page:      pg.Page,
		PageCount: pg.PageCount,
		Total:     pg.Total,
		HasMore:   pg.HasMore,
	}, nil
}

func (s *ShellService) VodSearch(site, q string) ([]VodItem, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	items, err := pv.Search(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return toVodItems(items), nil
}

// siteSearchTimeout 是全站搜索里单个站点的执行上限。
// 站点搜索接口正常几秒内返回；卡到 15s 客户端超时再重试一次（约 30s）
// 的大多是源站不可达，占着并发线程不放会让「线程数调大也没有改善」。
var siteSearchTimeout = 12 * time.Second

// searchDeadSkipFor 是超时站点被短时跳过搜索的时长。
// 首次搜索把一批死站点全部按 12s 罚站一遍之后，10 分钟内再次全站搜索
// 直接跳过它们，秒级完成；站点恢复后会自动重新参与搜索。
const searchDeadSkipFor = 10 * time.Minute

// markSearchSiteDead 记录某站点这次全站搜索吃满了超时，短时内不再搜它。
func (s *ShellService) markSearchSiteDead(site string, until time.Time) {
	s.searchDeadMu.Lock()
	defer s.searchDeadMu.Unlock()
	if s.searchDead == nil {
		s.searchDead = make(map[string]time.Time)
	}
	if len(s.searchDead) >= 256 { // 偶尔清理过期的，别让列表无限膨胀
		for k, v := range s.searchDead {
			if time.Now().After(v) {
				delete(s.searchDead, k)
			}
		}
	}
	s.searchDead[site] = until
}

// loadSearchDeadSites 从 store 恢复上一次会话的超时站点名单。
// 桌面端重启频率高（换构建、重启应用都会丢内存），名单只活在内存里时，
// 每次重开后的第一次全站搜索都会把同一批死站点重新罚站一遍。
func (s *ShellService) loadSearchDeadSites() {
	if s.store == nil {
		return
	}
	raw, ok, err := s.store.GetKV(searchDeadSitesKey)
	if err != nil || !ok || raw == "" {
		return
	}
	var stored map[string]int64
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return
	}
	now := time.Now()
	alive := make(map[string]time.Time, len(stored))
	for site, until := range stored {
		if t := time.Unix(0, until); t.After(now) {
			alive[site] = t
		}
	}
	if len(alive) == 0 {
		return
	}
	s.searchDeadMu.Lock()
	if s.searchDead == nil {
		s.searchDead = make(map[string]time.Time, len(alive))
	}
	for site, until := range alive {
		s.searchDead[site] = until
	}
	s.searchDeadMu.Unlock()
}

// saveSearchDeadSites 把仍未过期的超时站点名单写回 store，供下次启动复用。
func (s *ShellService) saveSearchDeadSites() {
	if s.store == nil {
		return
	}
	now := time.Now()
	out := map[string]int64{}
	s.searchDeadMu.Lock()
	for site, until := range s.searchDead {
		if until.After(now) {
			out[site] = until.UnixNano()
		}
	}
	s.searchDeadMu.Unlock()
	if len(out) == 0 {
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return
	}
	if err := s.store.SetKV(searchDeadSitesKey, string(raw)); err != nil {
		log.Printf("保存超时站点名单失败: %v", err)
	}
}

// searchSiteSkipped 判断该站点当前是否在短时免除列表里（命中即删除过期项）。
func (s *ShellService) searchSiteSkipped(site string, now time.Time) bool {
	s.searchDeadMu.Lock()
	defer s.searchDeadMu.Unlock()
	until, ok := s.searchDead[site]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(s.searchDead, site)
	return false
}

// searchID 字段用 map 装 vods 时，站点 key 即 provider 标识。
// VodSearchAll 全站搜索点播，返回结果带所属站点（Site 字段）。
// 并发度由 SearchThreads 控制，进度经 search:progress 事件推给前端。
func (s *ShellService) VodSearchAll(q string) ([]VodItem, error) {
	return s.vodSearchAll(q, 0)
}

// VodSearchAllWithToken 全站搜索并透传前端请求 token，用于隔离重复搜索。
func (s *ShellService) VodSearchAllWithToken(q string, token uint64) ([]VodItem, error) {
	return s.vodSearchAll(q, token)
}

func (s *ShellService) vodSearchAll(q string, token uint64) ([]VodItem, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.searchCancel != nil {
		s.searchCancel()
	}
	s.searchSeq++
	searchID := s.searchSeq
	s.searchCancel = cancel
	s.mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit("search:start", SearchStartEvent{ID: searchID, Token: token, Query: q})
	}

	s.mu.RLock()
	type sp struct {
		site string
		pv   provider.Provider
	}
	providers := make([]sp, 0, len(s.vods))
	for k, pv := range s.vods {
		providers = append(providers, sp{site: k, pv: pv})
	}
	total := len(providers)
	s.mu.RUnlock()

	threads := s.SearchThreads()
	// 一次全站搜索有上千条进度事件，逐条推给前端会让 UI 追着事件跑，
	// 进度文本反而一顿一顿地跳。节流到 ~8 条/秒，结尾的「搜索完成」强制发。
	throttle := &progressThrottle{gap: searchProgressGap}
	emitProgress := func(p Progress) {
		if throttle.allow() {
			s.emitSearchProgress(searchID, token, q, p)
		}
	}
	// 线程数与站点数开搜就记日志：用户反馈「调大线程没用」时，
	// 一眼能确认这次到底用了几个线程、要搜多少个站点。
	log.Printf("全站搜索开始: %q，站点 %d，线程 %d", q, total, threads)

	sem := make(chan struct{}, threads)
	var mu sync.Mutex
	var out []VodItem
	var done atomic.Int64
	var inflight atomic.Int64
	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(site string, pv provider.Provider) {
			defer wg.Done()
			// 站点脚本是远端不可信代码，一次 panic（如 goja 对对象调 String）
			// 只能让该站点失败，不能让整个搜索/应用闪退。
			defer func() {
				if r := recover(); r != nil {
					log.Printf("全站搜索站点 %q 内部错误，已跳过: %v", site, r)
					n := int(done.Add(1))
					s.emitSearchProgress(searchID, token, q, Progress{Stage: "search", Message: fmt.Sprintf("%d/%d · %s 异常跳过", n, total, displaySiteName(site)), Done: n, Total: total})
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil { // 已取消：跳过该站点
				done.Add(1)
				return
			}
			if s.searchSiteSkipped(site, time.Now()) { // 上次超时，短时内不再白等
				n := int(done.Add(1))
				emitProgress(Progress{Stage: "search", Message: fmt.Sprintf("%d/%d · 跳过 %s（上次超时）", n, total, displaySiteName(site)), Done: n, Total: total})
				return
			}
			// 单站点超时：慢/死站点最多占一个线程 siteSearchTimeout，
			// 而不是等客户端 15s 超时后还重试一次。
			siteCtx, cancelSite := context.WithTimeout(ctx, siteSearchTimeout)
			defer cancelSite()
			// 进度里同时给出并发路数：只报「正在 <站点>」会让人以为在逐个串行搜。
			live := int(inflight.Add(1))
			defer inflight.Add(-1)
			n := int(done.Load())
			emitProgress(Progress{Stage: "search", Message: fmt.Sprintf("%d/%d · %d 线程 · %s", n, total, live, displaySiteName(site)), Done: n, Total: total})
			startSearch := time.Now()
			items, err := pv.Search(siteCtx, q)
			elapsed := time.Since(startSearch)
			if err == nil {
				v := toVodItems(items)
				for i := range v {
					v[i].Site = site
				}
				mu.Lock()
				out = append(out, v...)
				mu.Unlock()
				s.emitSearchResults(searchID, token, q, v) // 逐步推送结果
			}
			finished := int(done.Add(1))
			emitProgress(Progress{Stage: "search", Message: fmt.Sprintf("%d/%d · %d 线程", finished, total, live), Done: finished, Total: total})
			// 慢站点记日志：全站搜索的耗时基本都在这几台上。
			if elapsed > 2*time.Second {
				log.Printf("全站搜索站点 %q 耗时 %s（结果 %d 条）", site, elapsed.Round(time.Millisecond), len(items))
			}
			// 吃满超时的站点短时列入跳过名单：下一次全站搜索不再为它白等。
			if elapsed >= siteSearchTimeout {
				s.markSearchSiteDead(site, time.Now().Add(searchDeadSkipFor))
				log.Printf("全站搜索站点 %q 达到超时上限，%s 内跳过该站点", site, searchDeadSkipFor)
			}
		}(p.site, p.pv)
	}
	wg.Wait()
	// 名单落盘：应用重启后第一次全站搜索也能直接跳过这批死站点。
	s.saveSearchDeadSites()

	s.mu.Lock()
	if s.searchSeq == searchID {
		s.searchCancel = nil
	}
	s.mu.Unlock()
	s.emitSearchProgress(searchID, token, q, Progress{Stage: "search", Message: "搜索完成", Done: total, Total: total})
	return out, nil
}

// displaySiteName 去掉站点 key 里的技术前缀（drpy_js_ / csp_xml_ / lf_js_），
// 进度要显示在搜索框里，位置很窄，只留用户认得出的站名。
func displaySiteName(site string) string {
	parts := strings.Split(site, "_")
	i := 0
	for i < len(parts)-1 && isLowerASCII(parts[i]) {
		i++
	}
	if i == 0 {
		return site
	}
	return strings.Join(parts[i:], "_")
}

// isLowerASCII 判断一段前缀是不是纯小写 ASCII（drpy / js / csp / xml / lf）。
func isLowerASCII(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// searchProgressGap 是全站搜索进度事件的最小间隔。
const searchProgressGap = 120 * time.Millisecond

// progressThrottle 限制进度事件频率（多个站点 goroutine 共用一把锁）。
type progressThrottle struct {
	mu   sync.Mutex
	last time.Time
	gap  time.Duration
}

func (t *progressThrottle) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) < t.gap {
		return false
	}
	t.last = now
	return true
}

// cancelSearch 中断当前全站搜索（若有）。
func (s *ShellService) cancelSearch() {
	s.mu.Lock()
	cancel := s.searchCancel
	s.searchSeq++
	s.searchCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// CancelSearch 中断当前全站搜索（前端「取消」按钮）。
func (s *ShellService) CancelSearch() {
	s.cancelSearch()
}

// emitSearchResults 把一批搜索结果推给前端（渐进渲染）。
func (s *ShellService) emitSearchResults(id, token uint64, q string, items []VodItem) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("search:result", SearchResultEvent{ID: id, Token: token, Query: q, Items: items})
}

// SearchThreads 返回全站搜索的并发线程数（默认 1，上限 16）。
//
// 直接读内存：设置页改完即时生效，且不会因为一次数据库繁忙（SQLITE_BUSY）
// 就被静默降级成 1 —— 那样用户把线程调到 16，搜索仍然一个一个站点地跑。
func (s *ShellService) SearchThreads() int {
	s.searchThreadsMu.RLock()
	defer s.searchThreadsMu.RUnlock()
	return s.searchThreads
}

// loadSearchThreads 启动时把线程数设置读进内存；读失败保持默认并记日志，
// 不再让「读库失败」看起来像「用户就设了 1 线程」。
func (s *ShellService) loadSearchThreads() {
	n := 1
	if s.store != nil {
		v, ok, err := s.store.GetKV(searchThreadsKey)
		switch {
		case err != nil:
			log.Printf("读取搜索线程设置失败，暂用 %d 线程: %v", n, err)
		case ok:
			if parsed, perr := strconv.Atoi(v); perr == nil && parsed >= 1 {
				n = parsed
			}
		}
	}
	if n > maxSearchThreads {
		n = maxSearchThreads
	}
	s.searchThreadsMu.Lock()
	s.searchThreads = n
	s.searchThreadsMu.Unlock()
}

// SetSearchThreads 设置全站搜索并发线程数（1/4/8/16）。
// 先落到内存（本次运行立即生效），再写库（下次启动恢复）。
func (s *ShellService) SetSearchThreads(n int) error {
	if n < 1 {
		n = 1
	}
	if n > maxSearchThreads {
		n = maxSearchThreads
	}
	s.searchThreadsMu.Lock()
	s.searchThreads = n
	s.searchThreadsMu.Unlock()
	if s.store == nil {
		return nil
	}
	return s.store.SetKV(searchThreadsKey, strconv.Itoa(n))
}

// SearchCacheTTLMinutes 返回搜索结果缓存的 TTL（分钟），默认 30 分钟。
func (s *ShellService) SearchCacheTTLMinutes() int {
	if s.store == nil {
		return 30
	}
	v, ok, err := s.store.GetKV(searchCacheTTLKey)
	if err != nil || !ok {
		return 30
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 30
	}
	return n
}

// SetSearchCacheTTLMinutes 设置搜索结果缓存的 TTL（分钟）。
// 允许 30（半小时）/240（4 小时）/720（12 小时）/1440（24 小时）。
func (s *ShellService) SetSearchCacheTTLMinutes(n int) error {
	if s.store == nil {
		return nil
	}
	switch n {
	case 30, 240, 720, 1440:
	default:
		n = 30
	}
	return s.store.SetKV(searchCacheTTLKey, strconv.Itoa(n))
}

// CurrentVersion 返回当前应用版本（无需联网，供「关于」页面即时回显）。
func (s *ShellService) CurrentVersion() string {
	return appVersion
}

// InternalVersion 返回内部构建版本（debug.ReadBuildInfo 的 Main.Version）。
func (s *ShellService) InternalVersion() string {
	return internalVersion()
}

// LogError 把前端传来的错误信息写入日志缓冲，使 RuntimeError 也能在「查看日志」里看到。
func (s *ShellService) LogError(msg string) {
	log.Printf("%s", msg)
}

// CheckUpdate 查询 GitHub 最新 release，返回是否有新版本。
//
// 优先走 releases/latest 的网页重定向（不占 API 配额），失败才退回 API：
// 未登录的 GitHub API 每小时只有 60 次，被限流时只会 403，
// 用户看到的「检查更新失败: HTTP 403」基本都是这个原因。
func (s *ShellService) CheckUpdate() (UpdateInfo, error) {
	info := UpdateInfo{CurrentVersion: appVersion}
	client := &http.Client{Timeout: 10 * time.Second}

	tag, htmlURL, found, redirectErr := latestReleaseByRedirect(client, updatePageURL)
	if redirectErr != nil || !found {
		apiTag, apiHTML, apiFound, apiErr := latestReleaseByAPI(client, updateURL)
		if apiErr != nil {
			if redirectErr == nil {
				return info, apiErr
			}
			return info, fmt.Errorf("检查更新失败: %v（备用接口: %v）", redirectErr, apiErr)
		}
		tag, htmlURL, found = apiTag, apiHTML, apiFound
	}
	if !found {
		return info, nil // 尚无 release
	}
	info.LatestVersion = tag
	info.URL = htmlURL
	info.HasUpdate = compareVersion(tag, appVersion) > 0
	return info, nil
}

// latestReleaseByRedirect 顺着 /releases/latest 的 302 读 tag。
// 跳不到 /releases/tag/ 说明仓库还没有发布（found=false，不是错误）。
func latestReleaseByRedirect(client *http.Client, pageURL string) (tag, htmlURL string, found bool, err error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("User-Agent", "UnBox/"+strings.TrimSpace(appVersion))
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	final := resp.Request.URL
	const marker = "/releases/tag/"
	index := strings.Index(final.Path, marker)
	if index < 0 {
		return "", "", false, nil
	}
	tag = strings.TrimSuffix(final.Path[index+len(marker):], "/")
	if tag == "" {
		return "", "", false, nil
	}
	return tag, final.Scheme + "://" + final.Host + final.Path, true, nil
}

// latestReleaseByAPI 是备用路径：调用 GitHub API 读最新 release。
func latestReleaseByAPI(client *http.Client, apiURL string) (tag, htmlURL string, found bool, err error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "UnBox/"+strings.TrimSpace(appVersion))
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", "", false, nil // 尚无 release
	case http.StatusForbidden, http.StatusTooManyRequests:
		return "", "", false, errors.New("检查更新失败: GitHub 接口限流（未登录每小时 60 次），请稍后再试")
	default:
		return "", "", false, fmt.Errorf("检查更新失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", false, err
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", "", false, err
	}
	if rel.TagName == "" {
		return "", "", false, nil
	}
	return rel.TagName, rel.HTMLURL, true, nil
}

// compareVersion 简单语义化版本比较：a>b 返回 1，a<b 返回 -1，相等返回 0。
func compareVersion(a, b string) int {
	trim := func(s string) string {
		return strings.TrimPrefix(strings.TrimSpace(s), "v")
	}
	as := strings.Split(trim(a), ".")
	bs := strings.Split(trim(b), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, _ := strconv.Atoi(as[i])
		bi, _ := strconv.Atoi(bs[i])
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	if len(as) > len(bs) {
		return 1
	}
	if len(as) < len(bs) {
		return -1
	}
	return 0
}

// SetLastVodSite 记忆最后选择的点播站点。
func (s *ShellService) SetLastVodSite(site string) error {
	if s.store == nil {
		return nil
	}
	return s.store.SetKV("lastVodSite", site)
}

// LastVodSite 返回上次选择的点播站点（无则空串）。
func (s *ShellService) LastVodSite() (string, error) {
	if s.store == nil {
		return "", nil
	}
	v, _, err := s.store.GetKV("lastVodSite")
	return v, err
}

func (s *ShellService) VodDetail(site, id string) (VodMedia, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return VodMedia{}, err
	}
	m, err := pv.Detail(context.Background(), id)
	if err != nil {
		return VodMedia{}, err
	}
	vm := VodMedia{
		ID: m.ID, Title: m.Title, Logo: m.Logo, Group: m.Group,
		Description: m.Description, Year: m.Year, Area: m.Area,
		Type: m.Type, Remarks: m.Remarks, Sources: m.Sources,
	}
	vm.Episodes = make([]EpisodeInfo, len(m.Episodes))
	for i, e := range m.Episodes {
		vm.Episodes[i] = EpisodeInfo{ID: e.ID, Source: e.Source, Name: e.Name}
	}
	return vm, nil
}

func (s *ShellService) PlayVod(site, epID string) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	pv, err := s.vodOf(site)
	if err != nil {
		return err
	}
	st, err := pv.Resolve(context.Background(), epID)
	if err != nil {
		return err
	}
	if err := s.player.Load(context.Background(), st); err != nil {
		return err
	}
	return s.player.Play()
}

func (s *ShellService) PrepareVod(site, epID string) (playback.Plan, error) {
	return s.prepareVod(site, epID, 0)
}

// PreloadVod 为点播下一集准备资源：Web 流注册独立代理会话，其余流只做轻量
// 网络预热。它不触碰当前播放 token、播放器或播放状态，失败只返回预载错误。
func (s *ShellService) PreloadVod(site, epID string) (playback.Plan, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return playback.Plan{}, err
	}
	stream, err := pv.Resolve(context.Background(), epID)
	if err != nil {
		return playback.Plan{}, err
	}
	if s.playback == nil {
		return playback.Plan{}, errors.New("播放控制器未就绪")
	}
	return s.playback.Preload(context.Background(), stream)
}

// ReleasePreload 取消并释放一次预载任务；未知 id 视为已释放。
func (s *ShellService) ReleasePreload(id string) error {
	if s.playback == nil {
		return nil
	}
	return s.playback.Release(id)
}

// PrepareVodWithToken 准备点播播放，并让后端丢弃已过期的前端请求。
func (s *ShellService) PrepareVodWithToken(site, epID string, token uint64) (playback.Plan, error) {
	return s.prepareVod(site, epID, token)
}

func (s *ShellService) prepareVod(site, epID string, token uint64) (playback.Plan, error) {
	requestID, accepted := s.claimPlayback(token)
	if !accepted {
		return playback.Plan{}, nil
	}
	pv, err := s.vodOf(site)
	if err != nil {
		return playback.Plan{}, err
	}
	st, err := pv.Resolve(context.Background(), epID)
	if err != nil {
		log.Printf("解析点播剧集失败 site=%s ep=%s: %v", site, epID, err)
		return playback.Plan{}, err
	}
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if !s.playbackCurrent(token, requestID) {
		return playback.Plan{}, nil
	}
	plan, err := s.playback.Prepare(context.Background(), st)
	if err != nil {
		log.Printf("准备点播播放失败 site=%s ep=%s: %v", site, epID, err)
	}
	return plan, err
}

func (s *ShellService) FallbackToMPV(id string) (playback.Plan, error) {
	return s.fallbackToMPV(id, 0, 0)
}

// FallbackToMPVWithToken 执行网页播放失败后的 mpv 切换，并隔离过期请求。
// start 是网页播放器已看到的位置（秒），mpv 从此续播；0 表示从头起播。
func (s *ShellService) FallbackToMPVWithToken(id string, token uint64, start float64) (playback.Plan, error) {
	return s.fallbackToMPV(id, token, start)
}

func (s *ShellService) fallbackToMPV(id string, token uint64, start float64) (playback.Plan, error) {
	requestID, accepted := s.claimPlayback(token)
	if !accepted {
		return playback.Plan{}, nil
	}
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if !s.playbackCurrent(token, requestID) {
		return playback.Plan{}, nil
	}
	return s.playback.Fallback(context.Background(), id, start)
}

func (s *ShellService) claimPlayback(token uint64) (uint64, bool) {
	s.mu.Lock()
	if token == 0 {
		s.playbackSeq++
		requestID := s.playbackSeq
		if s.playbackToken > s.playbackFloor {
			s.playbackFloor = s.playbackToken
		}
		s.playbackToken = 0
		s.mu.Unlock()
		return requestID, true
	}
	if token <= s.playbackFloor || (s.playbackToken != 0 && token < s.playbackToken) {
		requestID := s.playbackSeq
		s.mu.Unlock()
		return requestID, false
	}
	if token > s.playbackToken {
		s.playbackSeq++
		s.playbackToken = token
	}
	requestID := s.playbackSeq
	if token > s.playbackFloor {
		s.playbackToken = token
	}
	s.mu.Unlock()
	return requestID, true
}

func (s *ShellService) playbackCurrent(token, requestID uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.playbackSeq == requestID && (token == 0 || (token > s.playbackFloor && s.playbackToken == token))
}

// StopPlayback 使指定 token 及之前的播放请求失效，再暂停共享播放器。
func (s *ShellService) StopPlayback(token uint64) error {
	requestID, invalidated := s.invalidatePlayback(token)
	if !invalidated {
		return nil
	}
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if !s.playbackCurrent(0, requestID) || s.player == nil {
		return nil
	}
	// 暂停是一次 IPC 往返：mpv 主循环忙（开流/缓冲/慢源）时要等到命令应答超时，
	// 前端已经不等它了，这里记一笔耗时，方便判断「切页慢」是不是它。
	start := time.Now()
	err := s.player.Pause()
	if d := time.Since(start); d > 500*time.Millisecond {
		log.Printf("暂停播放器耗时 %s（mpv 命令应答偏慢）", d.Round(time.Millisecond))
	}
	return err
}

func (s *ShellService) invalidatePlayback(token uint64) (uint64, bool) {
	s.mu.Lock()
	if token != 0 && s.playbackToken > token {
		requestID := s.playbackSeq
		s.mu.Unlock()
		return requestID, false
	}
	s.playbackSeq++
	requestID := s.playbackSeq
	if s.playbackToken > s.playbackFloor {
		s.playbackFloor = s.playbackToken
	}
	if token > s.playbackFloor {
		s.playbackFloor = token
	}
	s.playbackToken = 0
	s.mu.Unlock()
	return requestID, true
}

func (s *ShellService) MPVReady() bool { return s.playback.MPVReady() }

func (s *ShellService) MPVStatus() mpvplugin.Status { return s.mpvPlugin.Status() }

// SetTheme 持久化主题名；name 为空时清除主题（回到默认）。
func (s *ShellService) SetTheme(name string) error {
	if s.store == nil {
		return nil
	}
	if name == "" {
		return s.store.SetKV("theme", "")
	}
	return s.store.SetKV("theme", name)
}

// GetTheme 返回当前主题名；无设置时返回空串（默认主题）。
func (s *ShellService) GetTheme() (string, error) {
	if s.store == nil {
		return "", nil
	}
	v, _, err := s.store.GetKV("theme")
	return v, err
}

// VodSkipMarks 是一部剧的跳过片头片尾标记（秒）；0 表示未标记。
type VodSkipMarks struct {
	IntroEnd   float64
	OutroStart float64
}

// vodSkipKey 是跳过标记在 store.kv 里的键：按「站点 + 影片」共享，同剧各集通用。
func vodSkipKey(site, vodID string) string {
	return fmt.Sprintf("vod.skip.%s.%s", site, vodID)
}

// GetVodSkipMarks 读取跳过标记；缺失、非法或存储不可用时返回零值（不影响播放）。
func (s *ShellService) GetVodSkipMarks(site, vodID string) VodSkipMarks {
	if s.store == nil {
		return VodSkipMarks{}
	}
	raw, ok, err := s.store.GetKV(vodSkipKey(site, vodID))
	if err != nil || !ok {
		return VodSkipMarks{}
	}
	var marks VodSkipMarks
	if err := json.Unmarshal([]byte(raw), &marks); err != nil {
		return VodSkipMarks{}
	}
	return marks
}

// SetVodSkipMarks 持久化跳过标记。
func (s *ShellService) SetVodSkipMarks(site, vodID string, marks VodSkipMarks) error {
	if s.store == nil {
		return nil
	}
	raw, err := json.Marshal(marks)
	if err != nil {
		return err
	}
	return s.store.SetKV(vodSkipKey(site, vodID), string(raw))
}

// GetPlaybackSettings 返回点播播放自动化选项；缺失、非法或存储读取失败均按关闭处理。
func (s *ShellService) GetPlaybackSettings() PlaybackSettings {
	return PlaybackSettings{
		AutoNext:         s.getPlaybackSetting(playbackAutoNextKey),
		AutoSwitchSource: s.getPlaybackSetting(playbackAutoSwitchSourceKey),
		PreloadNext:      s.getPlaybackSetting(playbackPreloadNextKey),
	}
}

func (s *ShellService) getPlaybackSetting(key string) bool {
	if s.store == nil {
		return false
	}
	v, ok, err := s.store.GetKV(key)
	return err == nil && ok && v == "true"
}

// SetPlaybackSettings 持久化点播播放自动化选项。
func (s *ShellService) SetPlaybackSettings(settings PlaybackSettings) error {
	if s.store == nil {
		return nil
	}
	for _, setting := range []struct {
		key   string
		value bool
	}{
		{playbackAutoNextKey, settings.AutoNext},
		{playbackAutoSwitchSourceKey, settings.AutoSwitchSource},
		{playbackPreloadNextKey, settings.PreloadNext},
	} {
		if err := s.store.SetKV(setting.key, strconv.FormatBool(setting.value)); err != nil {
			return err
		}
	}
	return nil
}

// SetContentCardStyle 持久化内容卡片样式；仅接受 list/grid，非法值回退列表。
func (s *ShellService) SetContentCardStyle(style string) error {
	if s.store == nil {
		return nil
	}
	if style != "grid" {
		style = "list"
	}
	return s.store.SetKV(contentCardStyleKey, style)
}

// GetContentCardStyle 返回内容卡片样式；未设置或非法值默认列表。
func (s *ShellService) GetContentCardStyle() (string, error) {
	if s.store == nil {
		return "list", nil
	}
	v, _, err := s.store.GetKV(contentCardStyleKey)
	if err != nil {
		return "list", err
	}
	if v != "grid" {
		return "list", nil
	}
	return v, nil
}

func (s *ShellService) InstallMPV() (mpvplugin.InstallResult, error) {
	return s.mpvPlugin.Install(context.Background())
}

func (s *ShellService) RefreshMPV() (mpvplugin.Status, error) {
	status := s.mpvPlugin.Status()
	if !status.Available {
		return status, nil
	}
	p, err := s.mpvPlugin.NewPlayer()
	if err != nil {
		return status, err
	}
	if err := s.playback.SetMPV(p); err != nil {
		return status, err
	}
	s.player = p
	return status, nil
}

func toVodItems(items []provider.Item) []VodItem {
	out := make([]VodItem, len(items))
	for i, it := range items {
		out[i] = VodItem{ID: it.ID, Title: it.Title, Logo: it.Logo, Group: it.Group}
	}
	return out
}

// PlaybackEvent 是 mpv 播放器事件经 Wails 事件 "playback:event" 桥接给前端的载荷。
// Token 是发起本次播放的会话 token；前端只处理与当前点播会话 token 一致的事件。
type PlaybackEvent struct {
	Token    uint64
	Kind     string
	Position float64
	Error    string
}

// playbackEventFor 把播放器事件映射为带会话 token 的桥接载荷；纯函数，便于单测。
// 第二个返回值为 false 表示该事件类型不向前端转发。
func playbackEventFor(ev player.Event, token uint64) (PlaybackEvent, bool) {
	var kind string
	switch ev.Kind {
	case player.EventPlaying:
		kind = "playing"
	case player.EventBuffering:
		kind = "buffering"
	case player.EventPosition:
		kind = "position"
	case player.EventError:
		kind = "error"
	case player.EventEOF:
		kind = "ended"
	default:
		return PlaybackEvent{}, false
	}
	errMsg := ""
	if ev.Err != nil {
		errMsg = ev.Err.Error()
	}
	return PlaybackEvent{Token: token, Kind: kind, Position: ev.Position, Error: errMsg}, true
}

// currentPlaybackToken 返回当前点播会话 token；0 表示无会话。
// playbackToken 与 claimPlayback/playbackCurrent 一样由 s.mu 保护。
func (s *ShellService) currentPlaybackToken() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.playbackToken
}

// emitPlaybackEvent 是播放器事件桥接的默认出口：经 Wails "playback:event" 推给前端。
// application 尚未就绪时静默丢弃（服务可能在 app 初始化前启动，或测试环境无全局 app）。
func (s *ShellService) emitPlaybackEvent(ev PlaybackEvent) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("playback:event", ev)
}

// startPlaybackBridge 启动播放器事件桥接 goroutine：消费 player.Events()，
// 读取当前会话 token 后经 Wails "playback:event" 推给前端；无当前 token 时丢弃。
// 桥接异常不应影响手动播放，因此事件循环错误只记日志并继续。
func (s *ShellService) startPlaybackBridge() {
	if s.player == nil {
		return
	}
	emit := s.playbackEventEmitter
	if emit == nil {
		emit = s.emitPlaybackEvent
	}
	// 信号 channel 与出口都先取到局部变量再交给 goroutine，
	// 避免 goroutine 与 stop/注入方并发读写 ShellService 字段。
	stop := make(chan struct{})
	s.playbackBridgeMu.Lock()
	s.playbackBridgeStop = stop
	s.playbackBridgeMu.Unlock()
	go func() {
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-s.player.Events():
				if !ok {
					return
				}
				payload, ok := playbackEventFor(ev, s.currentPlaybackToken())
				if !ok || payload.Token == 0 {
					continue
				}
				emit(payload)
			}
		}
	}()
}

// stopPlaybackBridge 关闭播放器事件桥接 goroutine；未启动或已停止时为空操作。
func (s *ShellService) stopPlaybackBridge() {
	s.playbackBridgeMu.Lock()
	stop := s.playbackBridgeStop
	s.playbackBridgeStop = nil
	s.playbackBridgeMu.Unlock()
	if stop != nil {
		close(stop)
	}
}
