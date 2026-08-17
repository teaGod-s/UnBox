package config

import (
	"encoding/json"
	"testing"
)

func TestSiteTypeString(t *testing.T) {
	cases := []struct {
		in   SiteType
		want string
	}{
		{SiteTypeXPath, "xpath"},
		{SiteTypeCMS, "cms"},
		{SiteTypeSpider, "spider"},
		{SiteTypeRemote, "remote"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("SiteType(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigCountsZeroValue(t *testing.T) {
	var c Config
	if len(c.Sites) != 0 || len(c.Lives) != 0 {
		t.Error("零值 Config 应当没有站点和直播源")
	}
}

func TestLiveListToleratesNonObjectEntries(t *testing.T) {
	raw := []byte(`{"sites":[{"key":"k","name":"n","type":1,"api":"https://x.com/api.php"}],"lives":[[]]}`)
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("lives:[[]] 不应导致整份配置解析失败: %v", err)
	}
	if len(cfg.Sites) != 1 {
		t.Fatalf("sites 应保留 1 个，实际 %d", len(cfg.Sites))
	}
	if len(cfg.Lives) != 0 {
		t.Fatalf("非对象 lives 项应被跳过，实际保留 %d 个", len(cfg.Lives))
	}
}
