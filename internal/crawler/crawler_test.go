package crawler

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	if New() == nil {
		t.Fatal("New() 返回 nil")
	}
}

func TestLoadAndCall(t *testing.T) {
	e := New()
	if err := e.Load(`function home(){ return "ok" }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "ok" {
		t.Fatalf("home() = %q, want %q", got, "ok")
	}
}

func TestCallUndefinedFunction(t *testing.T) {
	_, err := New().Call("missing")
	if err == nil {
		t.Fatal("Call 应返回未定义函数错误")
	}
}

func TestNormalizeModuleSourceStripsAsyncAwaitAcrossWhitespace(t *testing.T) {
	src := "async function home(){\n  return await\n    req('https://example.com')\n}\n"
	got := normalizeModuleSource(src)
	for _, forbidden := range []string{"async", "await"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("normalize 后仍含 %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "function home()") {
		t.Fatalf("async function 未归约为 function:\n%s", got)
	}
}

func TestNormalizeModuleSourceRewritesExportDefault(t *testing.T) {
	src := "function home(){ return 1 }\nexport default { home }"
	got := normalizeModuleSource(src)
	if strings.Contains(got, "export default") {
		t.Fatalf("export default 残留:\n%s", got)
	}
	if !strings.Contains(got, "var __crawler_exports =") {
		t.Fatalf("export default 未改写为 __crawler_exports:\n%s", got)
	}
}

func TestNormalizeModuleSourcePreservesIdentifiers(t *testing.T) {
	src := "const awaitable = 1; const asyncTask = 2;"
	got := normalizeModuleSource(src)
	for _, want := range []string{"awaitable", "asyncTask"} {
		if !strings.Contains(got, want) {
			t.Fatalf("标识符 %q 被误伤:\n%s", want, got)
		}
	}
}
