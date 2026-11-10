package crawler

import "testing"

// drpy2 的 filter_url 模板用 jinja 「或默认值」语法：
//
//	year={{fl.year or "all"}}&sort={{fl.sort or "all"}}
//
// 键缺失（如规则只有 filter 没有 filter_def）时取 or 后的默认值 all，
// 不能把整段 {{...}} 字面量原样带进请求（某影视源线上根因：某影视源接口收到
// 未展开模板 URL 返回空 JSON → "unexpected end of JSON input"）。
func TestRenderFilterTemplateOrDefault(t *testing.T) {
	tmpl := `year={{fl.year or "all"}}&sort={{fl.sort or "all"}}&chargeInfo={{fl.chargeInfo or "all"}}`
	values := map[string]string{} // 键全部缺失
	got := renderFilterTemplate(tmpl, values)
	want := `year=all&sort=all&chargeInfo=all`
	if got != want {
		t.Fatalf("renderFilterTemplate() = %q, want %q", got, want)
	}
}

// 提供某键默认值时，有值用值、无值用 or 默认值。
func TestRenderFilterTemplateOrDefaultMixed(t *testing.T) {
	tmpl := `year={{fl.year or "all"}}&sort={{fl.sort or "all"}}`
	values := map[string]string{"year": "2024"}
	got := renderFilterTemplate(tmpl, values)
	want := `year=2024&sort=all`
	if got != want {
		t.Fatalf("renderFilterTemplate() = %q, want %q", got, want)
	}
}

// 经典纯键语法 {{fl.year}} 不能被破坏。
func TestRenderFilterTemplatePlainKey(t *testing.T) {
	got := renderFilterTemplate(`p={{fl.page}}&x=1`, map[string]string{"page": "2"})
	if got != `p=2&x=1` {
		t.Fatalf("plain key = %q", got)
	}
}
