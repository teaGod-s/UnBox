package crawler

import "testing"

// 芒果列表规则 `json:data.hitDocs;title;img;updateInfo||rightCorner.text;playPartId`
// 的 id 段是 playPartId（站点字段名）。canonicalVodField 与 isJSONIDField、
// setVodField 必须把它当 id 处理，否则列表项 VodID 为空 → 详情 URL 失效
// （百忙无果线上根因之一）。
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
}
