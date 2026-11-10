package crawler

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// drpy2 脚本普遍引用 jsp 全局（jsp.pdfh/pdfa/pd），如某影视源二级:
//
//	pdfh=jsp.pdfh; pdfa=jsp.pdfa; pd=jsp.pd;
//	actual=pdfh(html,".vt-txt&&Text")        → "双人大逃杀"
//	pic=pd(html,".video-img&&img&&src")      → "..."
//
// 引擎此前完全不注入 jsp，脚本执行到 jsp.pdfh 就抛 ReferenceError。
func TestJSPShim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := New()
	e.SetContext(ctx)
	if _, err := e.vm.RunScript("jsp_test.js", `
var html = '<div class="vt-txt">双人大逃杀</div>' +
  '<p>2024</p><p>中国大陆</p>' +
  '<div class="video-img"><img src="http://img/a.jpg"></div>' +
  '<div class="desc"><span>简介：这是一部好片</span></div>';
var out = {
  title: jsp.pdfh(html, ".vt-txt&&Text"),
  year: jsp.pdfh(html, "p:eq(0)&&Text"),
  region: jsp.pdfh(html, "p:eq(1)&&Text"),
  img: jsp.pd(html, ".video-img&&img&&src"),
  all: jsp.pdfa(html, "p&&Text"),
  desc: jsp.pdfh(html, ".desc&&Text")
};
`); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	result := e.vm.Get("out").ToObject(e.vm)
	assertJSString(t, result, "title", "双人大逃杀")
	assertJSString(t, result, "year", "2024")
	assertJSString(t, result, "region", "中国大陆")
	assertJSString(t, result, "img", "http://img/a.jpg")
	assertJSString(t, result, "desc", "简介：这是一部好片")
	// pdfa 返回数组
	arr := result.Get("all")
	if arr == nil {
		t.Fatal("all 未定义")
	}
	if n := arr.ToObject(e.vm).Get("length").ToInteger(); n != 2 {
		t.Fatalf("all 长度 = %d, want 2", n)
	}
}

func assertJSString(t *testing.T, obj *goja.Object, key, want string) {
	t.Helper()
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		t.Fatalf("%s 未定义, want %q", key, want)
	}
	if got := v.String(); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
