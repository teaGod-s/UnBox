package crawler

import (
	"strings"
	"testing"
)

// json: 列表规则碰到 JSONP 响应（`_callback({...})`）必须剥壳再解析。
// 网易公版影像 active.163.com 的 `.jsonp` 接口返回
// `_callback({"status":"success","list":[...]})`，原 TVBox 引擎会剥掉
// callback 包裹；本引擎此前直接 json.Unmarshal → "invalid character '_'"。
func TestExtractJSONVodsJSONP(t *testing.T) {
	raw := `_callback({"status":"success","list":[
		{"id":1931,"cover_pic":"http://a.jpg","title":"黑猫","media_type":"film"},
		{"id":1932,"cover_pic":"http://b.jpg","title":"白狗","media_type":"film"}
	]})`
	// 规则：json:list;title;cover_pic;media_type;id —— 与网易公版影像脚本一致。
	fieldRule := "list;title;cover_pic;media_type;id"
	vods, err := extractJSONVods(raw, "json:"+fieldRule)
	if err != nil {
		t.Fatalf("extractJSONVods 遇到 JSONP 失败: %v", err)
	}
	if len(vods) != 2 {
		t.Fatalf("解析出 %d 项, want 2", len(vods))
	}
	if vods[0].VodName != "黑猫" || vods[0].VodID != "1931" {
		t.Fatalf("第一项 = %+v, want 黑猫/1931", vods[0])
	}
	if vods[1].VodID != "1932" || vods[1].VodPic != "http://b.jpg" {
		t.Fatalf("第二项 = %+v, want 1932/b.jpg", vods[1])
	}
	// 纯 JSON 不受影响。
	plain, err := extractJSONVods(`{"list":[{"id":1,"title":"x"}]}`, "json:list;title;id")
	if err != nil || len(plain) != 1 || plain[0].VodID != "1" {
		t.Fatalf("纯 JSON 回归异常: %v %+v", err, plain)
	}
}

// 网易公版的真实响应末尾带分号：`_callback({...});` —— stripJSONP 必须容忍。
func TestExtractJSONVodsJSONPTrailingSemi(t *testing.T) {
	raw := `_callback({"status":"success","list":[{"id":1931,"cover_pic":"http://a.jpg","title":"黑猫","media_type":"film"}]});`
	vods, err := extractJSONVods(raw, "json:list;title;cover_pic;media_type;id")
	if err != nil {
		t.Fatalf("带尾分号的 JSONP 失败: %v", err)
	}
	if len(vods) != 1 || vods[0].VodID != "1931" || vods[0].VodName != "黑猫" {
		t.Fatalf("vods = %+v, want [1931 黑猫]", vods)
	}
}

// 剥壳要处理连字符等宽松回调名，但 HTML 仍应报「网页」错误。
func TestExtractJSONVodsJSONPNotHTML(t *testing.T) {
	_, err := extractJSONVods(`<html><body>busy</body></html>`, "json:list;title;id")
	if err == nil || !strings.Contains(err.Error(), "网页") {
		t.Fatalf("HTML 应报网页错误，got %v", err)
	}
}
