package rule

import "testing"

func TestCut(t *testing.T) {
	const page = `<a title="甲" href="/v/1"><img data-original="https://img/1.jpg"></a>`
	cases := []struct{ spec, want string }{
		{`title="&&"`, "甲"},
		{`href="&&"`, "/v/1"},
		{`data-original="&&"`, "https://img/1.jpg"},
		{"", ""},
		{`不存在的&&"`, ""},
	}
	for _, c := range cases {
		if got := Cut(page, c.spec); got != c.want {
			t.Errorf("Cut(%q) = %q，期望 %q", c.spec, got, c.want)
		}
	}
}

func TestCutList(t *testing.T) {
	page := `<li class="module-item"><a title="甲" href="/1"></a></li>` +
		`<li class="module-item"><a title="乙" href="/2"></a></li>`
	got := CutList(page, `class="module-item"&&</li>`)
	if len(got) != 2 || !contains(got[0], "甲") || !contains(got[1], "乙") {
		t.Fatalf("CutList = %#v", got)
	}
}

func TestClean(t *testing.T) {
	if got := Clean(`<span>甲 &amp; 乙</span>`); got != "甲 & 乙" {
		t.Fatalf("Clean = %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
