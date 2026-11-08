package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestTemplateReadsRule(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "crawler", "example.js"))
	if err != nil {
		t.Fatal(err)
	}
	e := New()
	if err := e.Load(string(b)); err != nil {
		t.Fatal(err)
	}
	v, ok := e.vm.Get("rule").(*goja.Object)
	if !ok || v.Get("class_name").String() != "电影&电视剧" {
		t.Fatalf("rule.class_name=%v", e.vm.Get("rule"))
	}
}

func TestTemplateImperativeMethods(t *testing.T) {
	e := New()
	if err := e.Load(`
function homeVod(){ return [{vod_id:"1", vod_name:"甲", vod_pic:"p"}] }
function category(tid, pg){ return [{vod_id:tid+pg, vod_name:"乙"}] }
function search(wd){ return [{vod_id:wd, vod_name:"丙"}] }
function detail(id){ return {vod_id:id, vod_name:"丁", vod_content:"简介", vod_play_from:"线路", vod_play_url:"第一集$http://example.com/v"} }
`); err != nil {
		t.Fatal(err)
	}
	items, err := e.VodHome()
	if err != nil || len(items) != 1 || items[0].VodName != "甲" {
		t.Fatalf("home=%#v err=%v", items, err)
	}
	items, err = e.VodCategory("1", 2)
	if err != nil || len(items) != 1 || items[0].VodID != "12" {
		t.Fatalf("category=%#v err=%v", items, err)
	}
	items, err = e.VodSearch("q")
	if err != nil || len(items) != 1 || items[0].VodID != "q" {
		t.Fatalf("search=%#v err=%v", items, err)
	}
	d, err := e.VodDetail("1")
	if err != nil || d == nil || d.VodContent != "简介" {
		t.Fatalf("detail=%#v err=%v", d, err)
	}
}

func TestFongMiModuleActions(t *testing.T) {
	e := New()
	if err := e.Load(`
async function init(){ return JSON.stringify({}) }
async function homeContent(filter){ return JSON.stringify({class:[{type_id:"movie",type_name:"电影"}]}) }
async function categoryContent(tid, pg, filter, extend){ return JSON.stringify({list:[{vod_id:tid+pg,vod_name:"分类片"}]}) }
async function searchContent(key, quick){ return JSON.stringify({list:[{vod_id:"2",vod_name:key}]}) }
async function detailContent(ids){ return JSON.stringify({list:[{vod_id:ids[0],vod_name:"详情",vod_play_from:"线路",vod_play_url:"第01集$https://example.com/play.m3u8"}]}) }
async function playerContent(flag, id, vipFlags){ return JSON.stringify({parse:0,url:id}) }
export default { init, homeContent, categoryContent, searchContent, detailContent, playerContent }
`); err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	classes, err := e.VodClasses()
	if err != nil || len(classes) != 1 || classes[0].TypeID != "movie" {
		t.Fatalf("classes=%+v err=%v", classes, err)
	}
	items, err := e.VodCategory("movie", 2)
	if err != nil || len(items) != 1 || items[0].VodID != "movie2" {
		t.Fatalf("category=%+v err=%v", items, err)
	}
	items, err = e.VodSearch("搜索")
	if err != nil || len(items) != 1 || items[0].VodName != "搜索" {
		t.Fatalf("search=%+v err=%v", items, err)
	}
	detail, err := e.VodDetail("2")
	if err != nil || detail.VodPlayURL == "" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	playURL, err := e.VodPlay("线路", "https://example.com/play.m3u8")
	if err != nil || playURL != "https://example.com/play.m3u8" {
		t.Fatalf("playURL=%q err=%v", playURL, err)
	}
}

func TestDrpyRuleEndToEnd(t *testing.T) {
	script := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/spider.js":
			_, _ = w.Write([]byte(script))
		case "/":
			_, _ = w.Write([]byte(`<ul class="nav"><li><a href="/list/movie.html">电影</a></li></ul>`))
		case "/list/-1.json", "/list/movie-1.json", "/search":
			_, _ = w.Write([]byte(`{"data":{"movies":[{"title":"甲","cover":"p.jpg","id":"1","description":"简介","type_name":"电影"}]}}`))
		case "/detail/1.json":
			_, _ = w.Write([]byte(`{"data":{"title":"甲","cover":"p.jpg","content":"简介","play_from":"线路","play_url":"第一集$https://example.com/a.m3u8?token=1"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	script = fmt.Sprintf(`var rule={
  host:%q,
  url:"/list/fyclass-fypage.json",
  searchUrl:"/search?wd=**&page=fypage",
  detailUrl:"/detail/{id}.json",
  class_parse:".nav&&li;a&&Text;a&&href;/([a-z]+).html",
  一级:"json:data.movies;title;cover;id;description;type_name",
  搜索:"json:data.movies;title;cover;id;description;type_name",
  二级:"json:data;title;cover;content;play_from;play_url",
  lazy:"js:input={url:input.url.split('?')[0]}"
}`, srv.URL)

	e := New()
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	classes, err := e.VodClasses()
	if err != nil || len(classes) != 1 || classes[0].TypeID != "movie" {
		t.Fatalf("classes=%+v err=%v", classes, err)
	}
	items, err := e.VodHome()
	if err != nil || len(items) != 1 || items[0].VodName != "甲" {
		t.Fatalf("home=%+v err=%v", items, err)
	}
	items, err = e.VodCategory("movie", 1)
	if err != nil || len(items) != 1 || items[0].VodName != "甲" {
		t.Fatalf("category=%+v err=%v", items, err)
	}
	items, err = e.VodSearch("甲")
	if err != nil || len(items) != 1 || items[0].VodID != "1" {
		t.Fatalf("search=%+v err=%v", items, err)
	}
	detail, err := e.VodDetail("1")
	if err != nil || detail.VodName != "甲" || detail.VodPlayURL == "" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	playURL, err := e.VodPlay("线路", "https://example.com/a.m3u8?token=1")
	if err != nil || playURL != "https://example.com/a.m3u8" {
		t.Fatalf("play=%q err=%v", playURL, err)
	}
}

func TestDrpyRuleAppliesHeaders(t *testing.T) {
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Rule")
		_, _ = w.Write([]byte(`{"data":{"movies":[]}}`))
	}))
	defer srv.Close()

	e := New()
	script := fmt.Sprintf(`var rule={host:%q,url:"/list/fyclass-fypage.json",headers:{"X-Rule":"yes"},一级:"json:data.movies;title;id"}`, srv.URL)
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	if _, err := e.VodCategory("movie", 1); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "yes" {
		t.Fatalf("X-Rule=%q", got)
	}
}

func TestDrpyPlayURLTemplate(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={host:"https://example.test",playUrl:"/play/fyid.m3u8"}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.VodPlay("线路", "movie-1")
	if err != nil || got != "https://example.test/play/movie-1.m3u8" {
		t.Fatalf("play=%q err=%v", got, err)
	}
}

// 听海/库里等音乐规则的 playerContent 返回 {url:[清晰度,地址,...]} 数组
// （多清晰度列表）。VodPlay 应从数组里取第一个真实媒体地址，而不是因
// 数组无法解码为 string 而报错（否则 Resolve 把 rid 裸 ID 当播放地址）。
func TestDrpyPlayArrayURL(t *testing.T) {
	e := New()
	script := `async function playerContent(flag, id, vipFlags) {
  return JSON.stringify({parse:0, url:["FLAC无损","https://cdn.example.com/song.flac","HQ高品质","https://cdn.example.com/song.mp3"]})
}`
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	got, err := e.VodPlay("音乐欣赏", "rid123")
	if err != nil {
		t.Fatalf("VodPlay = %v, %v", got, err)
	}
	if got != "https://cdn.example.com/song.flac" {
		t.Fatalf("VodPlay = %q，期望取数组第一个媒体地址", got)
	}
}

// playerContent 返回单个字符串 url（常见形态）不能回归。
func TestDrpyPlayStringURL(t *testing.T) {
	e := New()
	script := `async function playerContent(flag, id, vipFlags) {
  return JSON.stringify({parse:0, url:"https://cdn.example.com/a.m3u8"})
}`
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	got, err := e.VodPlay("线路", "movie-1")
	if err != nil || got != "https://cdn.example.com/a.m3u8" {
		t.Fatalf("play=%q err=%v", got, err)
	}
}

func TestDrpyJSDetailReceivesDetailURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/detail" || r.URL.Query().Get("cat") != "2" || r.URL.Query().Get("id") != "42" {
			t.Errorf("url=%s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"title":"详情正文"}`))
	}))
	defer srv.Close()

	e := New()
	script := fmt.Sprintf(`var rule={host:%q,class_url:"2&1",detailUrl:"/detail?cat=fyclass&id=fyid",二级:"js:let data=JSON.parse(fetch(input,fetch_params)); VOD.vod_name=data.title"}`, srv.URL)
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	detail, err := e.VodDetail("42")
	if err != nil || detail.VodName != "详情正文" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

// 用户选择的筛选值必须进入 categoryContent 的 extend：旧实现只传空对象，
// 脚本拿不到选择（plan Task 3 验收）。
func TestVodCategoryExtendPassesExtend(t *testing.T) {
	e := New()
	src := `var rule = {
		host: "https://example.com",
		url: "/list/fyclass-fyfilter-fypage.html",
		filter_url: "&area={{fl.area}}&year={{fl.year}}",
		filter_def: {"1": {"area": "内地", "year": "2023"}}
	};
	function categoryContent(tid, pg, filter, extend){
		return JSON.stringify({list:[{vod_id:"1", vod_name:"热片", vod_remarks: extend ? (extend.area + "/" + extend.year) : "无"}]});
	}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	vods, err := e.VodCategoryExtend("1", 1, map[string]string{"area": "欧美", "year": "2023"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vods) != 1 || vods[0].VodName != "热片" {
		t.Fatalf("category 返回异常: %+v", vods)
	}
	if vods[0].VodRemarks != "欧美/2023" {
		t.Fatalf("extend 未把用户选择传入脚本: %q", vods[0].VodRemarks)
	}
}

// 规则 URL 路径：选择值覆盖 filter_def 默认值后再渲染 {{fl.xxx}}，
// 未选的维度沿用站点默认值。
func TestVodCategoryExtendMergesSelectionIntoFilterURL(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<div class="item"><a href="/v/1">热片</a><h3>热片</h3></div>`))
	}))
	defer srv.Close()

	e := New()
	src := fmt.Sprintf(`var rule = {
		host: %q,
		url: "/list/fyclass-fyfilter-fypage.html",
		filter_url: "&area={{fl.area}}&year={{fl.year}}",
		filter_def: {"1": {"area": "内地", "year": "2023"}},
		vod_selector: ".item",
		name_selector: "h3",
		id_selector: "a",
	};`, srv.URL)
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	if _, err := e.VodCategoryExtend("1", 1, map[string]string{"area": "欧美"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "area="+url.QueryEscape("欧美")) || !strings.Contains(got, "year=2023") {
		t.Fatalf("filter_url 未合并选择值（应 area=欧美、year 沿用默认 2023），请求 %q", got)
	}
}

// vod_id 形如 `分类$真实id` 时，详情 URL 要拆开填 fyclass / fyid。
func TestVodDetailSplitsClassFromDollarID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("cat"); got != "1" {
			t.Errorf("cat=%q, want 1", got)
		}
		if got := r.URL.Query().Get("id"); got != "abc" {
			t.Errorf("id=%q, want abc", got)
		}
		_, _ = w.Write([]byte(`{"title":"详情正文"}`))
	}))
	defer srv.Close()

	e := New()
	script := fmt.Sprintf(`var rule={host:%q,class_url:"2&1",detailUrl:"/detail?cat=fyclass&id=fyid",二级:"js:let data=JSON.parse(fetch(input,fetch_params)); VOD.vod_name=data.title"}`, srv.URL)
	if err := e.Load(script); err != nil {
		t.Fatal(err)
	}
	detail, err := e.VodDetail("1$abc")
	if err != nil || detail.VodName != "详情正文" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}
