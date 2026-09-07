package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unbox/unbox/internal/library/thumb"
)

type fakeThumbGenerator struct {
	err error
}

func (f fakeThumbGenerator) Generate(_, cachePath string) error {
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(cachePath, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644)
}

func newThumbTestLibrary(t *testing.T, gen thumb.Generator) (*Library, string) {
	t.Helper()
	postersDir := filepath.Join(t.TempDir(), "posters")
	return New(nil, postersDir, gen), postersDir
}

func TestEnsureThumbCacheHit(t *testing.T) {
	l, _ := newThumbTestLibrary(t, fakeThumbGenerator{})
	p := filepath.Join(t.TempDir(), "a.mp4")
	videoURL, posterURL, cached, err := l.EnsureThumb(p, 123)
	if err != nil || cached || videoURL == "" || posterURL == "" {
		t.Fatalf("首次 EnsureThumb = %q, %q, %v, %v", videoURL, posterURL, cached, err)
	}
	if _, err := l.SaveThumb(p, 123, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if _, _, cached, err = l.EnsureThumb(p, 123); err != nil || !cached {
		t.Fatalf("二次 EnsureThumb cached=%v err=%v", cached, err)
	}
	if _, _, cached, err = l.EnsureThumb(p, 124); err != nil || cached {
		t.Fatalf("mtime 变化不应命中缓存: cached=%v err=%v", cached, err)
	}
}

func TestSaveThumbWritesAtomicCache(t *testing.T) {
	l, postersDir := newThumbTestLibrary(t, fakeThumbGenerator{})
	p := filepath.Join(t.TempDir(), "a.mp4")
	posterURL, err := l.SaveThumb(p, 123, []byte{0xff, 0xd8, 0xff, 0xd9})
	if err != nil || posterURL == "" {
		t.Fatalf("SaveThumb = %q, %v", posterURL, err)
	}
	entries, err := os.ReadDir(postersDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("缓存文件数=%d err=%v", len(entries), err)
	}
	data, err := os.ReadFile(filepath.Join(postersDir, entries[0].Name()))
	if err != nil || len(data) == 0 {
		t.Fatalf("缓存文件为空: %v", err)
	}
}

func TestGenerateThumbMpvDelegates(t *testing.T) {
	l, _ := newThumbTestLibrary(t, fakeThumbGenerator{})
	poster, err := l.GenerateThumbMpv(filepath.Join(t.TempDir(), "a.mkv"), 456, fakeThumbGenerator{})
	if err != nil || poster == "" {
		t.Fatalf("GenerateThumbMpv = %q, %v", poster, err)
	}
}

func TestGenerateThumbMpvNoMpv(t *testing.T) {
	l, _ := newThumbTestLibrary(t, nil)
	_, err := l.GenerateThumbMpv(filepath.Join(t.TempDir(), "a.mkv"), 1, fakeThumbGenerator{err: thumb.ErrNoMpv})
	if !errors.Is(err, thumb.ErrNoMpv) {
		t.Fatalf("want ErrNoMpv, got %v", err)
	}
}
