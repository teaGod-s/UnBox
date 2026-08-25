package shell

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
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
	svc1.saveSubscription("http://sub", cfgs, nil)

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
	svc1.saveSubscription("http://ch.m3u", nil, chans)

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
			{Key: "jar", Name: "JAR站", Type: config.SiteTypeSpider, API: "csp_xxx"},
		},
	}}
	vods, names := collectVodSites(cfgs)
	if len(vods) != 2 {
		t.Fatalf("应有 2 个站点（cms + spider http），得 %d", len(vods))
	}
	if names["sp"] != "爬虫站" {
		t.Fatalf("names = %v", names)
	}
	if _, ok := vods["jar"]; ok {
		t.Fatalf("csp_ JAR 站点不应被收集")
	}
}
