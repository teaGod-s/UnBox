package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MacPlayer 播放页的 url 带 \/ 转义（看影网/360影视等），旧 findMediaURL
// 直接匹配不到 → Resolve 把播放页 URL 原样交出，编排层也解析失败，
// 报「播放页中未找到真实播放地址」。macPlayerURL 应解码转义后取到 m3u8。
func TestXYQResolveMacPlayerEscapedURL(t *testing.T) {
	const playPage = `<html><body>
<script>var player_aaaa={"flag":"play","url":"https:\/\/cdn.example.com\/2022\/a\/index.m3u8","from":"m3u8"}</script>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(playPage))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{HomeURL: srv.URL}, NewClient(nil))
	stream, err := e.Resolve(context.Background(), srv.URL+"/yun/1/1/")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://cdn.example.com/2022/a/index.m3u8"
	if stream.URL != want {
		t.Fatalf("Resolve = %q，期望解出转义 m3u8 %q", stream.URL, want)
	}
}

// 普通播放页（无转义）不能回归。
func TestXYQResolvePlainMediaURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>var x="https://cdn.example.com/a.mp4"</body></html>`))
	}))
	defer srv.Close()
	e := NewXYQHikerEngine(&XYQ{HomeURL: srv.URL}, NewClient(nil))
	stream, err := e.Resolve(context.Background(), srv.URL+"/p/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if stream.URL != "https://cdn.example.com/a.mp4" {
		t.Fatalf("Resolve = %q，期望明文 mp4", stream.URL)
	}
}
