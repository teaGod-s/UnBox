package config

import "testing"

func TestParseSpider(t *testing.T) {
	cases := []struct {
		raw      string
		wantPath string
		wantMD5  string
	}{
		{"./jar/XYQ.jar;md5;705BA6E23C4384C37A11DD904727520B", "./jar/XYQ.jar", "705ba6e23c4384c37a11dd904727520b"},
		{"https://x/y.jar;md5;abc", "https://x/y.jar", "abc"},
		{"https://x/y.jar", "https://x/y.jar", ""},
		{"  ./a.jar  ", "./a.jar", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		got := ParseSpider(c.raw)
		if got.Path != c.wantPath || got.MD5 != c.wantMD5 {
			t.Errorf("ParseSpider(%q) = (%q,%q)，期望 (%q,%q)", c.raw, got.Path, got.MD5, c.wantPath, c.wantMD5)
		}
	}
}

func TestSpiderDescriptorStringRoundTrip(t *testing.T) {
	d := SpiderDescriptor{Path: "https://x/y.jar", MD5: "abc"}
	if got := d.String(); got != "https://x/y.jar;md5;abc" {
		t.Fatalf("String() = %q", got)
	}
	if got := (SpiderDescriptor{Path: "https://x/y.jar"}).String(); got != "https://x/y.jar" {
		t.Fatalf("无 md5 时 String() = %q", got)
	}
}
