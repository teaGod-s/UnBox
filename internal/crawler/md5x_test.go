package crawler

import "testing"

// FongMi 引擎脚本的全局 md5X（金牌影视等）与 md5 同语义（hex 小写）。
// goja 环境缺 md5X → 脚本报 ReferenceError: md5X is not defined。
func TestMd5XGlobal(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ return md5X("abc") }`); err != nil {
		t.Fatalf("加载: %v", err)
	}
	got, err := e.Call("go")
	if err != nil {
		t.Fatalf("md5X 应可用: %v", err)
	}
	want := "900150983cd24fb0d6963f7d28e17f72" // md5("abc") hex
	if got.String() != want {
		t.Fatalf("md5X(\"abc\") = %q, 期望 %q", got.String(), want)
	}
}

// md5X 与既有 md5 全局一致。
func TestMd5XMatchesMd5(t *testing.T) {
	e := New()
	if err := e.Load(`function go(){ return md5X("你好123") === md5("你好123") }`); err != nil {
		t.Fatalf("加载: %v", err)
	}
	got, err := e.Call("go")
	if err != nil || got.String() != "true" {
		t.Fatalf("md5X != md5: %v %v", got, err)
	}
}
