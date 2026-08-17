package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

func TestBuildReportCounts(t *testing.T) {
	cfgs := []*config.Config{
		{
			Sites: []config.Site{
				{Type: config.SiteTypeSpider, API: "csp_A"},
				{Type: config.SiteTypeSpider, API: "csp_B"},
				{Type: config.SiteTypeSpider, API: "http://x/d.js"},
				{Type: config.SiteTypeCMS, API: "http://x/api.php"},
			},
			Lives: []config.Live{{Name: "直播1"}},
		},
	}
	r := BuildReport(cfgs)

	if r.TotalSites != 4 {
		t.Errorf("TotalSites = %d, want 4", r.TotalSites)
	}
	if r.Usable != 2 {
		t.Errorf("Usable = %d, want 2 (js + cms)", r.Usable)
	}
	if r.Unsupported != 2 {
		t.Errorf("Unsupported = %d, want 2 (两个 jar)", r.Unsupported)
	}
	if r.LiveGroups != 1 {
		t.Errorf("LiveGroups = %d, want 1", r.LiveGroups)
	}
	if r.ByKind["jar"] != 2 {
		t.Errorf("ByKind[jar] = %d, want 2", r.ByKind["jar"])
	}
}

func TestReportTextMentionsKeyNumbers(t *testing.T) {
	r := Report{TotalSites: 309, Usable: 25, LiveGroups: 32, ByKind: map[string]int{"jar": 261}}
	out := r.Text()
	for _, want := range []string{"309", "25", "32", "jar"} {
		if !strings.Contains(out, want) {
			t.Errorf("文本报告应包含 %q:\n%s", want, out)
		}
	}
}

func TestReportJSONValid(t *testing.T) {
	r := Report{TotalSites: 10, Usable: 3, ByKind: map[string]int{"js": 3}}
	b, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("输出非合法 JSON: %v", err)
	}
}

// TVBox 配置 JSON 本身不携带直播频道，频道只存在于每个直播源各自的 m3u
// 里，需要额外发一次网络请求才能拿到。unbox-scan 不做这一步展开，因此
// Report 里不应该出现一个必然是 0 的 LiveChannels 字段——那是在假装做过
// 一件实际没做的事。这里锁定该字段确实不存在。
func TestReportHasNoLiveChannelsField(t *testing.T) {
	r := Report{TotalSites: 1, LiveGroups: 1}
	b, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("输出非合法 JSON: %v", err)
	}
	if _, ok := back["liveChannels"]; ok {
		t.Errorf("JSON 输出不应包含 liveChannels 字段（频道数从未真正统计过）: %s", b)
	}
}

// BuildReport 面对只有直播、没有点播站点的配置时，TotalSites 应该如实
// 停在 0，不能因为存在 Lives 就悄悄凑出非零的站点相关计数。
func TestBuildReportZeroSites(t *testing.T) {
	cfgs := []*config.Config{
		{Lives: []config.Live{{Name: "直播1"}, {Name: "直播2"}}},
	}
	r := BuildReport(cfgs)

	if r.TotalSites != 0 {
		t.Errorf("TotalSites = %d, want 0", r.TotalSites)
	}
	if r.Usable != 0 || r.Maybe != 0 || r.Unsupported != 0 {
		t.Errorf("站点相关计数应全为 0，实际 Usable=%d Maybe=%d Unsupported=%d", r.Usable, r.Maybe, r.Unsupported)
	}
	if r.LiveGroups != 2 {
		t.Errorf("LiveGroups = %d, want 2", r.LiveGroups)
	}
}

// 当 TotalSites 为 0 时，Text() 绝不能输出形如 "0  (0.0%)" 这种看起来像
// 是"体检跑过了、结果恰好是 0"的文本；必须有一句明确说明没有发现任何
// 点播站点，避免调用方把"没扫到"和"扫到了、结果是 0"搞混。
func TestReportTextZeroTotalSitesDoesNotClaimSuccess(t *testing.T) {
	r := Report{ConfigCount: 1, LiveGroups: 2}
	out := r.Text()
	if !strings.Contains(out, "未发现") {
		t.Errorf("TotalSites=0 时应明确说明未发现任何点播站点:\n%s", out)
	}
	if strings.Contains(out, "0.0%") {
		t.Errorf("不应输出看似成功的百分比统计:\n%s", out)
	}
}

// 更一般地：整份报告全为零值时（既没有配置也没有站点也没有直播），
// Text() 不能读起来像是体检成功但结果恰好全是 0。
func TestReportTextAllZeroDoesNotClaimSuccess(t *testing.T) {
	r := Report{ByKind: map[string]int{}}
	out := r.Text()
	if !strings.Contains(out, "未发现") {
		t.Errorf("全零报告应明确说明未发现任何可用内容:\n%s", out)
	}
}
