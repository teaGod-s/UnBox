package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/provider/tvbox"
	"github.com/unbox/unbox/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
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
type persistedSubscription struct {
	Ref      string           `json:"ref"`
	CFGs     []*config.Config `json:"cfgs,omitempty"`     // 配置导入：解析后的终端配置
	Channels []config.Channel `json:"channels,omitempty"` // 播放列表导入：已组装频道
}

// subscriptionKey 是 store.kv 里订阅快照的键。
const subscriptionKey = "subscription"

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
	return &ShellService{
		live:     pv,
		player:   p,
		store:    st,
		vods:     map[string]provider.Provider{},
		vodNames: map[string]string{},
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

// ImportSubscription 拉取并解析订阅，重建 Provider。支持两类输入：
//   - TVBox 订阅配置（JSON，含 lives/storeHouse/urls，可能多仓）
//   - 独立 M3U/TXT 播放列表（#EXTM3U 或「名称,URL」行）
func (s *ShellService) ImportSubscription(ref string) (ImportResult, error) {
	s.emitProgress(Progress{Stage: "resolve", Message: "正在解析订阅…", Done: -1, Total: -1})
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if isPlaylist(raw) {
		return s.importPlaylist(ref, raw)
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		s.emitProgress(Progress{Stage: "error", Message: "导入失败", Done: -1, Total: -1})
		return ImportResult{}, err
	}

	// 直播源不在此拉取（m3u 数量多、耗时），仅收集定义，前端经 LoadLive 按需加载。
	liveList := flattenLives(cfgs)
	vods, vodNames := collectVodSites(cfgs)
	s.mu.Lock()
	s.live = nil
	s.liveSources = liveList
	s.liveCount = 0
	s.vods = vods
	s.vodNames = vodNames
	s.mu.Unlock()

	// 持久化快照：重启后免重新导入。
	s.saveSubscription(ref, cfgs, nil)

	s.emitProgress(Progress{Stage: "done", Message: "导入完成", Done: -1, Total: -1})
	return ImportResult{Sites: len(vods), LiveSources: len(liveList)}, nil
}

// saveSubscription 把解析后的订阅快照写进 store（无网络）。store 为 nil 时静默跳过。
func (s *ShellService) saveSubscription(ref string, cfgs []*config.Config, channels []config.Channel) {
	if s.store == nil {
		return
	}
	b, err := json.Marshal(persistedSubscription{Ref: ref, CFGs: cfgs, Channels: channels})
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

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(sub.Channels) > 0 {
		s.live = live.New(sub.Channels)
		s.liveSources = nil
		s.liveCount = len(sub.Channels)
		s.vods = map[string]provider.Provider{}
		s.vodNames = map[string]string{}
		return ImportResult{Channels: len(sub.Channels)}, nil
	}
	s.live = nil
	s.liveSources = flattenLives(sub.CFGs)
	s.liveCount = 0
	s.vods, s.vodNames = collectVodSites(sub.CFGs)
	return ImportResult{Sites: len(s.vods), LiveSources: len(s.liveSources)}, nil
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

// collectVodSites 从多份配置收集 type=1（CMS）站点，构建 tvbox.Provider。
func collectVodSites(cfgs []*config.Config) (map[string]provider.Provider, map[string]string) {
	vods := make(map[string]provider.Provider)
	names := make(map[string]string)
	for _, cfg := range cfgs {
		for _, site := range cfg.Sites {
			if site.Type == config.SiteTypeCMS && site.API != "" {
				vods[site.Key] = tvbox.New(site)
				names[site.Key] = site.Name
			}
		}
	}
	return vods, names
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

// importPlaylist 解析独立 M3U/TXT 播放列表并重建 Provider。
func (s *ShellService) importPlaylist(ref string, raw []byte) (ImportResult, error) {
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
	s.vods = map[string]provider.Provider{}
	s.vodNames = map[string]string{}
	s.mu.Unlock()
	s.saveSubscription(ref, nil, channels)
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
		out = append(out, SourceInfo{ID: k, Name: s.vodNames[k], Kind: "vod"})
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

func toVodItems(items []provider.Item) []VodItem {
	out := make([]VodItem, len(items))
	for i, it := range items {
		out[i] = VodItem{ID: it.ID, Title: it.Title, Logo: it.Logo, Group: it.Group}
	}
	return out
}
