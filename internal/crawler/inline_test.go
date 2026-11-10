package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestExtractVodsJSONRealFieldAliases(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={搜索:"json:data.rows;titleTxt||titlealias;cover;cat_name;cat_id+en_id;description"}`); err != nil {
		t.Fatal(err)
	}
	vods, err := e.extractVods("搜索", `{"data":{"rows":[{"titleTxt":"甲","cover":"p.jpg","cat_name":"电视剧","cat_id":"2","en_id":"Pb1","description":"简介"}]}}`, &Rule{})
	if err != nil || len(vods) != 1 || vods[0].VodName != "甲" || vods[0].VodID != "2$Pb1" || vods[0].TypeName != "电视剧" {
		t.Fatalf("vods=%+v err=%v", vods, err)
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

func TestDrpyHelpersAreAvailable(t *testing.T) {
	e := New()
	if err := e.Load(`function check(){ print("ok"); return [typeof buildUrl, typeof urlDeal, buildUrl("https://example.test/", "v"), urlDeal("https://example.test/", "p"), buildUrl("https://example.test/v", {start:1,site:"qiyi"})] }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("check")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := e.vm.ExportTo(v, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0] != "function" || got[1] != "function" || got[2] != "https://example.test/v" || got[3] != "https://example.test/p" || got[4] != "https://example.test/v?site=qiyi&start=1" {
		t.Fatalf("helpers=%v", got)
	}
}

func TestExtractDetailJSFetchReturnsContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"title":"真实响应"}`))
	}))
	defer srv.Close()

	e := New()
	if err := e.Load(`var rule={二级:"js:let data=JSON.parse(fetch(input,fetch_params)); VOD.vod_name=data.title"}`); err != nil {
		t.Fatal(err)
	}
	detail, err := e.extractDetail(fmt.Sprintf("%s/detail", srv.URL), &Rule{}, "42")
	if err != nil || detail.VodName != "真实响应" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

// drpy2 的列表规则里，id 字段写 `a+b` 时是 links.join("$")（源码 search/一级 两处），
// 不是裸拼接：详情那一步会把 "$" 前一段当分类（fyclass）、后一段当真实 id。
// 裸拼接会让 ?cat=fyclass&id=fyid 这类规则拿到「分类+id 粘在一起」的串，
// 站点直接拒绝（某影视源就是这么挂的：data 为 null，详情 JS 报 title of undefined）。
func TestJSONListJoinsIDFieldWithDollar(t *testing.T) {
	raw := `{"data":{"rows":[{"cat_id":"1","en_id":"abc","titleTxt":"忍者神龟","cover":"c","cat_name":"电影","description":"d"}]}}`
	vods, err := extractJSONVods(raw, "json:data.rows;titleTxt;cover;cat_name;cat_id+en_id;description")
	if err != nil {
		t.Fatal(err)
	}
	if len(vods) != 1 {
		t.Fatalf("应解析出 1 条，got %d", len(vods))
	}
	if got := string(vods[0].VodID); got != "1$abc" {
		t.Fatalf("id 字段 a+b 应以 $ 连接，got %q", got)
	}
}

// 站点会间歇性返回 {"longData":{}} 这种空响应：外层结构在、列表键缺失。
// 这不该报「搜索失败」，按没有结果处理。
func TestExtractJSONVodsEmptyParentMeansNoResults(t *testing.T) {
	vods, err := extractJSONVods(`{"data":{"longData":{}}}`, "json:data.longData.rows;title;cover;cat;id;desc")
	if err != nil {
		t.Fatalf("空列表响应不该报错: %v", err)
	}
	if len(vods) != 0 {
		t.Fatalf("应为 0 条，got %d", len(vods))
	}
}

// 但路径写错（上级都不存在）仍要报错，别把配置错误吞成「没搜到」。
func TestExtractJSONVodsWrongPathStillErrors(t *testing.T) {
	if _, err := extractJSONVods(`{"code":500}`, "json:data.longData.rows;title"); err == nil {
		t.Fatal("上级不存在的路径应报错")
	}
}
