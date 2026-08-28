package playback

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestResolverExtractsRelativeShareURLAndPreservesStreamMetadata(t *testing.T) {
	const referer = "https://site.example/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Referer"); got != referer {
			t.Errorf("Referer = %q, want %q", got, referer)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<script>const url="/media/index.m3u8?sign=a%2Bb";</script>`)
	}))
	defer srv.Close()

	in := player.Stream{
		URL:      srv.URL + "/share/1",
		Kind:     player.StreamHLS,
		Headers:  map[string]string{"Referer": referer},
		Backups:  []string{"https://backup.example/live.m3u8"},
		Subtitle: []player.SubtitleTrack{{URL: "https://sub.example/a.vtt", Lang: "zh"}},
	}
	got, err := NewResolver(srv.Client()).Resolve(context.Background(), in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != srv.URL+"/media/index.m3u8?sign=a%2Bb" || got.Kind != player.StreamHLS {
		t.Fatalf("Resolve = %+v", got)
	}
	if got.Headers["Referer"] != referer || len(got.Backups) != 1 || len(got.Subtitle) != 1 {
		t.Fatalf("Resolve 丢失流元数据: %+v", got)
	}
}

func TestResolverDecodesEscapedAbsoluteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<script>const url = "https:\/\/cdn.example\/movie.mp4?x=1\u0026y=2"</script>`)
	}))
	defer srv.Close()

	got, err := NewResolver(srv.Client()).Resolve(context.Background(), player.Stream{URL: srv.URL})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != "https://cdn.example/movie.mp4?x=1&y=2" || got.Kind != player.StreamMP4 {
		t.Fatalf("Resolve = %+v", got)
	}
}

func TestResolverPassesThroughNonHTMLWithoutReadingBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = io.WriteString(w, strings.Repeat("x", 2<<20))
	}))
	defer srv.Close()

	in := player.Stream{URL: srv.URL + "/live.m3u8", Kind: player.StreamHLS}
	got, err := NewResolver(srv.Client()).Resolve(context.Background(), in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != in.URL || got.Kind != player.StreamHLS {
		t.Fatalf("Resolve = %+v, want passthrough", got)
	}
}

func TestResolverRejectsHTMLWithoutPlayableURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html>missing stream</html>`)
	}))
	defer srv.Close()

	_, err := NewResolver(srv.Client()).Resolve(context.Background(), player.Stream{URL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "播放地址") {
		t.Fatalf("Resolve error = %v", err)
	}
}

func TestResolverRejectsOversizedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, strings.Repeat("x", (1<<20)+1))
	}))
	defer srv.Close()

	_, err := NewResolver(srv.Client()).Resolve(context.Background(), player.Stream{URL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("Resolve error = %v", err)
	}
}

func TestResolverReportsUpstreamStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	_, err := NewResolver(srv.Client()).Resolve(context.Background(), player.Stream{URL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "410") {
		t.Fatalf("Resolve error = %v", err)
	}
}
