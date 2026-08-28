package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
)

func sampleChannels() []config.Channel {
	return []config.Channel{
		{Name: "CCTV-1", URLs: []string{"http://x/1.m3u8", "http://x/1b.m3u8"}, Logo: "l1", Group: "央视"},
		{Name: "湖南卫视", URLs: []string{"http://x/hunan.ts"}, Group: "卫视"},
	}
}

func TestNewHomeAndBrowse(t *testing.T) {
	p := New(sampleChannels())
	if p.ID() != "live" {
		t.Fatalf("ID = %q", p.ID())
	}
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 2 {
		t.Fatalf("Home = %v, %v", secs, err)
	}
	pg, err := p.Browse(context.Background(), "央视", 0)
	if err != nil || len(pg.Items) != 1 || pg.Items[0].Title != "CCTV-1" {
		t.Fatalf("Browse = %v, %v", pg, err)
	}
}

func TestResolveBackups(t *testing.T) {
	p := New(sampleChannels())
	id := "央视/CCTV-1"
	st, err := p.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if st.URL != "http://x/1.m3u8" || len(st.Backups) != 1 || st.Backups[0] != "http://x/1b.m3u8" {
		t.Fatalf("Resolve = %+v", st)
	}
	if st.Kind != player.StreamHLS {
		t.Fatalf("Kind = %v, want HLS", st.Kind)
	}
}

func TestResolveUnknownID(t *testing.T) {
	p := New(sampleChannels())
	if _, err := p.Resolve(context.Background(), "不存在/x"); err == nil {
		t.Fatal("未知 ID 应报错")
	}
}

func TestResolveClassifiesHTTPTransportStream(t *testing.T) {
	p := New([]config.Channel{{Name: "TS", Group: "测试", URLs: []string{"https://media.example/live.ts?token=1"}}})
	st, err := p.Resolve(context.Background(), "测试/TS")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if st.Kind != player.StreamTS {
		t.Fatalf("Kind = %v, want TS", st.Kind)
	}
}

func TestFetchLiveParsesM3U(t *testing.T) {
	// 用 httptest 起一个 m3u 服务，验证 FetchLive 拉取 + 解析 + 归并同名备份。
	srv := newM3UTestServer(t, "#EXTM3U\n"+
		"#EXTINF:-1 group-title=\"测试\",频道A\nhttp://srv/a\n"+
		"#EXTINF:-1 group-title=\"测试\",频道A\nhttp://srv/a2\n")
	defer srv.Close()
	chs, err := FetchLive(context.Background(), config.Live{URL: srv.URL}, config.NewFetcher())
	if err != nil {
		t.Fatalf("FetchLive: %v", err)
	}
	if len(chs) != 1 || len(chs[0].URLs) != 2 {
		t.Fatalf("FetchLive = %+v，期望同名归并为 1 频道 2 备份", chs)
	}
}

// newM3UTestServer 起一个返回固定 body 的 HTTP 测试服务。
func newM3UTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	return srv
}
