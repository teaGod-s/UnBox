package rule

import (
	"encoding/json"
	"strings"
	"testing"
)

// 真实规则（文才影视 wcys.json）播放数组里出现 "题链#\\#>>"：
// 两个反斜杠 + # 在 JSON 中本来是合法转义（\\ = 反斜杠字面量，# 普通字符）。
// sanitizeJSONEscapes 曾逐字节把第二个 \ 当成独立转义再加一对反斜杠，
// 输出 \\\# 反而制造 invalid escape sequence，整站规则解析失败。
func TestSanitizeDoesNotBreakDoubleBackslash(t *testing.T) {
	raw := []byte(`{"播放数组":"题链#\\#>>"}`)
	got := sanitizeJSONEscapes(raw)

	// 修复后：\\#（2 反斜杠+#）应保持原样，不能变成 \\\#（3 反斜杠）。
	if strings.Count(string(got), `\`) != 2 {
		t.Fatalf("sanitize 应保留 2 个反斜杠，得 %d：%q", strings.Count(string(got), `\`), string(got))
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("sanitize 后应能解析: %v (got=%q)", err, string(got))
	}
	if m["播放数组"] != `题链#\#>>` {
		t.Fatalf("解析值 = %q，期望 题链#\\#>>", m["播放数组"])
	}
}

// 单个 \#（真实规则里 \& 之类）仍应被修复成 \\#。
func TestSanitizeFixesSingleInvalidEscape(t *testing.T) {
	raw := []byte(`{"a":"x\&y"}`)
	got := sanitizeJSONEscapes(raw)
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("\\& 应被修复: %v (got=%q)", err, string(got))
	}
	if m["a"] != `x\&y` {
		t.Fatalf("值 = %q，期望 x\\&y（字面反斜杠+&）", m["a"])
	}
}

// wcys.json 真实组合：同一 JSON 里同时出现字符串内转义引号 \"、合法 \\、
// 非法 \&。状态机必须正确跟踪字符串边界，不能把转义引号当结束符。
func TestSanitizeWcysMixedEscapes(t *testing.T) {
	raw := []byte("{\"数组二次截取\": \"list\\\":[&&]\", \"标题\": \"x\\&y\", \"播放数组\": \"题链#\\\\#>>\"}")
	got := sanitizeJSONEscapes(raw)
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("混合转义应能解析: %v\n  sanitized=%q", err, string(got))
	}
	wantArray := "list\":[&&]"
	if m["数组二次截取"] != wantArray {
		t.Fatalf("数组二次截取 = %q，期望 %q", m["数组二次截取"], wantArray)
	}
	wantTitle := `x\&y`
	if m["标题"] != wantTitle {
		t.Fatalf("标题 = %q，期望 %q", m["标题"], wantTitle)
	}
	wantPlay := `题链#\#>>`
	if m["播放数组"] != wantPlay {
		t.Fatalf("播放数组 = %q，期望 %q", m["播放数组"], wantPlay)
	}
}
