package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

// newDrpyTestServer 起一个按路径返回固定 JSON 的 drpy 服务，并记录命中请求。
func newDrpyTestServer(t *testing.T, paths map[string]string) (*httptest.Server, func() []string) {
	t.Helper()
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path+"?"+r.URL.RawQuery)
		body, ok := paths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return srv, func() []string { return hits }
}

func TestDrpyHome(t *testing.T) {
	srv, hits := newDrpyTestServer(t, map[string]string{
		"/api/homeVod": `{"class":[{"type_id":"1","type_name":"电影"},{"type_id":"2","type_name":"剧集"}],"list":[]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 2 || secs[0].Title != "电影" || secs[1].ID != "2" {
		t.Fatalf("Home = %+v, %v", secs, err)
	}
	if got := hits(); len(got) != 1 || got[0] != "/api/homeVod?" {
		t.Fatalf("Home 请求 = %v", got)
	}
}

func TestDrpyBrowseAndSearch(t *testing.T) {
	srv, hits := newDrpyTestServer(t, map[string]string{
		"/api/category":      `{"list":[{"vod_id":1,"vod_name":"电影A","vod_pic":"http://x/p.jpg"}]}`,
		"/api/searchContent": `{"list":[{"vod_id":2,"vod_name":"搜索结果"}]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	pg, err := p.Browse(context.Background(), "1", 2)
	if err != nil || len(pg.Items) != 1 || pg.Items[0].Title != "电影A" || pg.Items[0].Logo != "http://x/p.jpg" {
		t.Fatalf("Browse = %+v, %v", pg, err)
	}
	items, err := p.Search(context.Background(), "测试")
	if err != nil || len(items) != 1 || items[0].Title != "搜索结果" {
		t.Fatalf("Search = %+v, %v", items, err)
	}
	got := hits()
	if len(got) != 2 || got[0] != "/api/category?pg=2&tid=1" || got[1] != "/api/searchContent?key=%E6%B5%8B%E8%AF%95" {
		t.Fatalf("请求序列 = %v", got)
	}
}

func TestDrpyDetailAndResolve(t *testing.T) {
	srv, _ := newDrpyTestServer(t, map[string]string{
		"/api/detailContent": `{"list":[{"vod_id":10,"vod_name":"测试片","vod_play_from":"线路一","vod_play_url":"第01集$https://x/a.m3u8"}]}`,
	})
	defer srv.Close()

	p := NewDrpy(config.Site{Key: "s1", Name: "站点", Type: config.SiteTypeSpider, API: srv.URL})
	m, err := p.Detail(context.Background(), "10")
	if err != nil || len(m.Episodes) != 1 || m.Sources[0] != "线路一" || m.Episodes[0].ID != "10/0/0" {
		t.Fatalf("Detail = %+v, %v", m, err)
	}
	st, err := p.Resolve(context.Background(), "10/0/0")
	if err != nil || st.URL != "https://x/a.m3u8" {
		t.Fatalf("Resolve = %+v, %v", st, err)
	}
	if st.Headers["Referer"] == "" {
		t.Fatalf("Resolve 应带 Referer: %+v", st.Headers)
	}
}
