package crawler

import (
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
