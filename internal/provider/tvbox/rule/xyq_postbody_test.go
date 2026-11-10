package rule

import (
	"encoding/json"
	"strings"
	"testing"
)

// 真实规则（某直播源直播）的 POST请求数据 是对象 {"sk":"{wd}","offset":0,...}，
// 不是字符串。XYQ.PostBody 是 FlexString，直接 Unmarshal 不会把整站规则判废，
// 对象形态会被序列化成紧凑 JSON 字符串。
func TestParseXYQPostBodyObject(t *testing.T) {
	const raw = `{
  "规则名": "某直播源直播",
  "首页推荐链接": "https://www.douyu.com",
  "搜索链接": "https://api.example/search",
  "POST请求数据": {"sk": "{wd}", "offset": 0, "limit": 20}
}`
	r, err := ParseXYQ([]byte(raw))
	if err != nil {
		t.Fatalf("对象形态 POST请求数据 应能解析: %v", err)
	}
	got := string(r.PostBody)
	if got == "" {
		t.Fatal("PostBody 为空")
	}
	// 转成 JSON 字符串后应保留 {wd} 占位符，供 Search 替换。
	if !containsStr(got, `{wd}`) {
		t.Fatalf("PostBody = %q，应含 {wd} 占位符", got)
	}
	// 必须是合法 JSON（对象被紧凑序列化了）。
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("PostBody = %q 不是合法 JSON: %v", got, err)
	}
	sk, _ := obj["sk"].(string)
	if sk != "{wd}" {
		t.Fatalf("obj.sk = %q，期望 {wd}", sk)
	}
	// {wd} 替换仍应工作：Search 构造 POST 体时把占位符换成关键词。
	replaced := strings.ReplaceAll(got, "{wd}", "火影")
	if !containsStr(replaced, `"火影"`) {
		t.Fatalf("替换失败: %q", replaced)
	}
}

// 字符串形态（某搜索源等）不能回归。
func TestParseXYQPostBodyString(t *testing.T) {
	const raw = `{"首页推荐链接":"https://x.com","搜索链接":"https://x/s;post","POST请求数据":"{\"key\":\"{wd}\"}"}`
	r, err := ParseXYQ([]byte(raw))
	if err != nil {
		t.Fatalf("字符串 POST请求数据 应能解析: %v", err)
	}
	if string(r.PostBody) != `{"key":"{wd}"}` {
		t.Fatalf("PostBody = %q，期望原样保留", string(r.PostBody))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
