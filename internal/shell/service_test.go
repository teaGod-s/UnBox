package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/library"
	"github.com/unbox/unbox/internal/library/thumb"
	"github.com/unbox/unbox/internal/playback"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/provider/tvbox"
	"github.com/unbox/unbox/internal/store"
)

func newTestService(t *testing.T) *ShellService {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	channels := []config.Channel{
		{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}},
	}
	svc := NewShellService(live.New(channels), nil, s)
	t.Cleanup(func() { s.Close() })
	return svc
}

func newPlaybackSettingsService(t *testing.T, st *store.Store) *ShellService {
	t.Helper()
	svc := NewShellService(live.New(nil), nil, st)
	t.Cleanup(func() { _ = svc.ServiceShutdown() })
	return svc
}

func TestPlaybackSettingsRoundTripAndDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := newPlaybackSettingsService(t, st)
	if got := svc.GetPlaybackSettings(); got != (PlaybackSettings{}) {
		t.Fatalf("missing keys = %#v, want all false", got)
	}
	want := PlaybackSettings{AutoNext: true, AutoSwitchSource: true, PreloadNext: false}
	if err := svc.SetPlaybackSettings(want); err != nil {
		t.Fatal(err)
	}
	svc2 := newPlaybackSettingsService(t, st)
	if got := svc2.GetPlaybackSettings(); got != want {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestPlaybackSettingsInvalidValuesFallBackToFalse(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SetKV(playbackAutoNextKey, "not-a-bool"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKV(playbackAutoSwitchSourceKey, "true"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKV(playbackPreloadNextKey, "false"); err != nil {
		t.Fatal(err)
	}

	got := newPlaybackSettingsService(t, st).GetPlaybackSettings()
	want := PlaybackSettings{AutoSwitchSource: true}
	if got != want {
		t.Fatalf("invalid values = %#v, want %#v", got, want)
	}
}

func TestPlaybackSettingsStoreErrors(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/playback-settings.db")
	if err != nil {
		t.Fatal(err)
	}
	svc := newPlaybackSettingsService(t, st)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if got := svc.GetPlaybackSettings(); got != (PlaybackSettings{}) {
		t.Fatalf("closed store read = %#v, want all false", got)
	}
	if err := svc.SetPlaybackSettings(PlaybackSettings{AutoNext: true}); err == nil {
		t.Fatal("closed store write should return an error")
	}
}

func TestLibraryDirRoundTrip(t *testing.T) {
	svc := newTestService(t)
	dir := t.TempDir()
	if err := svc.AddLibraryDir(dir); err != nil {
		t.Fatal(err)
	}
	dirs, err := svc.ListLibraryDirs()
	if err != nil || len(dirs) != 1 || dirs[0].Path != dir {
		t.Fatalf("dirs=%+v err=%v", dirs, err)
	}
	if err := svc.RemoveLibraryDir(dir); err != nil {
		t.Fatal(err)
	}
	dirs, err = svc.ListLibraryDirs()
	if err != nil || len(dirs) != 0 {
		t.Fatalf("remove dirs=%+v err=%v", dirs, err)
	}
}

func TestLibraryBindingsScanListAndProgress(t *testing.T) {
	svc := newTestService(t)
	root := t.TempDir()
	path := filepath.Join(root, "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddLibraryDir(root); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ScanLibrary()
	if err != nil || result.Added != 1 {
		t.Fatalf("scan=%+v err=%v", result, err)
	}
	items, err := svc.ListLibrary()
	if err != nil || len(items) != 1 || items[0].Path != path {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if err := svc.RecordLibraryProgress(path, 12, 90); err != nil {
		t.Fatal(err)
	}
	history, err := svc.ListVodHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Site != "local" || history[0].VodID != path || history[0].Progress != 12 {
		t.Fatalf("history=%+v", history)
	}
}

func TestPrepareLibraryRejectsUnregisteredPath(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.PrepareLibrary(filepath.Join(t.TempDir(), "outside.mp4")); err == nil {
		t.Fatal("未注册目录中的本地媒体应被拒绝")
	}
}

func TestPrepareLibraryRequiresStore(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	if _, err := svc.PrepareLibrary("/tmp/movie.mp4"); err == nil {
		t.Fatal("无媒体库存储时不应允许本地播放")
	}
}

func TestLibraryThumbBindings(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.library = library.New(nil, filepath.Join(t.TempDir(), "posters"), nil)
	svc.mpvPlugin = nil
	path := filepath.Join(t.TempDir(), "movie.mp4")
	videoURL, posterURL, cached, err := svc.EnsureThumb(path, 1)
	if err != nil || cached || videoURL == "" || posterURL == "" {
		t.Fatalf("EnsureThumb = %q, %q, %v, %v", videoURL, posterURL, cached, err)
	}
	if _, err := svc.SaveThumb(path, 1, []byte{0xff, 0xd8, 0xff, 0xd9}); err != nil {
		t.Fatal(err)
	}
	if _, _, cached, err = svc.EnsureThumb(path, 1); err != nil || !cached {
		t.Fatalf("缓存后的 EnsureThumb cached=%v err=%v", cached, err)
	}
	if _, err := svc.GenerateThumbMpv(path, 1); !errors.Is(err, thumb.ErrNoMpv) {
		t.Fatalf("无 mpv 时应透传 ErrNoMpv, got %v", err)
	}
}

func TestThemeRoundTrip(t *testing.T) {
	svc := newTestService(t)
	theme, err := svc.GetTheme()
	if err != nil || theme != "" {
		t.Fatalf("默认主题应为空串, got %q, %v", theme, err)
	}
	if err := svc.SetTheme("aurora"); err != nil {
		t.Fatal(err)
	}
	theme, err = svc.GetTheme()
	if err != nil || theme != "aurora" {
		t.Fatalf("主题回读失败, got %q, %v", theme, err)
	}
}

func TestThemeEmptyService(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	if err := svc.SetTheme("aurora"); err != nil {
		t.Fatal(err)
	}
	theme, err := svc.GetTheme()
	if err != nil || theme != "" {
		t.Fatalf("无 store 应安全返回空串, got %q, %v", theme, err)
	}
}

func TestContentCardStyleRoundTrip(t *testing.T) {
	svc := newTestService(t)
	style, err := svc.GetContentCardStyle()
	if err != nil || style != "list" {
		t.Fatalf("默认内容卡片样式应为 list, got %q, %v", style, err)
	}
	if err := svc.SetContentCardStyle("grid"); err != nil {
		t.Fatal(err)
	}
	style, err = svc.GetContentCardStyle()
	if err != nil || style != "grid" {
		t.Fatalf("内容卡片样式回读失败, got %q, %v", style, err)
	}
}

func TestContentCardStyleInvalidValueFallsBackToList(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetContentCardStyle("cards"); err != nil {
		t.Fatal(err)
	}
	style, err := svc.GetContentCardStyle()
	if err != nil || style != "list" {
		t.Fatalf("非法内容卡片样式应回退 list, got %q, %v", style, err)
	}
}

func TestImportSubscriptionPlaylist(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, err := svc.ImportSubscription(path)
	if err != nil || r.Channels != 1 {
		t.Fatalf("ImportSubscription = %+v, %v", r, err)
	}
	gs, _ := svc.Groups()
	if len(gs) != 1 || gs[0] != "测试" {
		t.Fatalf("Groups = %v", gs)
	}
}

func TestGroupsAndChannels(t *testing.T) {
	svc := newTestService(t)
	gs, err := svc.Groups()
	if err != nil || len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("Groups = %v, %v", gs, err)
	}
	chs, err := svc.Channels("央视", 0)
	if err != nil || len(chs) != 1 || chs[0].Name != "CCTV-1" {
		t.Fatalf("Channels = %+v, %v", chs, err)
	}
}

func TestFavoriteRoundtrip(t *testing.T) {
	svc := newTestService(t)
	id := "央视/CCTV-1"
	if err := svc.AddFavorite(id); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	favs, err := svc.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
}

func TestPlayChannelRequiresPlayer(t *testing.T) {
	svc := newTestService(t)
	if err := svc.PlayChannel("央视/CCTV-1"); err == nil {
		t.Fatal("player 为 nil 时 PlayChannel 应报错")
	}
}

func TestGroupsBeforeImport(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	svc := NewShellService(nil, nil, s)
	gs, err := svc.Groups()
	if err != nil || len(gs) != 0 {
		t.Fatalf("导入前 Groups 应返回空且不报错: %v, %v", gs, err)
	}
}

func TestChannelsNilStore(t *testing.T) {
	channels := []config.Channel{
		{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}},
	}
	svc := NewShellService(live.New(channels), nil, nil)
	chs, err := svc.Channels("央视", 0)
	if err != nil || len(chs) != 1 || chs[0].Favorited {
		t.Fatalf("store 为 nil 时 Channels 应返回频道且 Favorited=false: %+v, %v", chs, err)
	}
}

func TestConcurrentImportAndGroups(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.ImportSubscription(path) }()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.Groups() }()
	}
	wg.Wait()
}

// stubProvider 是点播 Provider 的最小测试桩。
type stubProvider struct{ key string }

func (s *stubProvider) ID() string { return s.key }
func (s *stubProvider) Home(context.Context) ([]provider.Section, error) {
	return []provider.Section{{ID: "10", Title: "电影"}}, nil
}
func (s *stubProvider) Browse(context.Context, string, int) (provider.Page, error) {
	return provider.Page{Items: []provider.Item{{ID: "1", Title: "电影A"}}}, nil
}
func (s *stubProvider) Search(context.Context, string) ([]provider.Item, error) { return nil, nil }
func (s *stubProvider) Detail(context.Context, string) (provider.Media, error) {
	return provider.Media{ID: "1", Title: "电影A", Sources: []string{"x"}, Episodes: []provider.Episode{{ID: "1/0/0", Source: "x", Name: "第01集"}}}, nil
}
func (s *stubProvider) Resolve(context.Context, string) (player.Stream, error) {
	return player.Stream{URL: "https://x/a.m3u8", Kind: player.StreamHLS}, nil
}

type countingVodProvider struct {
	stubProvider
	homeCalls int
}

func (p *countingVodProvider) Home(context.Context) ([]provider.Section, error) {
	p.homeCalls++
	return []provider.Section{{ID: "10", Title: "电影"}}, nil
}

func TestVodPersistenceAPIs(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RecordVodSearch("斗罗"); err != nil {
		t.Fatalf("RecordVodSearch: %v", err)
	}
	history, err := svc.ListVodSearchHistory()
	if err != nil || len(history) != 1 || history[0] != "斗罗" {
		t.Fatalf("ListVodSearchHistory = %#v, %v", history, err)
	}
	if err := svc.DeleteVodSearchHistory("斗罗"); err != nil {
		t.Fatalf("DeleteVodSearchHistory: %v", err)
	}
	if err := svc.AddVodFavorite("site-a", "vod-1", "影片一", "logo", "电影"); err != nil {
		t.Fatalf("AddVodFavorite: %v", err)
	}
	ok, err := svc.IsVodFavorite("site-a", "vod-1")
	if err != nil || !ok {
		t.Fatalf("IsVodFavorite = %v, %v", ok, err)
	}
	favs, err := svc.ListVodFavorites()
	if err != nil || len(favs) != 1 || favs[0].Title != "影片一" {
		t.Fatalf("ListVodFavorites = %#v, %v", favs, err)
	}
	if err := svc.RemoveVodFavorite("site-a", "vod-1"); err != nil {
		t.Fatalf("RemoveVodFavorite: %v", err)
	}
	if err := svc.RecordVodHistory("site-a", "vod-1", "影片一", "", "ep-1", "第1集", "线路"); err != nil {
		t.Fatalf("RecordVodHistory: %v", err)
	}
	// GetVodHistory：查到刚记录的条目，关键字段回显正确。
	got, err := svc.GetVodHistory("site-a", "vod-1")
	if err != nil {
		t.Fatalf("GetVodHistory: %v", err)
	}
	if got.VodID != "vod-1" || got.EpID != "ep-1" || got.Source != "线路" {
		t.Fatalf("GetVodHistory = %+v, want vod-1/ep-1/线路", got)
	}
	// 不存在的条目返回零值（VodID 为空），不报错。
	missing, err := svc.GetVodHistory("site-a", "missing")
	if err != nil || missing.VodID != "" {
		t.Fatalf("GetVodHistory(missing) = %+v, %v, want zero VodHistoryInfo", missing, err)
	}
	if err := svc.DeleteVodHistory("site-a", "vod-1"); err != nil {
		t.Fatalf("DeleteVodHistory: %v", err)
	}
	// 删除后再查同样返回零值。
	after, err := svc.GetVodHistory("site-a", "vod-1")
	if err != nil || after.VodID != "" {
		t.Fatalf("GetVodHistory after delete = %+v, %v, want zero VodHistoryInfo", after, err)
	}
}

func TestVodCategoriesUsesShortTTLCache(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	provider := &countingVodProvider{stubProvider: stubProvider{key: "s1"}}
	svc.vods["s1"] = provider
	svc.vodCategoryNow = func() time.Time { return time.Unix(100, 0) }
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("first VodCategories: %v", err)
	}
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("cached VodCategories: %v", err)
	}
	if provider.homeCalls != 1 {
		t.Fatalf("cache hit calls Home %d times", provider.homeCalls)
	}
	svc.vodCategoryNow = func() time.Time { return time.Unix(106, 0) }
	if _, err := svc.VodCategories("s1"); err != nil {
		t.Fatalf("expired VodCategories: %v", err)
	}
	if provider.homeCalls != 2 {
		t.Fatalf("expired cache calls Home %d times", provider.homeCalls)
	}
}

func TestVodSourcesAndRoutes(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods["s1"] = &stubProvider{key: "s1"}
	svc.vodNames["s1"] = "站点一"

	srcs := svc.Sources()
	if len(srcs) != 2 || srcs[0].ID != "live" || srcs[1].ID != "s1" || srcs[1].Name != "站点一" {
		t.Fatalf("Sources 错误: %+v", srcs)
	}
	secs, err := svc.VodCategories("s1")
	if err != nil || len(secs) != 1 || secs[0].Title != "电影" {
		t.Fatalf("VodCategories 错误: %+v %v", secs, err)
	}
	m, err := svc.VodDetail("s1", "1")
	if err != nil || len(m.Episodes) != 1 || m.Sources[0] != "x" {
		t.Fatalf("VodDetail 错误: %+v %v", m, err)
	}
	if _, err := svc.VodCategories("nope"); err == nil {
		t.Fatalf("未知站点应报错")
	}
}

func writeTempM3U(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadLiveLazy(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	m3u := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"央视\",频道1\nhttp://x/1\n")
	svc.liveSources = []config.Live{{Name: "g1", URL: m3u}}

	// 未加载时 Groups 为空
	gs, _ := svc.Groups()
	if len(gs) != 0 {
		t.Fatalf("未加载直播时 Groups 应为空: %v", gs)
	}

	n, err := svc.LoadLive()
	if err != nil || n != 1 {
		t.Fatalf("LoadLive = %d, %v", n, err)
	}
	gs, _ = svc.Groups()
	if len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("加载后 Groups = %v", gs)
	}

	// 幂等：再次 LoadLive 直接返回
	n2, err := svc.LoadLive()
	if err != nil || n2 != 1 {
		t.Fatalf("重复 LoadLive = %d, %v", n2, err)
	}
}

func TestCollectChannelsParallelOrder(t *testing.T) {
	m3u1 := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"央视\",频道1\nhttp://x/1\n#EXTINF:-1 group-title=\"央视\",频道2\nhttp://x/2\n")
	m3u2 := writeTempM3U(t, "#EXTM3U\n#EXTINF:-1 group-title=\"卫视\",频道3\nhttp://x/3\n")

	lives := []config.Live{
		{Name: "g1", URL: m3u1},
		{Name: "g2", Channels: []config.Channel{{Name: "内嵌", Group: "G", URLs: []string{"http://x/0"}}}},
		{Name: "g3", URL: m3u2},
	}
	chs := collectChannels(context.Background(), lives, nil)
	if len(chs) != 4 {
		t.Fatalf("频道数 = %d, 期望 4", len(chs))
	}
	var names []string
	for _, c := range chs {
		names = append(names, c.Name)
	}
	want := []string{"频道1", "频道2", "内嵌", "频道3"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("顺序错误: 得 %v, 期望 %v", names, want)
		}
	}
}

func TestRestoreSubscriptionConfig(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	cfgs := []*config.Config{{
		Sites: []config.Site{{Key: "s1", Name: "站点一", Type: config.SiteTypeCMS, API: "http://x/api.php"}},
		Lives: config.LiveList{{Name: "g1", URL: "http://x/g1.m3u"}},
	}}
	svc1 := NewShellService(nil, nil, s)
	svc1.vodCFGs = cfgs
	svc1.liveCFGs = cfgs
	svc1.saveSubscription()

	svc2 := NewShellService(nil, nil, s)
	r, err := svc2.RestoreSubscription()
	if err != nil || r.Sites != 1 || r.LiveSources != 1 {
		t.Fatalf("RestoreSubscription = %+v, %v", r, err)
	}
	if _, ok := svc2.vods["s1"]; !ok {
		t.Fatalf("恢复后应有站点 s1")
	}
	if len(svc2.liveSources) != 1 {
		t.Fatalf("恢复后应有 1 个直播源")
	}
}

func TestRestoreSubscriptionNoSnapshot(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	svc := NewShellService(nil, nil, s)
	r, err := svc.RestoreSubscription()
	if err != nil || r != (ImportResult{}) {
		t.Fatalf("无快照应返回零值且不报错: %+v, %v", r, err)
	}
}

func TestRestoreSubscriptionPlaylist(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	chans := []config.Channel{{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}}}
	svc1 := NewShellService(nil, nil, s)
	svc1.liveChannels = chans
	svc1.saveSubscription()

	svc2 := NewShellService(nil, nil, s)
	r, err := svc2.RestoreSubscription()
	if err != nil || r.Channels != 1 {
		t.Fatalf("RestoreSubscription = %+v, %v", r, err)
	}
	gs, _ := svc2.Groups()
	if len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("恢复后 Groups = %v", gs)
	}
}

func TestCollectVodSitesSpider(t *testing.T) {
	cfgs := []*config.Config{{
		Sites: []config.Site{
			{Key: "cms", Name: "CMS站", Type: config.SiteTypeCMS, API: "http://x/api.php"},
			{Key: "sp", Name: "爬虫站", Type: config.SiteTypeSpider, API: "http://x:5757"},
			{Key: "js", Name: "JS站", Type: config.SiteTypeSpider, API: "https://example.com/spider.js"},
			{Key: "jar", Name: "JAR站", Type: config.SiteTypeSpider, API: "csp_xxx"},
		},
	}}
	vods, names, _ := collectVodSites(cfgs)
	if len(vods) != 3 {
		t.Fatalf("应有 3 个站点（cms + spider http + js），得 %d", len(vods))
	}
	if names["sp"] != "爬虫站" {
		t.Fatalf("names = %v", names)
	}
	if _, ok := vods["jar"]; ok {
		t.Fatalf("csp_ JAR 站点不应被收集")
	}
	if _, ok := vods["js"].(*tvbox.Spider); !ok {
		t.Fatalf("js 站点应使用 tvbox.Spider，实际类型 %T", vods["js"])
	}
}

func TestPlaybackTokenRejectsStaleRequests(t *testing.T) {
	svc := &ShellService{}
	firstID, ok := svc.claimPlayback(1)
	if !ok || !svc.playbackCurrent(1, firstID) {
		t.Fatalf("first playback token should be current: id=%d ok=%v", firstID, ok)
	}
	stopID, ok := svc.claimPlayback(0)
	if !ok || !svc.playbackCurrent(0, stopID) {
		t.Fatalf("stop token should invalidate active playback: id=%d ok=%v", stopID, ok)
	}
	if _, ok := svc.claimPlayback(1); ok {
		t.Fatal("stale token should not reclaim playback after stop")
	}
	secondID, ok := svc.claimPlayback(2)
	if !ok || !svc.playbackCurrent(2, secondID) {
		t.Fatalf("newer playback token should be accepted: id=%d ok=%v", secondID, ok)
	}
	if _, ok := svc.claimPlayback(1); ok {
		t.Fatal("older token should not reclaim newer playback")
	}
}

func TestStopPlaybackDoesNotInvalidateNewerToken(t *testing.T) {
	svc := &ShellService{}
	if _, ok := svc.claimPlayback(1); !ok {
		t.Fatal("first playback token should be accepted")
	}
	if _, ok := svc.claimPlayback(2); !ok {
		t.Fatal("newer playback token should be accepted")
	}
	if _, invalidated := svc.invalidatePlayback(1); invalidated {
		t.Fatal("stale stop must not invalidate a newer playback token")
	}
	if svc.playbackToken != 2 {
		t.Fatalf("newer playback token was cleared: got %d", svc.playbackToken)
	}
}

func TestPlaybackEventMapping(t *testing.T) {
	cases := []struct {
		name string
		ev   player.Event
		tok  uint64
		want PlaybackEvent
	}{
		{"playing", player.Event{Kind: player.EventPlaying}, 1,
			PlaybackEvent{Token: 1, Kind: "playing"}},
		{"buffering", player.Event{Kind: player.EventBuffering}, 1,
			PlaybackEvent{Token: 1, Kind: "buffering"}},
		{"position", player.Event{Kind: player.EventPosition, Position: 8.25}, 17,
			PlaybackEvent{Token: 17, Kind: "position", Position: 8.25}},
		{"error", player.Event{Kind: player.EventError, Err: errors.New("boom")}, 3,
			PlaybackEvent{Token: 3, Kind: "error", Error: "boom"}},
		{"ended", player.Event{Kind: player.EventEOF}, 4,
			PlaybackEvent{Token: 4, Kind: "ended"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := playbackEventFor(tc.ev, tc.tok)
			if !ok {
				t.Fatalf("%s: 期望事件被转发", tc.name)
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
	if _, ok := playbackEventFor(player.Event{Kind: player.EventKind(99)}, 1); ok {
		t.Fatal("未知事件类型不应转发给前端")
	}
}

// bridgeTestPlayer 是桥接循环测试用的可编程 player：Events() 返回注入通道。
type bridgeTestPlayer struct {
	events chan player.Event
}

func (p *bridgeTestPlayer) Load(ctx context.Context, s player.Stream) error { return nil }
func (p *bridgeTestPlayer) Play() error                                     { return nil }
func (p *bridgeTestPlayer) Pause() error                                    { return nil }
func (p *bridgeTestPlayer) Seek(float64) error                              { return nil }
func (p *bridgeTestPlayer) SetVolume(int) error                             { return nil }
func (p *bridgeTestPlayer) SelectTrack(player.TrackKind, int) error         { return nil }
func (p *bridgeTestPlayer) State() player.State                             { return player.State{} }
func (p *bridgeTestPlayer) Events() <-chan player.Event                     { return p.events }
func (p *bridgeTestPlayer) Close() error                                    { return nil }

func newBridgeTestPlayer() *bridgeTestPlayer {
	return &bridgeTestPlayer{events: make(chan player.Event, 16)}
}

func TestPlaybackBridgeFiltersTokenAndStops(t *testing.T) {
	inner := newBridgeTestPlayer()
	// 先注入事件出口再启动桥接，避免与桥接 goroutine 并发写字段。
	svc := newShellService(nil, inner, nil)

	var mu sync.Mutex
	var got []PlaybackEvent
	svc.playbackEventEmitter = func(ev PlaybackEvent) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}
	svc.startPlaybackBridge()
	defer func() { _ = svc.ServiceShutdown() }()

	// 无会话 token 时事件应被丢弃。
	inner.events <- player.Event{Kind: player.EventPlaying}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if n := len(got); n != 0 {
		mu.Unlock()
		t.Fatalf("无 token 时收到 %d 个事件，期望 0", n)
	}
	mu.Unlock()

	// 建立会话后事件应带当前 token 发出。
	if _, ok := svc.claimPlayback(7); !ok {
		t.Fatal("claimPlayback 应接受首个 token")
	}
	inner.events <- player.Event{Kind: player.EventPosition, Position: 8.25}
	waitForBridgeEvents(t, &mu, &got, 1)
	mu.Lock()
	if len(got) != 1 || got[0] != (PlaybackEvent{Token: 7, Kind: "position", Position: 8.25}) {
		mu.Unlock()
		t.Fatalf("got %#v，期望 {Token:7 Kind:position Position:8.25}", got)
	}
	mu.Unlock()

	// shutdown 后桥接应停止：后续事件不再被转发。
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	inner.events <- player.Event{Kind: player.EventEOF}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if n := len(got); n != 1 {
		mu.Unlock()
		t.Fatalf("shutdown 后仍收到事件（共 %d）", n)
	}
	mu.Unlock()
}

func waitForBridgeEvents(t *testing.T, mu *sync.Mutex, got *[]PlaybackEvent, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(*got) >= n {
			mu.Unlock()
			return
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待桥接事件超时")
}

// preloadTestProvider 是点播预载测试用的最小 Provider。
type preloadTestProvider struct {
	stream player.Stream
	calls  int
}

func (p *preloadTestProvider) ID() string { return "preload-test" }
func (p *preloadTestProvider) Home(context.Context) ([]provider.Section, error) {
	return nil, nil
}
func (p *preloadTestProvider) Browse(context.Context, string, int) (provider.Page, error) {
	return provider.Page{}, nil
}
func (p *preloadTestProvider) Search(context.Context, string) ([]provider.Item, error) {
	return nil, nil
}
func (p *preloadTestProvider) Detail(context.Context, string) (provider.Media, error) {
	return provider.Media{}, nil
}
func (p *preloadTestProvider) Resolve(context.Context, string) (player.Stream, error) {
	p.calls++
	return p.stream, nil
}

func TestPreloadVodRegistersPreloadWithoutTouchingPlayback(t *testing.T) {
	svc := newTestService(t)
	// 用本地文件流避免点播预载测试依赖网络：它走 mpv 分支，只登记可取消的预载任务。
	pv := &preloadTestProvider{stream: player.Stream{URL: "file:///tmp/movie.mp4", Kind: player.StreamLocal}}
	svc.vods["demo"] = pv
	svc.playbackToken = 7
	svc.playbackSeq = 3

	plan, err := svc.PreloadVod("demo", "ep-2")
	if err != nil {
		t.Fatalf("PreloadVod: %v", err)
	}
	if plan.Backend != playback.BackendMPV || plan.ID == "" {
		t.Fatalf("plan = %#v，期望可释放的 mpv 预载计划", plan)
	}
	if svc.playbackToken != 7 || svc.playbackSeq != 3 {
		t.Fatalf("预载不应改动播放会话: token=%d seq=%d", svc.playbackToken, svc.playbackSeq)
	}
	if err := svc.ReleasePreload(plan.ID); err != nil {
		t.Fatalf("ReleasePreload: %v", err)
	}
	if err := svc.ReleasePreload("unknown"); err != nil {
		t.Fatalf("释放未知预载应幂等: %v", err)
	}
}

func TestPreloadVodUnknownSiteDoesNotTouchPlayback(t *testing.T) {
	svc := newTestService(t)
	svc.playbackToken = 7
	svc.playbackSeq = 3

	if _, err := svc.PreloadVod("missing", "ep-1"); err == nil {
		t.Fatal("未知站点应返回错误")
	}
	if svc.playbackToken != 7 || svc.playbackSeq != 3 {
		t.Fatalf("预载失败不应改动播放会话: token=%d seq=%d", svc.playbackToken, svc.playbackSeq)
	}
}
