package playback

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

func TestProxyInjectsHeadersAndRewritesAllHLSReferences(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://origin.example/" ||
			r.Header.Get("User-Agent") != "Unbox-Test" || r.Header.Get("Cookie") != "sid=abc" {
			http.Error(w, "missing headers", http.StatusForbidden)
			return
		}
		mu.Lock()
		requested[r.URL.RequestURI()]++
		mu.Unlock()
		switch r.URL.Path {
		case "/live/index.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\n"+
				"#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n"+
				"#EXT-X-MAP:URI=\"init.mp4\"\n"+
				"#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio/track.m3u8\"\n"+
				"#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=86000,URI=\"iframe.m3u8\"\n"+
				"segment.ts?sig=1\n")
		case "/live/audio/track.m3u8", "/live/iframe.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\nchild.ts\n")
		default:
			_, _ = io.WriteString(w, "asset:"+r.URL.RequestURI())
		}
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.Client(), time.Minute)
	t.Cleanup(func() { _ = proxy.Close() })
	proxyURL, err := proxy.Register(context.Background(), player.Stream{
		URL:  upstream.URL + "/live/index.m3u8",
		Kind: player.StreamHLS,
		Headers: map[string]string{
			"Referer":    "https://origin.example/",
			"User-Agent": "Unbox-Test",
			"Cookie":     "sid=abc",
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	body := getBody(t, proxyURL)
	if strings.Contains(body, `URI="key.bin"`) || strings.Contains(body, "\nsegment.ts?sig=1\n") {
		t.Fatalf("清单仍含未代理地址:\n%s", body)
	}
	refs := manifestProxyURLs(body)
	if len(refs) != 5 {
		t.Fatalf("重写地址数 = %d, want 5; body:\n%s", len(refs), body)
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, proxy.baseURL()+"/proxy/") {
			t.Fatalf("重写地址未指向本地代理: %s", ref)
		}
		_ = getBody(t, ref)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/live/key.bin", "/live/init.mp4", "/live/audio/track.m3u8", "/live/iframe.m3u8", "/live/segment.ts?sig=1"} {
		if requested[path] != 1 {
			t.Errorf("upstream %s requests = %d, want 1", path, requested[path])
		}
	}
}

func TestProxyForwardsRangeStatusAndCORS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=2-4" {
			t.Errorf("Range = %q", got)
		}
		w.Header().Set("Content-Range", "bytes 2-4/6")
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "cde")
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.Client(), time.Minute)
	t.Cleanup(func() { _ = proxy.Close() })
	proxyURL, err := proxy.Register(context.Background(), player.Stream{URL: upstream.URL + "/movie.mp4", Kind: player.StreamMP4})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, proxyURL, nil)
	req.Header.Set("Range", "bytes=2-4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != "cde" {
		t.Fatalf("response = %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Range") != "bytes 2-4/6" || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("headers = %v", resp.Header)
	}
}

func TestProxyPreservesStreamHeadersAcrossRedirectHosts(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://origin.example/" || r.Header.Get("Cookie") != "sid=abc" {
			http.Error(w, "redirect lost headers", http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/final.ts", http.StatusFound)
	}))
	defer source.Close()

	proxy := NewProxy(source.Client(), time.Minute)
	t.Cleanup(func() { _ = proxy.Close() })
	proxyURL, err := proxy.Register(context.Background(), player.Stream{
		URL:     source.URL + "/start.ts",
		Headers: map[string]string{"Referer": "https://origin.example/", "Cookie": "sid=abc"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := getBody(t, proxyURL); got != "ok" {
		t.Fatalf("body = %q", got)
	}
}

func TestProxyRejectsTamperedAndExpiredURLs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secret")
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.Client(), time.Minute)
	t.Cleanup(func() { _ = proxy.Close() })
	now := time.Unix(100, 0)
	proxy.now = func() time.Time { return now }
	proxyURL, err := proxy.Register(context.Background(), player.Stream{URL: upstream.URL + "/a.ts"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	tampered, _ := url.Parse(proxyURL)
	query := tampered.Query()
	query.Set("url", "aHR0cDovLzEyNy4wLjAuMTo5L3NlY3JldA")
	tampered.RawQuery = query.Encode()
	if status := getStatus(t, tampered.String()); status != http.StatusForbidden {
		t.Fatalf("tampered status = %d, want 403", status)
	}
	now = now.Add(2 * time.Minute)
	if status := getStatus(t, proxyURL); status != http.StatusNotFound {
		t.Fatalf("expired status = %d, want 404", status)
	}
}

func TestProxyCloseIsIdempotent(t *testing.T) {
	proxy := NewProxy(http.DefaultClient, time.Minute)
	if _, err := proxy.Register(context.Background(), player.Stream{URL: "https://media.example/a.ts"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := proxy.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := proxy.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func getBody(t *testing.T, rawURL string) string {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", rawURL, resp.StatusCode, body)
	}
	return string(body)
}

func getStatus(t *testing.T, rawURL string) int {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

var proxyURLPattern = regexp.MustCompile(`http://127\.0\.0\.1:\d+/proxy/[A-Za-z0-9_-]+\?[^\s"']+`)

func manifestProxyURLs(body string) []string {
	return proxyURLPattern.FindAllString(body, -1)
}
