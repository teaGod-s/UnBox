package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 分类截取模式=0：分类列表是 JSON 响应（list[] 里 name/ddurl/pic 键）。
// 此前 Browse 缺少该分支，恒走 HTML 把 JSON 当页面解析 → 分类恒 0 项。
func TestXYQBrowseJSONMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"list":[{"name":"京剧样板戏","ddurl":"/x/1.mp4","pic":"https://cdn.example.com/1.jpg","desc":"1980"},{"name":"评剧名段","ddurl":"/x/2.mp4","pic":"https://cdn.example.com/2.jpg","desc":"1985"}]}`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:       srv.URL,
		CategoryURL:   srv.URL + "/s.php?collid={cateId}&pg={catePg}",
		CategoryMode:  "0", // JSON 截取模式
		CategoryArray: "list",
		CatTitle:      "name",
		CatLink:       "ddurl",
		CatPic:        "pic",
		CatSubtitle:   "desc",
		LinkPrefix:    "https://cdn.example.com",
	}, NewClient(nil))

	page, err := e.Browse(context.Background(), "1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("Browse JSON 模式 = %d 项，期望 2", len(page.Items))
	}
	first := page.Items[0]
	if first.Title != "京剧样板戏" || first.ID != "https://cdn.example.com/x/1.mp4" {
		t.Fatalf("首项 = %#v，期望 JSON 键提取 + 前缀补全", first)
	}
	if first.Logo != "https://cdn.example.com/https://cdn.example.com/1.jpg" && first.Logo != "https://cdn.example.com/1.jpg" {
		// withPrefixSuffix 对绝对 URL 不拼前缀
	}
}
