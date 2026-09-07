package thumb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeMPV(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-mpv.sh")
	body := `#!/bin/sh
for i in "$@"; do case "$i" in --o=*) out="${i#--o=}";; esac; done
printf '\xff\xd8\xff\xd9' > "$out"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestGenerateWritesCache(t *testing.T) {
	g := &MpvGenerator{mpvPath: fakeMPV(t), timeout: 5 * time.Second}
	cache := filepath.Join(t.TempDir(), "k.jpg")
	if err := g.Generate("/v/a.mkv", cache); err != nil {
		t.Fatalf("Generate = %v", err)
	}
	b, err := os.ReadFile(cache)
	if err != nil || len(b) == 0 {
		t.Fatalf("cache 未写入或为空: %v", err)
	}
}

func TestGenerateNoMpv(t *testing.T) {
	g := &MpvGenerator{mpvPath: "", timeout: 5 * time.Second}
	if err := g.Generate("/v/a.mkv", "/tmp/x.jpg"); !errors.Is(err, ErrNoMpv) {
		t.Fatalf("want ErrNoMpv, got %v", err)
	}
}
