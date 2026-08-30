package crawler

import "testing"

func TestParseClasses(t *testing.T) {
	html := `<ul><li><a href="/list/movie.html">电影</a></li><li><a href="/list/tv.html">电视剧</a></li></ul>`
	cls, err := parseClasses(html, `li&&a&&Text;li&&a&&Text;li&&a&&href;/(\w+)\.html`)
	if err != nil || len(cls) != 2 || cls[0].TypeID != "movie" || cls[0].TypeName != "电影" {
		t.Fatalf("cls=%+v err=%v", cls, err)
	}
}
