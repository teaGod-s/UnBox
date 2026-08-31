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
