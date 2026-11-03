package rule

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/player"
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

// 真实规则有两套详情字段名：影片类型/影片年代/影片地区/影片状态 与
// 类型/年代/地区/状态。后者不能被引擎忽略。
func TestXBPQDetailLegacyFieldAliases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title>
<div>类型：电影</div><div>年份：2026</div><div>地区：大陆</div><div>状态：HD</div>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{
		HomeURL:      srv.URL + "/",
		LegacyType:   `类型：&&<`,
		LegacyYear:   `年份：&&<`,
		LegacyArea:   `地区：&&<`,
		LegacyStatus: `状态：&&<`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.TypeName != "电影" || v.Year != "2026" || v.Area != "大陆" || v.Remarks != "HD" {
		t.Fatalf("字段别名未生效: %#v", v)
	}
}

// 详情页的 h1 比列表用的 标题 规则更可靠：真实站点的 标题 规则会先匹配到
// 导航里的「首页」。
func TestXBPQDetailPrefersH1Title(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<h1 class="title"><a href="/v/1"><span>生化危机</span></a></h1>
<nav><a>首页</a></nav>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{
		HomeURL: srv.URL + "/",
		Title:   `a>&&</a>`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "生化危机" {
		t.Fatalf("详情标题 = %q，期望 h1 文本", v.Name)
	}
}

// 真实规则常留空 播放列表/播放标题/播放链接，XBPQ 引擎此时要用默认锚点规则。
func TestXBPQDetailPlayRuleDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title>
<div class="play">播放集数<ul><li><a href="/p/1.html">第1集</a></li><li><a href="/p/2.html">第2集</a></li></ul></div>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{
		HomeURL:   srv.URL + "/",
		PlayArray: `播放集数&&</div>`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	want := "第1集$" + srv.URL + "/p/1.html#第2集$" + srv.URL + "/p/2.html"
	if v.PlayURL != want {
		t.Fatalf("PlayURL = %q，期望 %q", v.PlayURL, want)
	}
}

// 播放块里常先出现「视频排序」这类 javascript:; 按钮，不能当成剧集。
func TestXBPQDetailSkipsJavascriptLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title>
<div class="play">播放集数<a href="javascript:;">视频排序：正序</a><a href="/p/1.html">第1集</a></div>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{
		HomeURL:   srv.URL + "/",
		PlayArray: `播放集数&&</div>`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.PlayURL != "第1集$"+srv.URL+"/p/1.html" {
		t.Fatalf("PlayURL = %q，javascript 链接应被跳过", v.PlayURL)
	}
}

// 线路数组规则可能过宽，多出的名称不能超过实际播放段数。
func TestXBPQDetailCapsSourcesToPlayGroups(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>甲</title>
<div class="src"><b>线路一</b></div><div class="src"><b>线路二</b></div><div class="src"><b>线路三</b></div>
<div class="play">播放集数<a href="/p/1.html">第1集</a></div>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{
		HomeURL:       srv.URL + "/",
		PlayFromArray: `class="src">&&</div>`,
		PlayFromTitle: `>&&<`,
		PlayArray:     `播放集数&&</div>`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/v/1")
	if err != nil {
		t.Fatal(err)
	}
	if v.PlayFrom != "线路一" {
		t.Fatalf("PlayFrom = %q，应截断到实际播放段数", v.PlayFrom)
	}
}

// 真实站点（冷曦影视所用 fed 播放器）把真实 m3u8 以 SXn+base64 放在
// iframe 的 data-play 属性里；Resolve 必须解出它，而不是把 HTML 播放页
// 当视频地址交给播放器。
func TestXBPQResolvePlayPageDataPlay(t *testing.T) {
	const realURL = "https://hn.example.com/play/abc/index.m3u8"
	encoded := "SXn" + base64.StdEncoding.EncodeToString([]byte(realURL))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<iframe id="fed-play-iframe" data-play="` + encoded + `" data-pars="/player.php?id=m3u8&url="></iframe>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{HomeURL: srv.URL + "/"}, NewClient(nil))
	stream, err := e.Resolve(context.Background(), srv.URL+"/p/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if stream.URL != realURL || stream.Kind != player.StreamHLS {
		t.Fatalf("Resolve = %#v，期望解出真实 m3u8", stream)
	}
}

// 播放页里没有 data-play 时保持原地址，避免破坏其他站点。
func TestXBPQResolveFallsBackToPageURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>no player</body></html>`))
	}))
	defer srv.Close()

	e := NewXBPQEngine(&XBPQ{HomeURL: srv.URL + "/"}, NewClient(nil))
	page := srv.URL + "/p/1.html"
	stream, err := e.Resolve(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	if stream.URL != page {
		t.Fatalf("Resolve = %q，期望保留原地址", stream.URL)
	}
}
