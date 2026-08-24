package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientVideolist(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Query().Get("ac") != "videolist" {
			t.Errorf("ac = %q, 期望 videolist", r.URL.Query().Get("ac"))
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":98823,"vod_name":"狂怒追缉","vod_pic":"http://x/p.jpg","type_id":16,"type_name":"欧美剧"}]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	items, err := c.videolist(context.Background(), "16", "", 0)
	if err != nil {
		t.Fatalf("videolist 失败: %v", err)
	}
	if len(items) != 1 || items[0].VodName != "狂怒追缉" || items[0].TypeName != "欧美剧" {
		t.Fatalf("解析错误: %+v", items)
	}
	if gotPath != "/" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestClientBaseStripsQuery(t *testing.T) {
	// 无水印采集站点的 api 内嵌了 ?ac=list，须剥离后再拼参数。
	c := newClient("https://api.example.com/api.php/provide/vod/?ac=list")
	if c.base != "https://api.example.com/api.php/provide/vod/" {
		t.Fatalf("base 未剥离 query: %q", c.base)
	}
}

func TestClientDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") != "detail" || r.URL.Query().Get("ids") != "98823" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":98823,"vod_name":"狂怒追缉","vod_play_from":"feifan$$$ffm3u8","vod_play_url":"第01集$a#第02集$b"}]}`))
	}))
	defer srv.Close()

	v, err := newClient(srv.URL).detail(context.Background(), "98823")
	if err != nil {
		t.Fatalf("detail 失败: %v", err)
	}
	if v.VodPlayFrom != "feifan$$$ffm3u8" {
		t.Fatalf("detail 解析错误: %+v", v)
	}
}
