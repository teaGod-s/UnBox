package rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetSetsUAAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "okhttp/3.12.11" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("X-Test") != "1" {
			t.Errorf("自定义头丢失")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewClient(map[string]string{"X-Test": "1"})
	got, err := c.Get(context.Background(), srv.URL)
	if err != nil || got != "ok" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}

func TestClientRejectsOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxBodySize+1))
	}))
	defer srv.Close()
	c := NewClient(nil)
	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("超限响应应报错")
	}
}
