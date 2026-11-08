package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// XYQHiker 引擎对 类型/年代/地区 详情字段从未填充（Vod 只给 ID/Name/Pic/Content），
// 前端详情 meta 行 {{Type}}·{{Year}}·{{Area}} 全空渲染成 " · ·"。
// 规则配置 类型详情/年代详情/地区详情 时必须生效。
func TestXYQDetailMetaFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<div class="v_info"><span class="type">奇幻</span><span class="year">2026</span><span class="area">大陆</span></div>
<div class="module-player-list"><div class="play-row">空选集</div></div>
</body></html>`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:    srv.URL,
		TypeDetail: ".v_info&&.type&&Text",
		YearDetail: ".v_info&&.year&&Text",
		AreaDetail: ".v_info&&.area&&Text",
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/d/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if v.TypeName != "奇幻" || v.Year != "2026" || v.Area != "大陆" {
		t.Fatalf("Detail meta = type=%q year=%q area=%q，期望 奇幻/2026/大陆", v.TypeName, v.Year, v.Area)
	}
}
