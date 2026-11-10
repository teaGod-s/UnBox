package crawler

import "testing"

func TestResolveLazy(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input={url:input.split("?")[0]}'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a.m3u8" {
		t.Fatalf("resolveLazy=%q", got)
	}
}

func TestResolveLazyStringResult(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:'js:input=input.split("?")[0]'}`); err != nil {
		t.Fatal(err)
	}
	got, err := e.resolveLazy("线路", "a.m3u8?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a.m3u8" {
		t.Fatalf("resolveLazy=%q", got)
	}
}

// 规则脚本常用 `let html=...` / `const x=...` 开头。同一个引擎第二次执行同一条
// 脚本时，直接跑在共享 VM 的全局作用域里会命中
// "Identifier 'html' has already been declared"（某影视源详情第二次就挂）。
func TestResolveLazyRepeatedRunsDoNotCollide(t *testing.T) {
	e := New()
	if err := e.Load(`var rule={lazy:"js:let base=input.url.split('?')[0]; input=base"}`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := e.resolveLazy("", "http://x/a.m3u8?tok=1")
		if err != nil {
			t.Fatalf("第 %d 次 lazy 执行失败: %v", i+1, err)
		}
		if got != "http://x/a.m3u8" {
			t.Fatalf("第 %d 次 got %q", i+1, got)
		}
	}
}

func TestExtractJSDetailRepeatedRunsDoNotCollide(t *testing.T) {
	e := New()
	script := `let html = {title:"甲"}; const tag = "x"; VOD.vod_name = html.title + tag`
	rule := &Rule{Host: "https://example.com"}
	for i := 0; i < 3; i++ {
		detail, err := e.extractJSDetail("", script, rule, "1")
		if err != nil {
			t.Fatalf("第 %d 次详情 JS 执行失败: %v", i+1, err)
		}
		if detail.VodName != "甲x" {
			t.Fatalf("第 %d 次 VodName=%q", i+1, detail.VodName)
		}
	}
}
