package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 动漫巴士：分类片单图片 ".lazy&&data-bg"，data-bg 属性必须在取值属性表里。
func TestXYQBrowseDataBG(t *testing.T) {
	const page = `<html><body><div class="v_list">
<li><div class="lazy" data-bg="https://cdn.example.com/pic/a.jpg"></div><a title="甲" href="/d/1.html">甲</a></li>
</div></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()
	e := NewXYQHikerEngine(&XYQ{
		HomeURL:       srv.URL,
		CategoryURL:   srv.URL + "/list-{cateId}-{catePg}.html",
		CategoryArray: ".v_list&&li",
		CatTitle:      "a&&title",
		CatLink:       "a&&href",
		CatPic:        ".lazy&&data-bg",
		LinkPrefix:    srv.URL,
		Reverse:       "0",
	}, NewClient(nil))
	pg, err := e.Browse(context.Background(), "1", 1)
	if err != nil || len(pg.Items) != 1 {
		t.Fatalf("Browse: %v items=%d", err, len(pg.Items))
	}
	if pg.Items[0].Logo != "https://cdn.example.com/pic/a.jpg" {
		t.Fatalf("Logo = %q，期望 data-bg 直链", pg.Items[0].Logo)
	}
}

// 去看吧动漫：详情页海报按「分类片单图片」规则提取。
// 真实结构为 <a class="fed-list-pics" data-original="...">，data-original 在 a 自身。
func TestXYQDetailPicFromRule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<a class="fed-list-pics" href="/d/1.html" data-original="https://img.qkan.com/poster.jpg"></a>
</body></html>`))
	}))
	defer srv.Close()
	e := NewXYQHikerEngine(&XYQ{
		HomeURL: srv.URL,
		CatPic:  ".fed-list-pics&&data-original",
	}, NewClient(nil))
	v, err := e.Detail(context.Background(), srv.URL+"/d/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if v.Pic != "https://img.qkan.com/poster.jpg" {
		t.Fatalf("详情海报 = %q，期望规则提取 data-original 直链", v.Pic)
	}
}
