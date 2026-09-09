package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
)

// newTestProvider 起一个打桩 CMS 站点并返回 Provider。
func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac := r.URL.Query().Get("ac")
		switch ac {
		case "list":
			w.Write([]byte(`{"code":1,"class":[{"type_id":10,"type_name":"电影"},{"type_id":20,"type_name":"电视剧"}],"list":[]}`))
		case "videolist":
			w.Write([]byte(`{"code":1,"list":[
				{"vod_id":1,"vod_name":"电影A","type_id":10,"type_name":"电影"},
				{"vod_id":2,"vod_name":"剧集B","type_id":20,"type_name":"电视剧"}
			]}`))
		case "detail":
			w.Write([]byte(`{"code":1,"list":[{"vod_id":1,"vod_name":"电影A","vod_content":"简介","vod_year":"2026","vod_area":"大陆","type_name":"电影","vod_play_from":"x$$$y","vod_play_url":"第01集$a#第02集$b$$$第01集$c"}]}`))
		default:
			http.Error(w, "bad ac", 400)
		}
	}))
	t.Cleanup(srv.Close)
	return New(config.Site{Key: "test", Name: "测试站", Type: config.SiteTypeCMS, API: srv.URL})
}

func TestProviderHome(t *testing.T) {
	secs, err := newTestProvider(t).Home(context.Background())
	if err != nil {
		t.Fatalf("Home 失败: %v", err)
	}
	if len(secs) != 2 || secs[0].ID != "10" || secs[0].Title != "电影" || secs[1].Title != "电视剧" {
		t.Fatalf("分类派生错误: %+v", secs)
	}
}

func TestProviderHomeUsesClassMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") != "list" {
			t.Errorf("ac = %q, want list", r.URL.Query().Get("ac"))
		}
		_, _ = w.Write([]byte(`{"code":1,"class":[{"type_id":10,"type_name":"国产剧"},{"type_id":20,"type_name":"韩国剧"}],"list":[{"vod_id":1,"vod_name":"当前页","type_id":10,"type_name":"国产剧"}]}`))
	}))
	defer srv.Close()

	p := New(config.Site{Key: "test", Name: "测试站", Type: config.SiteTypeCMS, API: srv.URL})
	secs, err := p.Home(context.Background())
	if err != nil {
		t.Fatalf("Home 失败: %v", err)
	}
	if len(secs) != 2 || secs[1].Title != "韩国剧" {
		t.Fatalf("Home 未保留完整分类: %+v", secs)
	}
}

func TestProviderBrowseReportsPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"page":"2","pagecount":5,"limit":"20","total":99,"list":[{"vod_id":1,"vod_name":"第二页"}]}`))
	}))
	defer srv.Close()

	p := New(config.Site{Key: "test", Name: "测试站", Type: config.SiteTypeCMS, API: srv.URL})
	page, err := p.Browse(context.Background(), "10", 2)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if page.Page != 2 || page.PageCount != 5 || page.Total != 99 || !page.HasMore {
		t.Fatalf("分页元数据错误: %+v", page)
	}
}

func TestProviderDetailAndResolve(t *testing.T) {
	p := newTestProvider(t)
	m, err := p.Detail(context.Background(), "1")
	if err != nil {
		t.Fatalf("Detail 失败: %v", err)
	}
	if m.Title != "电影A" || m.Description != "简介" || len(m.Sources) != 2 || len(m.Episodes) != 3 {
		t.Fatalf("Detail 解析错误: %+v", m)
	}
	st, err := p.Resolve(context.Background(), "1/0/1")
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	if st.URL != "b" || st.Kind != player.StreamHLS {
		t.Fatalf("Resolve 错误: %+v", st)
	}
	if st.Headers["Referer"] == "" {
		t.Fatalf("Resolve 应带 Referer: %+v", st.Headers)
	}
}

func TestResolveUnknownEpisode(t *testing.T) {
	p := newTestProvider(t)
	if _, err := p.Resolve(context.Background(), "1/9/9"); err == nil {
		t.Fatalf("未知剧集应报错")
	}
}

func TestKindForURLClassifiesTransportStream(t *testing.T) {
	if got := kindForURL("https://media.example/live.ts?token=1"); got != player.StreamTS {
		t.Fatalf("kindForURL() = %v, want TS", got)
	}
}
