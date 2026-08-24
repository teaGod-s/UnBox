package tvbox

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSplitSources(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"feifan$$$ffm3u8", []string{"feifan", "ffm3u8"}},
		{"feifan,ffm3u8", []string{"feifan", "ffm3u8"}},
		{"single", []string{"single"}},
		{"", nil},
		{"a$$$b$$$c", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitSources(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitSources(%q) = %v, 期望 %v", c.in, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("splitSources(%q)[%d] = %q, 期望 %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// loadDetailFixture 从真实 fixture 读详情，返回 vod_play_from / vod_play_url。
func loadDetailFixture(t *testing.T) (from, playURL string) {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/cms/detail.json")
	if err != nil {
		t.Fatalf("读 fixture 失败: %v", err)
	}
	var resp struct {
		List []struct {
			VodPlayFrom string `json:"vod_play_from"`
			VodPlayURL  string `json:"vod_play_url"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("解析 fixture 失败: %v", err)
	}
	return resp.List[0].VodPlayFrom, resp.List[0].VodPlayURL
}

func TestParseEpisodesRealFixture(t *testing.T) {
	from, playURL := loadDetailFixture(t)
	sources := splitSources(from) // 期望 ["feifan","ffm3u8"]
	if len(sources) != 2 {
		t.Fatalf("fixture 线路数 = %d, 期望 2", len(sources))
	}
	eps := parseEpisodes("98823", playURL, sources)
	if len(eps) != 14 {
		t.Fatalf("剧集数 = %d, 期望 14（2 线路 × 7 集）", len(eps))
	}
	if eps[0].ID != "98823/0/0" || eps[0].Source != "feifan" || eps[0].Name != "第01集" || eps[0].URL == "" {
		t.Fatalf("首集解析错误: %+v", eps[0])
	}
	if eps[7].ID != "98823/1/0" || eps[7].Source != "ffm3u8" {
		t.Fatalf("第二线路首集解析错误: %+v", eps[7])
	}
	if eps[13].ID != "98823/1/6" || eps[13].Source != "ffm3u8" {
		t.Fatalf("末集解析错误: %+v", eps[13])
	}
}

func TestParseEpisodesEdgeCases(t *testing.T) {
	// 两集一路线
	eps := parseEpisodes("1", "第01集$a#第02集$b", []string{"x"})
	if len(eps) != 2 || eps[0].Name != "第01集" || eps[1].URL != "b" {
		t.Fatalf("两集解析错误: %+v", eps)
	}
	// 空 playURL
	if n := len(parseEpisodes("1", "", nil)); n != 0 {
		t.Fatalf("空 playURL 应得 0 集, 实得 %d", n)
	}
	// 无名字的裸地址
	eps = parseEpisodes("1", "https://x/a.m3u8", nil)
	if len(eps) != 1 || eps[0].Name != "" || eps[0].URL != "https://x/a.m3u8" {
		t.Fatalf("裸地址解析错误: %+v", eps)
	}
}
