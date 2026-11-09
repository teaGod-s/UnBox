package crawler

import "testing"

// 二级 js: 脚本把数字直接赋给 VOD 的字符串字段（如博看听书
// `VOD.vod_name = data.list[0].id`，id=5690048）时，JSON 里出现数字值，
// 直接 json.Unmarshal 到 Detail 的 string 字段会报
// "cannot unmarshal number into Go struct field ... of type string"。
// JS 语义要求 number → 十进制字符串（"5690048"），引擎必须容错。
func TestExportDetailNumberField(t *testing.T) {
	raw := `{"vod_name":5690048,"vod_id":"3331822","vod_play_from":"bookan","vod_play_url":"01.$http://a.m4a#02.$http://b.m4a"}`
	detail, flow := lenientDetailFromJSON([]byte(raw))
	if flow != "" {
		t.Logf("flow: %s", flow)
	}
	if detail == nil {
		t.Fatal("detail is nil")
	}
	if detail.VodName != "5690048" {
		t.Fatalf("VodName = %q, want %q", detail.VodName, "5690048")
	}
	if detail.VodID != "3331822" {
		t.Fatalf("VodID = %q", detail.VodID)
	}
	if detail.VodPlayURL != "01.$http://a.m4a#02.$http://b.m4a" {
		t.Fatalf("VodPlayURL = %q", detail.VodPlayURL)
	}
}

// 数组包裹形态 {list:[...]} 同样要容忍数字字段。
func TestExportDetailNumberFieldEnvelope(t *testing.T) {
	raw := `{"list":[{"vod_name":10086,"vod_play_url":""}]}`
	detail, flow := lenientDetailFromJSON([]byte(raw))
	if flow != "" {
		t.Logf("flow: %s", flow)
	}
	if detail == nil {
		t.Fatal("detail is nil")
	}
	if detail.VodName != "10086" {
		t.Fatalf("VodName = %q, want %q", detail.VodName, "10086")
	}
}
