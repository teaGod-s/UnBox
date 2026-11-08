package config

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		site     Site
		wantKind string
		wantSup  Support
	}{
		{Site{Type: SiteTypeSpider, API: "csp_Ftyg"}, "jar", SupportMaybe},
		{Site{Type: SiteTypeSpider, API: "csp_XBPQ"}, "jar", SupportYes},
		{Site{Type: SiteTypeSpider, API: "csp_xbpq?v=1"}, "jar", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.js"}, "js", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://x.com/d.py"}, "python", SupportNo},
		{Site{Type: SiteTypeSpider, API: "https://x.com/api"}, "http", SupportYes},
		{Site{Type: SiteTypeCMS, API: "https://x.com/api.php"}, "cms", SupportYes},
		{Site{Type: SiteTypeXPath, API: "https://x.com"}, "xpath", SupportNo},
		{Site{Type: SiteTypeRemote, API: "http://127.0.0.1:9978"}, "remote", SupportMaybe},
		// 未定义的 type 值（如 2）应落入 default 分支；零值 Site 的 type 为 0，即 XPath
		{Site{Type: 2}, "unknown", SupportNo},
		{Site{}, "xpath", SupportNo},
		// Regression tests for .json substring matching bug
		{Site{Type: SiteTypeSpider, API: "https://example.com/data.json"}, "http", SupportYes},
		{Site{Type: SiteTypeSpider, API: "https://example.com/api.json?token=1"}, "http", SupportYes},
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

// knownJarEngines 必须与 rule.Supported() 的实现类集合保持一致：
// rule.Supported() 新增引擎类（如 M6.2 的 csp_xyqhiker）时这里要同步登记，
// 否则支持程度会误标成「待定」。
func TestKnownJarEnginesMatchRuleSupported(t *testing.T) {
	// case 里的 api 会被 ToLower，测试用全小写。
	implemented := []string{
		"csp_xbpq",
		"csp_xyqhiker",
		"csp_xyqhikeral",
	}
	for _, class := range implemented {
		if !knownJarEngines[class] {
			t.Errorf("knownJarEngines 缺已实现类 %q（rule.Supported 已支持）", class)
		}
		site := Site{Type: SiteTypeSpider, API: class}
		_, sup := Classify(site)
		if sup != SupportYes {
			t.Errorf("Classify(%q) support=%v, want 可用", class, sup)
		}
	}
}
