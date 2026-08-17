package config

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		site     Site
		wantKind string
		wantSup  Support
	}{
		{Site{Type: SiteTypeSpider, API: "csp_Ftyg"}, "jar", SupportNo},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.js"}, "js", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.py"}, "python", SupportNo},
		{Site{Type: SiteTypeSpider, API: "https://x.com/api"}, "http", SupportMaybe},
		{Site{Type: SiteTypeCMS, API: "https://x.com/api.php"}, "cms", SupportYes},
		{Site{Type: SiteTypeXPath, API: "https://x.com"}, "xpath", SupportNo},
		{Site{Type: SiteTypeRemote, API: "http://127.0.0.1:9978"}, "remote", SupportMaybe},
		// Regression tests for .json substring matching bug
		{Site{Type: SiteTypeSpider, API: "https://example.com/data.json"}, "http", SupportMaybe},
		{Site{Type: SiteTypeSpider, API: "https://example.com/api.json?token=1"}, "http", SupportMaybe},
		{Site{Type: SiteTypeSpider, API: "https://example.com/spider.js?token=abc"}, "js", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://example.com/spider.py?v=2"}, "python", SupportNo},
	}
	for _, c := range cases {
		kind, sup := Classify(c.site)
		if kind != c.wantKind || sup != c.wantSup {
			t.Errorf("Classify(%v) = (%q,%v), want (%q,%v)",
				c.site.API, kind, sup, c.wantKind, c.wantSup)
		}
	}
}
