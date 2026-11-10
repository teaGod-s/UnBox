package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 某影视源RuleHTML 模拟 某影视源 的模块化页面：分类列表（title 属性在 a 上、
// 图片 data-src、广告项混入）、详情页（多演员/多段简介/线路/选集）。
const 某影视源RuleHTML = `<html><body>
<div class="module-items">
  <div class="module-item"><a title="片名A" href="/d/1.html"><div class="module-item-pic"><img data-src="/img/a.jpg"></div><div class="module-item-text">更新至12集</div></a></div>
  <div class="module-item"><a title="片名B" href="/d/2.html"><div class="module-item-pic"><img data-src="/img/b.jpg"></div><div class="module-item-text">全20集</div></a></div>
  <div class="module-item">某推荐源推广</div>
</div>
</body></html>`

const 某影视源DetailHTML = `<html><body>
<div class="video-info-main">
  <div class="video-info-actor">主演：张甲</div>
  <div class="video-info-actor">主演：李乙</div>
  <div class="video-info-content"><p>第一部简介</p><p>第二部简介</p></div>
</div>
<div class="module-player-tab"><div class="module-tab-item"><span>线路A</span></div><div class="module-tab-item"><span>线路B</span></div></div>
<div class="module-player-list">
  <div class="module-row-one"><div class="btn-down"><a href="https://www.aliyundrive.com/s/abc1"><h4>第1集</h4></a></div></div>
  <div class="module-row-one"><div class="btn-down"><a href="https://www.aliyundrive.com/s/abc2"><h4>第2集</h4></a></div></div>
</div>
<div class="module-player-list">
  <div class="module-row-one"><div class="btn-down"><a href="https://www.aliyundrive.com/s/abc3"><h4>第1集</h4></a></div></div>
</div>
</body></html>`

// 某影视源SearchHTML 模拟搜索结果列表（.module-search-item 结构）。
const 某影视源SearchHTML = `<html><body>
<div class="module-items">
  <div class="module-search-item"><h3><a href="/d/1.html">片名A</a></h3><img class="lazyload" data-src="/img/a.jpg"><div class="video-info"><a>更新A</a></div></div>
</div>
</body></html>`

// 某影视源Rule 与 /tmp/xyq/某影视源.json 同字段集合。
func 某影视源Rule(base string) *XYQ {
	return &XYQ{
		HomeURL:         base,
		CategoryURL:     base + "/vodshow/{cateId}--{by}------{catePg}---{year}.html",
		CategoryNames:   "电影&剧集",
		CategoryIDs:     "1&2",
		CategoryArray:   ".module-items&&.module-item:not(:matches(某推荐源|公众号))",
		CatTitle:        "a&&title",
		CatLink:         "a&&href",
		CatPic:          ".module-item-pic&&img&&data-src",
		CatSubtitle:     ".module-item-text&&Text",
		LinkPrefix:      base,
		SearchURL:       base + "/vodsearch/{wd}---{SearchPg}---.html",
		SearchArray:     ".module-items&&.module-search-item",
		SearchTitle:     "h3&&Text",
		SearchLink:      "h3&&a&&href",
		SearchPic:       ".lazyload&&data-src",
		SearchSubtitle:  ".video-info&&a&&Text",
		SearchPrefix:    base,
		Actor:           ".video-info-main&&.video-info-actor,1&&Text",
		Content:         ".video-info-content&&p,-1&&Text",
		PlayerTabArray:  ".module-player-tab&&.module-tab-item",
		PlayerTabTitle:  "span&&Text",
		PlayerListArray: "body&&.module-player-list",
		PlayArray:       ".module-row-one:matches(https://www.ali(pan|yundrive).com/s/)",
		PlayTitle:       "h4&&Text",
		PlayLink:        ".btn-down&&a&&href",
		Headers:         map[string]string{"User-Agent": uaMobile},
	}
}

func xyqFixture(t *testing.T) (*XYQHikerEngine, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "vodsearch"):
			_, _ = w.Write([]byte(某影视源SearchHTML))
		case strings.Contains(r.URL.Path, "vodshow"):
			_, _ = w.Write([]byte(某影视源RuleHTML))
		case strings.Contains(r.URL.Path, "detail1") || strings.Contains(r.URL.Path, "detail2"):
			_, _ = w.Write([]byte(某影视源DetailHTML))
		default:
			_, _ = w.Write([]byte(某影视源RuleHTML))
		}
	})
	srv := httptest.NewServer(mux)
	return NewXYQHikerEngine(某影视源Rule(srv.URL), NewClient(nil)), srv
}

// 解析真实规则（某影视源 是 JSONC：带 // 注释）。
func TestParseXYQJSONC(t *testing.T) {
	const raw = `{
  "规则名": "某影视站",
  // "DEBUG":"1",
  "分类名称": "电影&剧集",
  "分类名称替换词": "1&2",
  "分类链接": "https://wogg.net/vodshow/{cateId}--{by}------{catePg}---{year}.html",
  "分类列表数组规则": ".module-items&&.module-item:not(:matches(某推荐源|公众号))"
}`
	r, err := ParseXYQ([]byte(raw))
	if err != nil {
		t.Fatalf("解析 JSONC 规则失败: %v", err)
	}
	if r.CategoryNames != "电影&剧集" || r.CategoryIDs != "1&2" {
		t.Fatalf("解析结果异常: %+v", r)
	}
}

func TestParseXYQRejectsEmpty(t *testing.T) {
	if _, err := ParseXYQ([]byte(`{"分类链接":""}`)); err == nil {
		t.Fatal("缺少分类链接应报错")
	}
}

// csp_XYQHiker 与 csp_XYQHikerAL 两种类名都要能路由到同一引擎。
func TestSupportedXYQHiker(t *testing.T) {
	for _, class := range []string{"csp_XYQHiker", "csp_XYQHikerAL", "csp_xyqhiker"} {
		if !Supported(class) {
			t.Errorf("Supported(%q) = false，期望 true", class)
		}
	}
}

func TestNewEngineXYQHiker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"首页推荐链接":"https://wogg.net","分类名称":"电影&剧集","分类名称替换词":"1&2","分类链接":"https://wogg.net/vodshow/{cateId}--{by}------{catePg}---{year}.html"}`))
	}))
	defer srv.Close()
	for _, class := range []string{"csp_XYQHiker", "csp_XYQHikerAL"} {
		got, err := NewEngine(context.Background(), class, srv.URL)
		if err != nil || got == nil {
			t.Fatalf("NewEngine(%s) = %v, %v", class, got, err)
		}
	}
}

func TestXYQHome(t *testing.T) {
	e, srv := xyqFixture(t)
	defer srv.Close()
	secs, err := e.Home(context.Background())
	if err != nil || len(secs) != 2 {
		t.Fatalf("Home = %#v, %v", secs, err)
	}
	if secs[0].ID != "1" || secs[0].Title != "电影" {
		t.Fatalf("首分类 = %#v", secs[0])
	}
}

func TestXYQBrowse(t *testing.T) {
	e, srv := xyqFixture(t)
	defer srv.Close()
	page, err := e.Browse(context.Background(), "1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("Browse 项数 = %d，期望 2（广告项应被滤掉）", len(page.Items))
	}
	first := page.Items[0]
	if first.Title != "片名A" || first.ID != srv.URL+"/d/1.html" || first.Logo != srv.URL+"/img/a.jpg" {
		t.Fatalf("首项 = %#v", first)
	}
}

func TestXYQSearch(t *testing.T) {
	e, srv := xyqFixture(t)
	defer srv.Close()
	items, err := e.Search(context.Background(), "重器")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "片名A" || items[0].ID != srv.URL+"/d/1.html" {
		t.Fatalf("Search = %#v", items)
	}
}

func TestXYQDetail(t *testing.T) {
	e, srv := xyqFixture(t)
	defer srv.Close()
	// 列表里给的是 /d/1.html，但 server 以 detail1 路径返回详情页
	v, err := e.Detail(context.Background(), srv.URL+"/detail1.html")
	if err != nil {
		t.Fatal(err)
	}
	// 演员取 .video-info-actor,1 → 第二个演员；简介取 p,-1 → 第二段
	if v.Content != "第二部简介" {
		t.Fatalf("简介 = %q，期望 .video-info-content p,-1 第二段", v.Content)
	}
	if v.PlayFrom != "线路A$$$线路B" {
		t.Fatalf("线路 = %q，期望 线路A$$$线路B", v.PlayFrom)
	}
	want := "第1集$https://www.aliyundrive.com/s/abc1#第2集$https://www.aliyundrive.com/s/abc2$$$第1集$https://www.aliyundrive.com/s/abc3"
	if v.PlayURL != want {
		t.Fatalf("PlayURL = %q，期望 %q", v.PlayURL, want)
	}
}

// 无选集时应保留线路名（某影视源 的 :matches 过滤条件与页面分享链接格式不符）。
// 选集规则全部不匹配时不能连线路名一起清空（某影视源 的 :matches 过滤条件
// 与页面分享链接格式不符就是这种情形）。
func TestXYQDetailKeepsSourcesWhenNoEpisodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<div class="module-player-tab"><div class="module-tab-item"><span>线路A</span></div></div>
<div class="module-player-list"><div class="module-row-one">无匹配链接</div></div>
</body></html>`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:         srv.URL,
		PlayerTabArray:  ".module-player-tab&&.module-tab-item",
		PlayerTabTitle:  "span&&Text",
		PlayerListArray: "body&&.module-player-list",
		PlayArray:       ".module-row-one:matches(https://www.ali(pan|yundrive).com/s/)",
		PlayTitle:       "h4&&Text",
		PlayLink:        ".btn-down&&a&&href",
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/d/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if v.PlayFrom != "线路A" {
		t.Fatalf("无选集时应保留线路名，得 %q", v.PlayFrom)
	}
}

// 非 JSoup 详情（详情是否Jsoup写法="0"）：字段可以是 "A&&B" 标记截取
// （qkan 的 演员详情="主演：</span>&&</li>" 即此类），也可以是 XPath。
func TestXYQDetailMarkedExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div class="fed-info">
<p><span>简介：</span>这是第一部简介</p>
<p><span>主演：</span><a href="/a">张甲</a></p>
</div></body></html>`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:    srv.URL,
		DetailMode: "0",
		Actor:      `主演：</span>&&</li>`,
		Content:    `简介：</span>&&</p>`,
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/d/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "这是第一部简介" {
		t.Fatalf("标记截取简介 = %q，期望「这是第一部简介」", v.Content)
	}
}

// JSON 响应（某搜索源 模式）：搜索截取模式 0，列表数组/标题/链接是 JSON 路径，
// POST 请求体里 {wd} 替换。
func TestXYQSearchJSONResponse(t *testing.T) {
	var gotBody, gotMethod, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		_, _ = w.Write([]byte(`{"code":200,"data":[{"title":"驱动一下","key":"xyz","img":"","desc":""}]}`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:        srv.URL,
		SearchURL:      srv.URL + "/v3/ali/search;post",
		PostBody:       `{"pageNum":1,"pageSize":30,"title":"{wd}","root":1,"cat":"all"}`,
		SearchArray:    "data",
		SearchTitle:    "title",
		SearchLink:     "key",
		SearchPrefix:   "https://www.aliyundrive.com/s/",
		SearchModeJSON: "0",
	}, NewClient(nil))

	items, err := e.Search(context.Background(), "重器")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" {
		t.Fatalf("JSON 搜索应 POST，实际 %s", gotMethod)
	}
	if !strings.Contains(gotBody, "重器") {
		t.Fatalf("请求体未含 {wd} 替换值: %q", gotBody)
	}
	if !strings.Contains(gotCT, "application/x-www-form-urlencoded") && !strings.Contains(gotCT, "application/json") && gotCT == "" {
		t.Fatalf("Content-Type 缺失")
	}
	if len(items) != 1 || items[0].Title != "驱动一下" {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ID != "https://www.aliyundrive.com/s/xyz" {
		t.Fatalf("item.ID = %q，期望加前缀", items[0].ID)
	}
}

// 存在 {SearchPg} / {catePg} 占位符时替换为页码。
func TestXYQExpandURL(t *testing.T) {
	got := xyqExpandURL("/vodshow/{cateId}--{by}------{catePg}---{year}.html", "5", 3)
	want := "/vodshow/5--------3---.html"
	if got != want {
		t.Fatalf("展开 = %q，期望 %q", got, want)
	}
}

// 搜索链接尾部的 ;post / ;get 是方法声明，不能当作 URL 的一部分。
func TestSplitMethodSuffix(t *testing.T) {
	cases := []struct {
		in       string
		wantURL  string
		wantPost bool
	}{
		{"https://a/s;post", "https://a/s", true},
		{"https://a/s;get", "https://a/s", false},
		{"https://a/s?x=1", "https://a/s?x=1", false},
	}
	for _, c := range cases {
		gotURL, gotPost := splitMethodSuffix(c.in)
		if gotURL != c.wantURL || gotPost != c.wantPost {
			t.Errorf("splitMethodSuffix(%q) = (%q,%v)，期望 (%q,%v)", c.in, gotURL, gotPost, c.wantURL, c.wantPost)
		}
	}
}

// [firstPage=...] 语法：第一页去 firstPage，第二页起走主链接拼页码。
func TestResolveCategoryURLFirstPage(t *testing.T) {
	e := NewXYQHikerEngine(&XYQ{
		CategoryURL: "https://s/browse/{cateId}/{catePg}[firstPage=https://s/browse/{cateId}]",
	}, NewClient(nil))
	if got := e.resolveCategoryURL("0", 1); got != "https://s/browse/0" {
		t.Fatalf("第一页 = %q，期望 https://s/browse/0", got)
	}
	if got := e.resolveCategoryURL("0", 2); got != "https://s/browse/0/2" {
		t.Fatalf("第二页 = %q，期望 https://s/browse/0/2", got)
	}
}

// 无 [firstPage] 时行为保持原样。
func TestResolveCategoryURLPlain(t *testing.T) {
	e := NewXYQHikerEngine(&XYQ{
		CategoryURL: "https://s/vodshow/{cateId}--{by}------{catePg}---{year}.html",
	}, NewClient(nil))
	if got := e.resolveCategoryURL("5", 3); got != "https://s/vodshow/5--------3---.html" {
		t.Fatalf("普通分类链接展开 = %q", got)
	}
}
