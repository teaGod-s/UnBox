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
