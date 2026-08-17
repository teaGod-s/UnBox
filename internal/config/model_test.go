package config

import "testing"

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
