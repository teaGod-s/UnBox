package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

// 真实配置里 ext 可能是 JSON 对象，必须序列化后交给脚本，否则 cfg.ext 为
// undefined，脚本的 cfg.ext.hasOwnProperty(...) 直接抛 TypeError。
func TestScriptExtSerializesObject(t *testing.T) {
	site := config.Site{Ext: map[string]any{"categories": "短剧#沙雕", "cookie": ""}}
	got := scriptExt(site)
	if !strings.HasPrefix(got, "{") || !strings.Contains(got, "短剧") {
		t.Fatalf("对象 ext 应序列化成 JSON，得 %q", got)
	}
	if got := scriptExt(config.Site{Ext: "./js/rule.js"}); got != "./js/rule.js" {
		t.Fatalf("字符串 ext 应原样返回，得 %q", got)
	}
	if got := scriptExt(config.Site{}); got != "" {
		t.Fatalf("无 ext 应为空，得 %q", got)
	}
}

func TestSpiderRunsImperativeCrawler(t *testing.T) {
	script := `
function home(){ return [{vod_id:"1",vod_name:"甲",type_name:"电影"}] }
function category(tid, pg){ return [{vod_id:"1",vod_name:"甲",vod_pic:"https://example.com/a.jpg",type_name:tid}] }
function search(wd){ return [{vod_id:"1",vod_name:wd}] }
function detail(id){ return {vod_id:id,vod_name:"甲",vod_pic:"https://example.com/a.jpg",vod_content:"简介",vod_year:"2026",vod_area:"中国",vod_play_from:"线路一",vod_play_url:"第01集$https://example.com/play.m3u8"} }
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(script)) }))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "s1", Name: "示例站", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != "s1" {
		t.Fatalf("ID=%q", p.ID())
	}
	sections, err := p.Home(context.Background())
	if err != nil || len(sections) != 1 || sections[0].Title != "电影" {
		t.Fatalf("Home=%+v err=%v", sections, err)
	}
	page, err := p.Browse(context.Background(), "电影", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Title != "甲" {
		t.Fatalf("Browse=%+v err=%v", page, err)
	}
	items, err := p.Search(context.Background(), "搜索")
	if err != nil || len(items) != 1 || items[0].Title != "搜索" {
		t.Fatalf("Search=%+v err=%v", items, err)
	}
	media, err := p.Detail(context.Background(), "1")
	if err != nil || media.Title != "甲" || len(media.Episodes) != 1 {
		t.Fatalf("Detail=%+v err=%v", media, err)
	}
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil || stream.URL != "https://example.com/play.m3u8" {
		t.Fatalf("Resolve=%+v err=%v", stream, err)
	}
}

func TestSpiderScriptURL(t *testing.T) {
	cases := []struct {
		name string
		site config.Site
		want string
	}{
		{
			name: "drpy2 用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js", Ext: "https://x/dr/js/某影视源.js"},
			want: "https://x/dr/js/某影视源.js",
		},
		{
			name: "带 query 的 drpy2 用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js?v=1", Ext: "https://x/dr/js/a.js"},
			want: "https://x/dr/js/a.js",
		},
		{
			name: "非标准命名 drpy2.1 用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy2.1.min.js", Ext: "https://x/dr/js/a.js"},
			want: "https://x/dr/js/a.js",
		},
		{
			name: "fragment 的 drpy 引擎库用 ext",
			site: config.Site{API: "https://x/dr/lib/drpy.min.js#v", Ext: "https://x/dr/js/a.js"},
			want: "https://x/dr/js/a.js",
		},
		{
			name: "带 query 的 ext 也识别为 js",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js", Ext: "https://x/dr/js/rule.js?v=2"},
			want: "https://x/dr/js/rule.js?v=2",
		},
		{
			name: "相对 ext 带 query",
			site: config.Site{API: "https://x/dr/lib/drpy2.min.js", Ext: "./rule.js?v=2"},
			want: "./rule.js?v=2",
		},
		{
			name: "站点脚本直接用 api",
			site: config.Site{API: "https://x/js/apple.js", Ext: "https://x/js/other.js"},
			want: "https://x/js/apple.js",
		},
		{
			name: "ext 不是 js 时用 api",
			site: config.Site{API: "https://x/js/apple.js", Ext: "https://x/rule.json"},
			want: "https://x/js/apple.js",
		},
		{
			name: "无 ext 时用 api",
			site: config.Site{API: "https://x/js/apple.js"},
			want: "https://x/js/apple.js",
		},
	}
	for _, c := range cases {
		if got := spiderScriptURL(c.site); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSpiderRunsFongMiModuleCrawler(t *testing.T) {
	script := `
async function init(){ return JSON.stringify({}) }
async function homeContent(filter){ return JSON.stringify({class:[{type_id:"movie",type_name:"电影"}]}) }
async function categoryContent(tid, pg, filter, extend){ return JSON.stringify({list:[{vod_id:"1",vod_name:"甲",type_name:tid}]}) }
async function searchContent(key, quick){ return JSON.stringify({list:[{vod_id:"1",vod_name:key}]}) }
async function detailContent(ids){ return JSON.stringify({list:[{vod_id:ids[0],vod_name:"甲",vod_play_from:"线路一",vod_play_url:"第01集$https://example.com/episode"}]}) }
async function playerContent(flag, id, vipFlags){ return JSON.stringify({parse:0,url:id+".m3u8"}) }
export default { init, homeContent, categoryContent, searchContent, detailContent, playerContent }
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(script)) }))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "js0", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := p.Home(context.Background())
	if err != nil || len(sections) != 1 || sections[0].ID != "movie" {
		t.Fatalf("Home=%+v err=%v", sections, err)
	}
	page, err := p.Browse(context.Background(), "movie", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Group != "movie" {
		t.Fatalf("Browse=%+v err=%v", page, err)
	}
	media, err := p.Detail(context.Background(), "1")
	if err != nil || len(media.Episodes) != 1 {
		t.Fatalf("Detail=%+v err=%v", media, err)
	}
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil || stream.URL != "https://example.com/episode.m3u8" {
		t.Fatalf("Resolve=%+v err=%v", stream, err)
	}
}

// VodClasses 失败且 rule/home 回退都没有可用结果时，必须把分类错误抛给上层，
// 而不是返回空分类让调用方误以为站点没有分类。
func TestSpiderHomePropagatesClassesError(t *testing.T) {
	script := `
function homeContent(){ throw new Error("分类接口失败") }
function home(){ return [] }
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(script)) }))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "bad", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := p.Home(context.Background())
	if err == nil {
		t.Fatalf("Home 应返回分类错误，实际 sections=%+v", sections)
	}
	if !strings.Contains(err.Error(), "分类接口失败") {
		t.Fatalf("错误未包含分类失败原因: %v", err)
	}
}

// 有 home 回退且能返回数据时，保持原行为：使用回退结果，不因 VodClasses 失败而报错。
func TestSpiderHomeKeepsHomeFallback(t *testing.T) {
	script := `
function homeContent(){ throw new Error("分类接口失败") }
function home(){ return [{vod_id:"1",vod_name:"甲",type_name:"电影"}] }
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(script)) }))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "fallback", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := p.Home(context.Background())
	if err != nil {
		t.Fatalf("有回退时不应报错: %v", err)
	}
	if len(sections) != 1 || sections[0].Title != "电影" {
		t.Fatalf("Home = %+v，应使用 home 回退", sections)
	}
}

func TestSpiderESMJsEvalReturn(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dep.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`export default "1";`))
	})
	mux.HandleFunc("/spider.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import tid from "./dep.js";
export function __jsEvalReturn(){
  return { homeContent(){ return JSON.stringify({class:[{type_id:tid,type_name:"电影"}]}) } }
}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "esm", Type: config.SiteTypeSpider, API: srv.URL + "/spider.js"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := p.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || sections[0].ID != "1" || sections[0].Title != "电影" {
		t.Fatalf("Home = %#v", sections)
	}
}

// 浏览带筛选：选择值必须经 extend 传入 categoryContent（Task 6 验收）。
func TestSpiderBrowseFilteredPassesExtend(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "filter_site.js"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "sp", Name: "筛选站", Type: config.SiteTypeSpider, API: srv.URL + "/filter_site.js"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.BrowseFiltered(context.Background(), "1", 1, map[string]string{"area": "欧美"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Group != "欧美" {
		t.Fatalf("extend 未生效: %+v", page.Items)
	}
}

// Home 时必须从 rule.filter（FongMi 数组方言）挂载筛选组（Task 6 验收补）。
func TestSpiderHomeCarriesFilterGroups(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "filter_site.js"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	p, err := NewSpider(config.Site{Key: "sp", Name: "筛选站", Type: config.SiteTypeSpider, API: srv.URL + "/filter_site.js"})
	if err != nil {
		t.Fatal(err)
	}
	sections, err := p.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sections {
		for _, g := range s.Filters {
			if g.Key == "area" && len(g.Options) == 2 && g.Options[1].Name == "欧美" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("Home 未从 rule.filter 挂载筛选组: %+v", sections)
	}
}
