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

func TestParseClassesRequiresFourSegments(t *testing.T) {
	if _, err := parseClasses(`<li>x</li>`, `li;Text;href`); err == nil {
		t.Fatal("expected error for missing class_parse segment")
	}
	if _, err := parseClasses(`<li>x</li>`, `li;Text;href;/(x)/;extra`); err == nil {
		t.Fatal("expected error for extra class_parse segment")
	}
}
