package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
