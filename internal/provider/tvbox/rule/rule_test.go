package rule

import (
	"context"
	"testing"
)

func TestParseXBPQ(t *testing.T) {
	raw := []byte(`{
		"主页url":"https://a/",
		"分类":"电影$1#剧集$2",
		"分类url":"https://a/vod/{cateId}/page/{catePg}.html",
		"数组":"class=\"item\"&&</li>",
		"标题":"title=\"&&\"",
		"链接":"href=\"&&\"",
		"筛选":{"1":[{"key":"cateId","name":"类型","value":[{"n":"全部","v":"1"}]}]}
	}`)
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.HomeURL != "https://a/" || r.Classes != "电影$1#剧集$2" {
		t.Fatalf("解析结果 = %+v", r)
	}
	if len(r.Filters["1"]) != 1 || r.Filters["1"][0].Values[0].Value != "1" {
		t.Fatalf("筛选解析 = %+v", r.Filters)
	}
}

func TestParseXBPQRejectsEmpty(t *testing.T) {
	if _, err := ParseXBPQ([]byte(`{}`)); err == nil {
		t.Fatal("空规则应返回错误")
	}
}

func TestSupported(t *testing.T) {
	if !Supported("csp_XBPQ") || Supported("csp_Unknown") {
		t.Fatalf("Supported 结果不符")
	}
}

func TestNewEngineRejectsUnknown(t *testing.T) {
	if _, err := NewEngine(context.Background(), "csp_Unknown", "https://x/r.json"); err == nil {
		t.Fatal("未知类应返回错误")
	}
}
