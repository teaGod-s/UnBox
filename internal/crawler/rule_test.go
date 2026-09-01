package crawler

import (
	"testing"
)

const testHTML = `<html><body><ul class="list"><li><a href="/a" data-x="1">甲</a></li><li><a href="/b" data-x="2">乙</a></li></ul></body></html>`

func callString(t *testing.T, e *Engine, script string) string {
	t.Helper()
	if err := e.Load("function go(){ return " + script + " }"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	return v.String()
}

func TestPdfhAttr(t *testing.T) {
	e := New()
	got := callString(t, e, `pdfh(`+quoteJS(testHTML)+`, 'ul&&a&&href')`)
	if got != "/a" {
		t.Fatalf("pdfh = %q, want %q", got, "/a")
	}
}

func TestPdfaText(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ return pdfa(` + quoteJS(testHTML) + `, 'li&&Text()') }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := e.vm.ExportTo(v, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "甲" || got[1] != "乙" {
		t.Fatalf("pdfa = %#v", got)
	}
}

func TestPdfSpecialMethods(t *testing.T) {
	e := New()
	if got := callString(t, e, `pdfh(`+quoteJS(testHTML)+`, 'a&&attr("data-x")')`); got != "1" {
		t.Fatalf("attr = %q", got)
	}
	if got := callString(t, e, `pd(`+quoteJS(testHTML)+`, 'li&&Text()', '|')`); got != "甲|乙" {
		t.Fatalf("pd = %q", got)
	}
}

func TestRuleParsesDrpyFields(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={host:"https://example.com",url:"/list/fyclass-fypage.html",searchUrl:"/s?wd=**",class_parse:".nav&&li;a&&Text;a&&href;/(\\w+).html",lazy:"js:input=input",headers:{"User-Agent":"UA"}}`); err != nil {
		t.Fatal(err)
	}
	r, err := e.Rule()
	if err != nil || r.URL != "/list/fyclass-fypage.html" || r.ClassParse == "" || r.Lazy == "" {
		t.Fatalf("rule=%+v err=%v", r, err)
	}
}

func quoteJS(s string) string {
	return `"` + jsEscape(s) + `"`
}

func jsEscape(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			out = append(out, '\\', '\\')
		case '"':
			out = append(out, '\\', '"')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
