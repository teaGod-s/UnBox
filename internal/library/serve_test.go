package library

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerServesRegisteredFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(p, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newServer()
	defer s.close()
	u := s.register(p)
	if !strings.HasPrefix(u, "http://127.0.0.1:") {
		t.Fatalf("register url=%q", u)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "fake-video-bytes" {
		t.Fatalf("body=%q", b)
	}
}

func TestServerRejectsBadToken(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := s.register(p)
	bad := strings.Replace(u, "?t=", "?t=wrong", 1)
	resp, err := http.Get(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token should be 403, got %d", resp.StatusCode)
	}
}

func TestServerUsesOpaqueIDAndSupportsRange(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "private", "a.mp4")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := s.register(p)
	if strings.Contains(u, p) || strings.Contains(u, filepath.Dir(p)) {
		t.Fatalf("file path leaked in URL %q", u)
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(b) != "2345" {
		t.Fatalf("range response status=%d body=%q", resp.StatusCode, b)
	}
}

func TestServerUnknownIDAndClose(t *testing.T) {
	s := newServer()
	defer s.close()
	p := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := s.register(p)
	unknown := strings.Replace(u, "/v/1?", "/v/999?", 1)
	resp, err := http.Get(unknown)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id should be 404, got %d", resp.StatusCode)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
}
