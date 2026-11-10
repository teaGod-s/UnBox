package crawler

import "testing"

// 某影视源列表规则 `json:data.hitDocs;title;img;updateInfo||rightCorner.text;playPartId`
// 的 id 段是 playPartId（站点字段名）。canonicalVodField 与 isJSONIDField、
// setVodField 必须把它当 id 处理，否则列表项 VodID 为空 → 详情 URL 失效
// （某影视源线上根因之一）。
func TestPlayPartID(t *testing.T) {
	if !isJSONIDField("playPartId") {
		t.Fatal("isJSONIDField(playPartId) 应为 true")
	}
	var vod Vod
	setVodField(&vod, "playPartId", "24760359")
	if vod.VodID != "24760359" {
		t.Fatalf("VodID = %q, want 24760359", vod.VodID)
	}
	// 经典别名回归。
	if !isJSONIDField("id") {
		t.Fatal("isJSONIDField(id) 应为 true")
	}
	// vid 是 TVBox 通用 id 字段（某公版影像源规则 `json:list;…;vid`）。
	if !isJSONIDField("vid") {
		t.Fatal("isJSONIDField(vid) 应为 true")
	}
	var vod2 Vod
	setVodField(&vod2, "vid", "10086")
	if vod2.VodID != "10086" {
		t.Fatalf("setVodField(vid) = %q, want 10086", vod2.VodID)
	}
}
