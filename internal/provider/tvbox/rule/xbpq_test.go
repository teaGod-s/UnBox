package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func xbpqFixture(t *testing.T) (*XBPQEngine, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/category/1/1":
			_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"><img data-original="https://img/1.jpg"></a></li>
<li class="item"><a title="乙" href="/v/2"><img data-original="https://img/2.jpg"></a></li>`))
		case "/search":
			_, _ = w.Write([]byte(`<li class="item"><a title="甲" href="/v/1"></a></li>`))
		case "/v/1":
			_, _ = w.Write([]byte(`<title>甲</title>
<div>类型：电影</div><div>年份：2026</div><div>地区：大陆</div><div>状态：HD</div>
<div class="src"><b>线路一</b></div><div class="src"><b>线路二</b></div>
<div class="play"> <a href="/p/1"><span>第1集</span></a><a href="/p/2"><span>第2集</span></a></div>
<div class="play"> <a href="/p/3"><span>第1集</span></a></div>`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	rule := &XBPQ{
		HomeURL:       srv.URL + "/",
		Classes:       "电影$1#剧集$2",
		CategoryURL:   srv.URL + "/category/{cateId}/{catePg}",
		Array:         `class="item"&&</li>`,
		Title:         `title="&&"`,
		Link:          `href="&&"`,
		Pic:           `data-original="&&"`,
		Type:          `类型：&&<`,
		Year:          `年份：&&<`,
		Area:          `地区：&&<`,
		Remarks:       `状态：&&<`,
		PlayFromArray: `class="src">&&</div>`,
		PlayFromTitle: `>&&<`,
		PlayArray:     `class="play">&&</div>`,
		PlayList:      `<a&&</a>`,
		PlayTitle:     `<span>&&</span>`,
		PlayLink:      `href="&&"`,
	}
	return NewXBPQEngine(rule, NewClient(nil)), srv
}

func TestNewEngineXBPQ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"主页url":"https://a/","分类":"电影$1","分类url":"https://a/{cateId}/{catePg}"}`))
	}))
	defer srv.Close()
	got, err := NewEngine(context.Background(), "csp_XBPQ", srv.URL)
	if err != nil || got == nil {
		t.Fatalf("NewEngine = %v, %v", got, err)
	}
}

func TestXBPQHome(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	secs, err := e.Home(context.Background())
	if err != nil || len(secs) != 2 || secs[0].ID != "1" || secs[0].Title != "电影" {
		t.Fatalf("Home = %#v, %v", secs, err)
	}
}

func TestXBPQBrowse(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	page, err := e.Browse(context.Background(), "1", 1)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("Browse = %#v, %v", page, err)
	}
	if page.Items[0].Title != "甲" || page.Items[0].ID != srv.URL+"/v/1" {
		t.Fatalf("首项 = %#v", page.Items[0])
	}
}

func TestXBPQDetail(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "甲" || v.TypeName != "电影" || v.PlayFrom != "线路一$$$线路二" {
		t.Fatalf("Detail = %#v", v)
	}
	wantPlay := "第1集$" + srv.URL + "/p/1#第2集$" + srv.URL + "/p/2$$$第1集$" + srv.URL + "/p/3"
	if v.PlayURL != wantPlay {
		t.Fatalf("PlayURL = %q，期望 %q", v.PlayURL, wantPlay)
	}
}

func TestXBPQResolve(t *testing.T) {
	e, srv := xbpqFixture(t)
	defer srv.Close()
	stream, err := e.Resolve(context.Background(), "https://cdn/x.m3u8")
	if err != nil || stream.URL != "https://cdn/x.m3u8" {
		t.Fatalf("Resolve = %#v, %v", stream, err)
	}
}
