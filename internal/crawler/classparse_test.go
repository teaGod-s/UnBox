package crawler

import "testing"

func TestParseClasses(t *testing.T) {
	html := `<ul><li><a href="/list/movie.html">电影</a></li><li><a href="/list/tv.html">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;/(\w+)\.html`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "movie" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesUsesSelectedEntries(t *testing.T) {
	html := `<div><a href="/other.html">无关</a></div><ul><li><a href="/list/movie.html">电影</a></li><li><a href="/list/tv.html">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;/(\w+)\.html`)
	if err != nil || len(cls) != 2 || cls[0].TypeName != "电影" || cls[0].TypeID != "movie" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesRequiresSegments(t *testing.T) {
	// 2 段太少了。
	if _, err := parseClasses(`<li>x</li>`, `li;Text`); err == nil {
		t.Fatal("expected error for missing class_parse segment")
	}
	// 5 段太多。
	if _, err := parseClasses(`<li>x</li>`, `li;Text;href;/(x)/;extra`); err == nil {
		t.Fatal("expected error for extra class_parse segment")
	}
}

// 三段省略 id 正则：很多 drpy2 真实脚本（如 B站影视）的 id 规则已直接
// 取出值，不必再配 /…/ 提取正则。
func TestParseClassesThreeSegmentsWithoutRegexp(t *testing.T) {
	html := `<ul><li><a href="/movie.html" data-id="17">电影</a></li><li><a href="/tv.html" data-id="18">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li;a&&Text;a&&data-id`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "17" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}

func TestParseClassesSupportsJQueryLimit(t *testing.T) {
	html := `<ul class="nav"><li><a href="/movie.html">电影</a></li><li><a href="/tv.html">电视剧</a></li><li><a href="/anime.html">动漫</a></li></ul>`
	cls, err := parseClasses(html, `.nav&&li:lt(2);a&&Text;a&&href;/([a-z]+)\.html`)
	if err != nil || len(cls) != 2 || cls[1].TypeID != "tv" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}
