package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLenientPreservesURLSlashes(t *testing.T) {
	in := []byte(`{
//数据接口
"jxurl":"http://jx.84jia.com/x.php?url=",
"api":"https://example.com//double"
}`)
	out := Lenient(in)
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("清洗后仍无法解析: %v\n输出: %s", err, out)
	}
	if m["jxurl"] != "http://jx.84jia.com/x.php?url=" {
		t.Errorf("URL 被破坏: %q", m["jxurl"])
	}
	if m["api"] != "https://example.com//double" {
		t.Errorf("URL 中的双斜杠被破坏: %q", m["api"])
	}
}

func TestLenientStripsBlockComment(t *testing.T) {
	in := []byte(`{/* 说明 */"a":1}`)
	var m map[string]int
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("块注释未被剥离: %v", err)
	}
	if m["a"] != 1 {
		t.Errorf("got %d, want 1", m["a"])
	}
}

func TestLenientStripsTrailingComma(t *testing.T) {
	in := []byte(`{"a":[1,2,3,],"b":{"c":1,},}`)
	var m map[string]any
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("尾随逗号未被处理: %v", err)
	}
}

func TestLenientEscapesControlChars(t *testing.T) {
	// 字符串内的裸制表符/换行是非法 JSON，实测 4 个真实样本含此问题
	in := []byte("{\"name\":\"ab\tcd\"}")
	var m map[string]string
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("控制字符未被转义: %v", err)
	}
	if !strings.Contains(m["name"], "ab") {
		t.Errorf("内容丢失: %q", m["name"])
	}
}

func TestLenientKeepsCommentMarkerInsideString(t *testing.T) {
	in := []byte(`{"note":"这里有 // 和 /* 不是注释"}`)
	var m map[string]string
	if err := json.Unmarshal(Lenient(in), &m); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if m["note"] != "这里有 // 和 /* 不是注释" {
		t.Errorf("字符串内的注释标记被误删: %q", m["note"])
	}
}

func TestLenientTrailingCommaWithLineCommentBetween(t *testing.T) {
	// Regression test: trailing comma followed by line comment should be stripped
	in := []byte(`[1,2, // comment
	]`)
	out := Lenient(in)
	var arr []int
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("尾随逗号+注释处理失败: %v\n输出: %s", err, out)
	}
	if len(arr) != 2 || arr[0] != 1 || arr[1] != 2 {
		t.Errorf("数组值被破坏: %v", arr)
	}
}

func TestLenientBackslashFollowedByControlChar(t *testing.T) {
	// Regression test: backslash followed by raw control char should be properly escaped
	in := []byte("{\"name\":\"x\\\tvalue\"}")
	out := Lenient(in)
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("反斜杠+控制字符处理失败: %v\n输出: %s", err, out)
	}
	// The backslash before the tab should allow proper JSON parsing
	if !strings.Contains(m["name"], "x") {
		t.Errorf("字符串内容被破坏: %q", m["name"])
	}
}
