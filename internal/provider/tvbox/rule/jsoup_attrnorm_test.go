package rule

import (
	"testing"
)

// goquery 属性选择器 `a[href^=#playlist]`（值未加引号）解析失败返回 0；
// 加引号 `a[href^="#playlist"]` 才匹配。规则文件常写不带引号的形态，
// 引擎须在交给 goquery 前规范化属性值。
func TestJsoupNormalizeAttrSelector(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`a[href^=#playlist]`, `a[href^="#playlist"]`},
		{`a[href$=.html]`, `a[href$=".html"]`},
		{`[class=poster]`, `[class="poster"]`},
		{`a[href*='playlist']`, `a[href*='playlist']`}, // 已带引号不变
		{`a[href="#x"]`, `a[href="#x"]`},               // 已带引号不变
		{`div.item`, `div.item`},                       // 无属性选择器不变
	}
	for _, c := range cases {
		got := normalizeAttrSelector(c.in)
		if got != c.want {
			t.Errorf("normalizeAttrSelector(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 端到端：unquote 的 [href^=#playlist] 规则能取到线路名。
func TestJsoupSelectHashPseudoAttr(t *testing.T) {
	const html = `<html><body>
<a href="#playlist5" data-toggle="tab">云播放①</a>
<a href="#playlist1" data-toggle="tab">云线路④</a>
</body></html>`
	doc := jsoupDoc(t, html)
	// 模拟规则 "body&&a[href^=#playlist]"
	nodes := jsoupSelectNodes(doc, "body&&a[href^=#playlist]")
	if nodes == nil || nodes.Length() != 2 {
		t.Fatalf("选择器命中 %d，期望 2", nodes.Length())
	}
}
