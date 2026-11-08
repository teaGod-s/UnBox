package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 图片是绝对 CDN 直链时不能在前缀拼站点域名：
// prefix(站点域名)+logo(绝对URL) = "https://站https://cdn/..." 畸形，图片加载不出来。
func TestXYQBrowseAbsoluteLogoNoDoublePrefix(t *testing.T) {
	const page = `<html><body><div class="module-items">
<div class="module-item"><a title="片A" href="/d/1.html"><img data-src="https://cdn.example.com/pic/a.jpg"></a></div>
</div></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:       srv.URL,
		CategoryURL:   srv.URL + "/vodshow/{cateId}--{by}------{catePg}---{year}.html",
		CategoryArray: ".module-items&&.module-item",
		CatTitle:      "a&&title",
		CatLink:       "a&&href",
		CatPic:        "img&&data-src",
		LinkPrefix:    srv.URL, // 真实规则把站点域名当前缀
	}, NewClient(nil))

	pg, err := e.Browse(context.Background(), "1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pg.Items) != 1 {
		t.Fatalf("items=%d", len(pg.Items))
	}
	got := pg.Items[0].Logo
	want := "https://cdn.example.com/pic/a.jpg"
	if got != want {
		t.Fatalf("Logo = %q，期望 %q（不应带站点前缀双拼）", got, want)
	}
}
