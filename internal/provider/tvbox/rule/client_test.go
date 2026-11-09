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

// 黑狐影院等站点针对 TVBox 默认的 okhttp UA 定向 403，规则却没写请求头。
// Client 收到 403 且当前 UA 正是默认 okhttp 时，应换浏览器 UA 重试一次；
// 规则显式指定的 UA（如 User-Agent$PC_UA）不受影响。
func TestClientRetries403WithBrowserUA(t *testing.T) {
	firstUA := ""
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") == tvboxUA {
			firstUA = tvboxUA
			w.WriteHeader(http.StatusForbidden)
			return
		}
		firstUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()
	c := NewClient(nil)
	got, err := c.Get(context.Background(), srv.URL)
	if err != nil || got != "data" {
		t.Fatalf("Get = %q, %v（UA 应为浏览器形态，实为 %q）", got, err, firstUA)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2（okhttp 403 → 浏览器 UA 重试）", hits)
	}
}

// 规则显式指定了 UA 时，403 不应被自动覆盖（尊重规则作者的意图）。
func TestClientDoesNotOverrideExplicitUA(t *testing.T) {
	ua := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(map[string]string{"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) 1"})
	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("显式 UA 下 403 应保持报错")
	}
	if ua != "Mozilla/5.0 (X11; Linux x86_64) 1" {
		t.Fatalf("显式 UA 被改动: %q", ua)
	}
}
