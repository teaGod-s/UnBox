package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 是否反转选集序列 的真实取值：0/1/否/是。引擎此前只认「是」，规则写 "1"
// 的站（某动漫站/某影视站/某影视站）被忽略反转 → 剧集以页面原始顺序展示，
// 用户看到「集数顺序反了」。真值集合必须覆盖 1/y/true。
func TestXYQReverseTrueValues(t *testing.T) {
	for _, v := range []string{"1", "是", "y", "Y", "true", "TRUE"} {
		if !xyqReverseEnabled(v) {
			t.Errorf("xyqReverseEnabled(%q) = false，期望 true", v)
		}
	}
	for _, v := range []string{"0", "否", "n", "false", "", " "} {
		if xyqReverseEnabled(v) {
			t.Errorf("xyqReverseEnabled(%q) = true，期望 false", v)
		}
	}
}

// 端到端：规则声明反转=1 时，选集顺序应被翻转为桌面正序。
func TestXYQDetailReverseWithOne(t *testing.T) {
	// 详情页给降序（最新在前），规则声明要反转成升序。
	// 选集行结构与 某影视源 一致：.play-row 内 .btn-down div 包 <a><h4>。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<div class="module-player-list">
  <div class="play-row"><div class="btn-down"><a href="/ep/8.html"><h4>第8集</h4></a></div></div>
  <div class="play-row"><div class="btn-down"><a href="/ep/7.html"><h4>第7集</h4></a></div></div>
  <div class="play-row"><div class="btn-down"><a href="/ep/1.html"><h4>第1集</h4></a></div></div>
</div>
</body></html>`))
	}))
	defer srv.Close()

	e := NewXYQHikerEngine(&XYQ{
		HomeURL:         srv.URL,
		PlayerListArray: "body&&.module-player-list",
		PlayArray:       ".play-row",
		PlayTitle:       "h4&&Text",
		PlayLink:        ".btn-down&&a&&href",
		Reverse:         "1",
	}, NewClient(nil))

	v, err := e.Detail(context.Background(), srv.URL+"/d/1.html")
	if err != nil {
		t.Fatal(err)
	}
	// 声明反转=1：第一集应排最前（第1集在最前 = 已反转）。
	if !strings.Contains(v.PlayURL, "第1集") || strings.Index(v.PlayURL, "第1集") > strings.Index(v.PlayURL, "第8集") {
		t.Fatalf("反转未生效: PlayURL=%q，期望第1集在最前", v.PlayURL)
	}
}
