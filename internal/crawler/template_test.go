package crawler

import (
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
async function home(){ return JSON.stringify({class:[{type_id:"movie",type_name:"电影"}]}) }
async function category(tid, pg, filter, extend){ return JSON.stringify({list:[{vod_id:tid+pg,vod_name:"分类片"}]}) }
async function search(wd){ return JSON.stringify({list:[{vod_id:"2",vod_name:wd}]}) }
async function detail(id){ return JSON.stringify({list:[{vod_id:id,vod_name:"详情",vod_play_from:"线路",vod_play_url:"第01集$https://example.com/play.m3u8"}]}) }
async function play(flag, id){ return JSON.stringify({parse:0,url:id}) }
export default { init, home, category, search, detail, play }
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
