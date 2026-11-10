package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dev云播（某云播源）等加密源：player_aaaa.url 是密文（非 http 明文），
// 真实地址由第三方云解析+混淆 JS 运行时生成。Resolve 不能静默把播放页
// URL 原样交出（编排层只会报误导性的「未找到真实播放地址」），
// 应返回带源类型的明确错误。
func TestXYQResolveCipherCloudError(t *testing.T) {
	const playPage = `<html><body><script>
var player_aaaa={"flag":"play","encrypt":0,"url":"1071_0bc3iyaaaaaabiamwiqcxjvc6rqeabdaaaca","from":"某云播源","server":"no","note":"17","id":"8044","sid":1};
</script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(playPage))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{HomeURL: srv.URL}, NewClient(nil))
	stream, err := e.Resolve(context.Background(), srv.URL+"/play/8044_1_1.html")
	if err == nil {
		t.Fatalf("加密云播源应返回错误，实际静默返回 stream=%+v", stream)
	}
	if !strings.Contains(err.Error(), "某云播源") && !strings.Contains(err.Error(), "加密") {
		t.Fatalf("错误应指明加密/云播源类型，得: %v", err)
	}
}

// 明文 m3u8（某影视源）不能回归：仍能正常解出。
func TestXYQResolveCipherDoesNotBreakPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<script>var player_aaaa={"flag":"play","url":"https:\/\/cdn.example.com\/index.m3u8","from":"qibom3u8"}</script>`))
	}))
	defer srv.Close()
	e := NewXYQHikerEngine(&XYQ{HomeURL: srv.URL}, NewClient(nil))
	stream, err := e.Resolve(context.Background(), srv.URL+"/play/1.html")
	if err != nil {
		t.Fatalf("明文播放页不应报错: %v", err)
	}
	if stream.URL != "https://cdn.example.com/index.m3u8" {
		t.Fatalf("URL = %q，期望明文 m3u8", stream.URL)
	}
}
