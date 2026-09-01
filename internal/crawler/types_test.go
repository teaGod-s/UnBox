package crawler

import "testing"

func TestDetailPromotesVodContent(t *testing.T) {
	detail := Detail{Vod: Vod{VodContent: "简介"}}
	if detail.VodContent != "简介" {
		t.Fatalf("Detail.VodContent = %q, want %q", detail.VodContent, "简介")
	}
}
