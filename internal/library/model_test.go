package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplayName(t *testing.T) {
	cases := map[string]string{
		"/mnt/电影/流浪地球.2019.BluRay.1080p.mp4": "流浪地球",
		"/mnt/剧/庆余年.S01E01.mkv":              "庆余年",
		"/mnt/剧/漫长的季节 第03集.mp4":              "漫长的季节",
	}
	for path, want := range cases {
		if got := displayName(path); got != want {
			t.Fatalf("displayName(%q)=%q, want %q", path, got, want)
		}
	}
}

func TestFindPoster(t *testing.T) {
	dir := t.TempDir()
	if got := findPoster(dir, "movie"); got != "" {
		t.Fatalf("空目录应无海报, got %q", got)
	}
	// 同名 poster.jpg
	poster := filepath.Join(dir, "movie-poster.jpg")
	if err := os.WriteFile(poster, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findPoster(dir, "movie"); got != poster {
		t.Fatalf("findPoster=%q, want %q", got, poster)
	}
}
