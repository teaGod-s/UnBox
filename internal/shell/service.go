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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unbox/unbox/internal/config"
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
// 播放列表导入时 Channels 立即可用；TVBox 配置导入时 Sites/LiveSources 非空，
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

// appVersion 是当前应用版本（与 GitHub release tag 对齐）。
// 本地/开发构建默认为 0.0.1；发布构建通过 -ldflags
// "-X github.com/unbox/unbox/internal/shell.appVersion=<version>" 注入真实版本。
var appVersion = "0.0.1"

// updateURL 是 GitHub 最新 release 的 API 地址（repo 改名后改这里）。
const updateURL = "https://api.github.com/repos/teaGod-s/UnBox/releases/latest"

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
	ID    string
	Title string
}

// VodItem 是点播影片列表项。
type VodItem struct {
	ID    string
	Title string
	Logo  string
	Group string
	Site  string // 所属站点 key（全站搜索时非空）
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

// NewShellService 组装壳层服务。pv 为直播 Provider（可为 nil）；p 可为 nil（播放器未就绪）。
func NewShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	root, _ := os.UserConfigDir()
	manager := mpvplugin.New(runtime.GOOS, root)
	controller := playback.NewController(playback.NewResolver(nil), playback.NewProxy(nil, 0), p)
	// Linux WebKitGTK 无 MSE 也不原生支持 HLS：hls.js/mpegts.js 用不了，
	// HLS/FLV/TS 必须走 mpv，只有 MP4 能由原生 <video> 播。其余平台 Web 能力默认为真。
	if runtime.GOOS == "linux" {
		controller.SetWebMSE(false)
	}
	return &ShellService{
		live:      pv,
		player:    p,
		store:     st,
		vods:      map[string]provider.Provider{},
		vodNames:  map[string]string{},
		playback:  controller,
		mpvPlugin: manager,
	}
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
func (s *ShellService) emitSearchProgress(p Progress) {
	app := application.Get()
	if app == nil {
		return
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	app.Event.Emit("search:progress", p)
}

// ImportSubscription 拉取并解析订阅，重建 Provider。支持两类输入：
//   - TVBox 订阅配置（JSON，含 lives/storeHouse/urls，可能多仓）
//   - 独立 M3U/TXT 播放列表（#EXTM3U 或「名称,URL」行）
func (s *ShellService) ImportSubscription(ref string) (ImportResult, error) {
	s.emitProgress(Progress{Stage: "resolve", Message: "正在解析订阅…", Done: -1, Total: -1})
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		log.Printf("导入订阅拉取失败 %s: %v", ref, err)
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
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
	return s.store.DeleteSource(kind, ref)
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
	}
	s.mu.Unlock()
	s.addSource("live", ref)
	s.saveSubscription()
	return ImportResult{Channels: len(channels)}, nil
}

// resolveConfigs 解析 TVBox 配置；索引节点（storeHouse/urls 非空）交给 Resolver 展开。
func resolveConfigs(ctx context.Context, ref string, raw []byte) ([]*config.Config, error) {
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0 {
		cfgs, rerr := config.NewResolver().Resolve(ctx, ref)
		if rerr != nil && len(cfgs) == 0 {
			return nil, fmt.Errorf("展开订阅失败: %w", rerr)
		}
		return cfgs, nil
	}
	return []*config.Config{cfg}, nil
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
		out[i] = Section{ID: sc.ID, Title: sc.Title}
	}
	return out, nil
}

func (s *ShellService) VodList(site, cat string, page int) ([]VodItem, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	pg, err := pv.Browse(context.Background(), cat, page)
	if err != nil {
		return nil, err
	}
	return toVodItems(pg.Items), nil
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

// VodSearchAll 全站搜索点播，返回结果带所属站点（Site 字段）。
// 并发度由 SearchThreads 控制，进度经 search:progress 事件推给前端。
func (s *ShellService) VodSearchAll(q string) ([]VodItem, error) {
	s.cancelSearch() // 取消上一次搜索
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.searchCancel = cancel
	s.mu.Unlock()

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

	sem := make(chan struct{}, s.SearchThreads())
	var mu sync.Mutex
	var out []VodItem
	var done atomic.Int64
	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(site string, pv provider.Provider) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil { // 已取消：跳过该站点
				done.Add(1)
				return
			}
			items, err := pv.Search(ctx, q)
			if err == nil {
				v := toVodItems(items)
				for i := range v {
					v[i].Site = site
				}
				mu.Lock()
				out = append(out, v...)
				mu.Unlock()
				s.emitSearchResults(v) // 逐步推送结果
			}
			n := int(done.Add(1))
			s.emitSearchProgress(Progress{Stage: "search", Message: fmt.Sprintf("全站搜索 %d/%d", n, total), Done: n, Total: total})
		}(p.site, p.pv)
	}
	wg.Wait()

	s.mu.Lock()
	s.searchCancel = nil
	s.mu.Unlock()
	s.emitSearchProgress(Progress{Stage: "search", Message: "搜索完成", Done: total, Total: total})
	return out, nil
}

// cancelSearch 中断当前全站搜索（若有）。
func (s *ShellService) cancelSearch() {
	s.mu.Lock()
	cancel := s.searchCancel
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
func (s *ShellService) emitSearchResults(items []VodItem) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("search:result", items)
}

// SearchThreads 返回全站搜索的并发线程数（默认 1，上限 16）。
func (s *ShellService) SearchThreads() int {
	if s.store == nil {
		return 1
	}
	v, ok, err := s.store.GetKV(searchThreadsKey)
	if err != nil || !ok {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	if n > 16 {
		n = 16
	}
	return n
}

// SetSearchThreads 设置全站搜索并发线程数（1/4/8/16）。
func (s *ShellService) SetSearchThreads(n int) error {
	if s.store == nil {
		return nil
	}
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	return s.store.SetKV(searchThreadsKey, strconv.Itoa(n))
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
func (s *ShellService) CheckUpdate() (UpdateInfo, error) {
	info := UpdateInfo{CurrentVersion: appVersion}
	req, err := http.NewRequest(http.MethodGet, updateURL, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "UnBox")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return info, nil // 尚无 release
	}
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("检查更新失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return info, err
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return info, err
	}
	info.LatestVersion = rel.TagName
	info.URL = rel.HTMLURL
	info.HasUpdate = compareVersion(rel.TagName, appVersion) > 0
	return info, nil
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
	pv, err := s.vodOf(site)
	if err != nil {
		return playback.Plan{}, err
	}
	st, err := pv.Resolve(context.Background(), epID)
	if err != nil {
		log.Printf("解析点播剧集失败 site=%s ep=%s: %v", site, epID, err)
		return playback.Plan{}, err
	}
	plan, err := s.playback.Prepare(context.Background(), st)
	if err != nil {
		log.Printf("准备点播播放失败 site=%s ep=%s: %v", site, epID, err)
	}
	return plan, err
}

func (s *ShellService) FallbackToMPV(id string) (playback.Plan, error) {
	return s.playback.Fallback(context.Background(), id)
}

func (s *ShellService) MPVReady() bool { return s.playback.MPVReady() }

func (s *ShellService) MPVStatus() mpvplugin.Status { return s.mpvPlugin.Status() }

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
