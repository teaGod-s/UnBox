package crawler

import "testing"

func TestExtractVodsJSON(t *testing.T) {
	html := `{"data":{"movies":[{"title":"","name":"乙","cover":"p.jpg","cover2":"?v=2","id":"1","description":"简介","cat_name":"电影"}]}}`
	e := New()
	if err := e.Load(`var rule={一级:"json:data.movies;title||name;cover+cover2;id;description;cat_name"}`); err != nil {
		t.Fatal(err)
	}
	vods, err := e.extractVods("一级", html, &Rule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(vods) != 1 {
		t.Fatalf("vods=%+v", vods)
	}
	got := vods[0]
	if got.VodName != "乙" || got.VodPic != "p.jpg?v=2" || got.VodID != "1" || got.VodContent != "简介" || got.TypeName != "电影" {
		t.Fatalf("vod=%+v", got)
	}
}

func TestExtractVodsMubanHTMLFallback(t *testing.T) {
	e := New()
	if err := e.Load(`muban.首图2.一级.vod = ".item"; muban.首图2.一级.name = ".name&&Text"; var rule={}`); err != nil {
		t.Fatal(err)
	}
	vods, err := e.extractVods("一级", `<div class="item"><a href="/1"><span class="name">甲</span></a></div>`, &Rule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(vods) != 1 || vods[0].VodID != "/1" || vods[0].VodName != "甲" {
		t.Fatalf("vods=%+v", vods)
	}
}

func TestExtractDetailJSON(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={二级:"json:data;title;content;year;area;play_from;play_url"}`); err != nil {
		t.Fatal(err)
	}
	detail, err := e.extractDetail(`{"data":{"title":"甲","content":"简介","year":"2024","area":"中国","play_from":"线路","play_url":"第一集$https://example.test/1"}}`, &Rule{}, "42")
	if err != nil {
		t.Fatal(err)
	}
	if detail.VodID != "42" || detail.VodName != "甲" || detail.VodContent != "简介" || detail.VodYear != "2024" || detail.VodArea != "中国" || detail.VodPlayFrom != "线路" || detail.VodPlayURL != "第一集$https://example.test/1" {
		t.Fatalf("detail=%+v", detail)
	}
}

func TestExtractDetailJS(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={二级:"js:VOD.vod_name='甲'; VOD.vod_content=input; VOD.vod_play_from='线路'; VOD.vod_play_url=urljoin2('https://example.test/a/','1'); VOD.vod_remarks=typeof fetch"}`); err != nil {
		t.Fatal(err)
	}
	detail, err := e.extractDetail("简介", &Rule{}, "42")
	if err != nil {
		t.Fatal(err)
	}
	if detail.VodID != "42" || detail.VodName != "甲" || detail.VodContent != "简介" || detail.VodPlayFrom != "线路" || detail.VodPlayURL != "https://example.test/a/1" || detail.VodRemarks != "function" {
		t.Fatalf("detail=%+v", detail)
	}
}
