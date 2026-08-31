package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dop251/goja"
)

func TestReqInjectsAndCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wd") != "test" {
			t.Errorf("query wd=%q", r.URL.Query().Get("wd"))
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Errorf("X-Test=%q", got)
		}
		_, _ = w.Write([]byte("resp-body"))
	}))
	defer srv.Close()

	e := New()
	e.installReq(http.DefaultClient)
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q, {headers:{"X-Test":"yes"}}) }`, srv.URL+"?wd=test")); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(e.vm)
	if got := obj.Get("content").String(); got != "resp-body" {
		t.Fatalf("content = %q", got)
	}
	if got := obj.Get("statusCode").ToInteger(); got != 200 {
		t.Fatalf("statusCode = %d", got)
	}
	if got := obj.Get("finalUrl").String(); got == "" {
		t.Fatal("finalUrl 为空")
	}
	if _, ok := obj.Get("headers").(*goja.Object); !ok {
		t.Fatal("headers 应为对象")
	}
}

func TestReqPostData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	e := New()
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q, {method:"POST",data:"a=1"}).content }`, srv.URL)); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "ok" {
		t.Fatalf("req POST = %v, %v", v, err)
	}
}

func TestDecodeBody(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        string
	}{
		{
			name:        "GBK",
			body:        []byte{0xd6, 0xd0, 0xce, 0xc4}, // 中文
			contentType: "text/html; charset=GBK",
			want:        "中文",
		},
		{
			name:        "UTF-8 fallback",
			body:        []byte("中文"),
			contentType: "application/json; charset=utf-8",
			want:        "中文",
		},
		{
			name:        "GBK without charset",
			body:        []byte{0xd6, 0xd0, 0xce, 0xc4},
			contentType: "",
			want:        "中文",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeBody(tt.body, tt.contentType); got != tt.want {
				t.Fatalf("decodeBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReqDecodesGBKResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=gb2312")
		_, _ = w.Write([]byte{0xd6, 0xd0, 0xce, 0xc4})
	}))
	defer srv.Close()

	e := New()
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q).content }`, srv.URL)); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "中文" {
		t.Fatalf("req GBK content = %v, %v", v, err)
	}
}

func TestLoadFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`function loaded(){ return "ok" }`))
	}))
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("loaded")
	if err != nil || v.String() != "ok" {
		t.Fatalf("loaded() = %v, %v", v, err)
	}
}
