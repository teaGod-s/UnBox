package rule

import (
	"context"
	"strings"
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

// 真实规则里存在 "\&" 这类非标准 JSON 转义；整份规则不应因此作废。
func TestParseXBPQToleratesInvalidJSONEscape(t *testing.T) {
	raw := []byte(`{"主页url":"https://a/","分类url":"https://a/list","标题":"a\&b"}`)
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatalf("非标准转义不应导致解析失败: %v", err)
	}
	if r.Title != `a\&b` {
		t.Fatalf("Title = %q，期望保留反斜杠字面量", r.Title)
	}
}

// 真实规则的 请求头 既可能是对象，也可能是 "User-Agent$MOBILE_UA" 这种字符串。
func TestParseXBPQHeadersAsString(t *testing.T) {
	raw := []byte(`{"主页url":"https://a/","分类url":"https://a/list","请求头":"User-Agent$MOBILE_UA#Referer$https://a/"}`)
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Headers["User-Agent"], "Mozilla/5.0 (Linux; Android 11") {
		t.Fatalf("MOBILE_UA 未展开: %q", r.Headers["User-Agent"])
	}
	if r.Headers["Referer"] != "https://a/" {
		t.Fatalf("Referer = %q", r.Headers["Referer"])
	}
}

func TestParseXBPQHeadersAsObject(t *testing.T) {
	raw := []byte(`{"主页url":"https://a/","分类url":"https://a/list","请求头":{"User-Agent":"UA"}}`)
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Headers["User-Agent"] != "UA" {
		t.Fatalf("对象型请求头解析失败: %#v", r.Headers)
	}
}
