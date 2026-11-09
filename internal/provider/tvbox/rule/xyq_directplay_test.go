package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// JSON 截取模式的 分类片单链接（ddurl）直接是 mp4 直链。
// Detail 不能把直链当详情页 fetch（会下载整个视频，4MB+ 超限报错），
// 应直接作为可播放剧集返回。
func TestXYQDetailMediaDirectLinkNoFetch(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{HomeURL: srv.URL}, NewClient(nil))
	mediaURL := srv.URL + "/v/200929/abc.mp4"
	v, err := e.Detail(context.Background(), mediaURL)
	if err != nil {
		t.Fatalf("直链 Detail 不应 fetch: %v (called=%v)", err, called)
	}
	if called {
		t.Fatal("直链被 fetch 了（这就是 4MB 超限根因）")
	}
	if !strings.Contains(v.PlayURL, mediaURL) {
		t.Fatalf("PlayURL = %q，应含直链地址", v.PlayURL)
	}
}

// 规则声明 链接是否直接播放=是 时，走直连路径。
func TestXYQDetailDirectPlayFlag(t *testing.T) {
	e := NewXYQHikerEngine(&XYQ{HomeURL: "https://x", DirectPlay: "是"}, NewClient(nil))
	v, err := e.Detail(context.Background(), "https://x/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.PlayURL, "video.mp4") {
		t.Fatalf("PlayURL = %q", v.PlayURL)
	}
}
