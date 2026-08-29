package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

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

func TestSpiderRunsFongMiModuleCrawler(t *testing.T) {
	script := `
async function init(){ return JSON.stringify({}) }
async function home(){ return JSON.stringify({class:[{type_id:"movie",type_name:"电影"}]}) }
async function category(tid, pg){ return JSON.stringify({list:[{vod_id:"1",vod_name:"甲",type_name:tid}]}) }
async function search(wd){ return JSON.stringify({list:[{vod_id:"1",vod_name:wd}]}) }
async function detail(id){ return JSON.stringify({list:[{vod_id:id,vod_name:"甲",vod_play_from:"线路一",vod_play_url:"第01集$https://example.com/episode"}]}) }
async function play(flag, id){ return JSON.stringify({parse:0,url:id+".m3u8"}) }
export default { init, home, category, search, detail, play }
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
