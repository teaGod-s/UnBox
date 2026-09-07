package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/store"
)

func openLibraryTest(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "unbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLibraryScanAndList(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.AddDir(root); err != nil {
		t.Fatal(err)
	}
	res, err := lib.Scan()
	if err != nil || res.Added != 1 || res.Dirs != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	items, err := lib.List()
	if err != nil || len(items) != 1 || items[0].Name != "a" || items[0].Dir != root {
		t.Fatalf("items=%+v err=%v", items, err)
	}

	if err := os.Remove(filepath.Join(root, "a.mp4")); err != nil {
		t.Fatal(err)
	}
	res, err = lib.Scan()
	if err != nil || res.Removed != 1 {
		t.Fatalf("res after removal=%+v err=%v", res, err)
	}
	items, err = lib.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("stale items=%+v err=%v", items, err)
	}
}

func TestLibraryStreamForRouting(t *testing.T) {
	lib := New(nil)
	mp4, err := lib.StreamFor("/x/a.mp4")
	if err != nil || mp4.Kind != player.StreamMP4 {
		t.Fatalf("mp4=%+v err=%v", mp4, err)
	}
	if !strings.HasPrefix(mp4.URL, "http://127.0.0.1:") {
		t.Fatalf("mp4 URL 应为本地 http, got %q", mp4.URL)
	}
	mkv, err := lib.StreamFor("/x/b.mkv")
	if err != nil || mkv.Kind != player.StreamLocal || mkv.URL != "file:///x/b.mkv" {
		t.Fatalf("mkv=%+v err=%v", mkv, err)
	}
	if _, err := lib.StreamFor("/x/readme.txt"); err == nil {
		t.Fatal("非视频文件应被拒绝")
	}
}

func TestLibraryRecordProgress(t *testing.T) {
	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.RecordProgress("/x/a.mp4", 42, 100); err != nil {
		t.Fatal(err)
	}
	h, err := st.ListVodHistory(10)
	if err != nil || len(h) != 1 || h[0].Site != "local" || h[0].VodID != "/x/a.mp4" || h[0].Progress != 42 {
		t.Fatalf("h=%+v err=%v", h, err)
	}
}

func TestLibraryRecordProgressKeepsDurationWhenUnknown(t *testing.T) {
	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.RecordProgress("/x/a.mp4", 42, 100); err != nil {
		t.Fatal(err)
	}
	if err := lib.RecordProgress("/x/a.mp4", 84, 0); err != nil {
		t.Fatal(err)
	}
	h, err := st.ListVodHistory(10)
	if err != nil || len(h) != 1 || h[0].Progress != 84 || h[0].Duration != 100 {
		t.Fatalf("h=%+v err=%v", h, err)
	}
}

func TestLibraryRecordProgressKeepsPoster(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "movie.mp4")
	poster := filepath.Join(root, "movie-poster.jpg")
	if err := os.WriteFile(media, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poster, []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.AddDir(root); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Scan(); err != nil {
		t.Fatal(err)
	}
	if err := lib.RecordProgress(media, 42, 100); err != nil {
		t.Fatal(err)
	}
	h, err := st.ListVodHistory(10)
	if err != nil || len(h) != 1 || h[0].VodLogo == "" {
		t.Fatalf("h=%+v err=%v", h, err)
	}
}

func TestLibraryStreamForRejectsPathOutsideRegisteredDirs(t *testing.T) {
	root := t.TempDir()
	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.AddDir(root); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.StreamFor(filepath.Join(t.TempDir(), "outside.mp4")); err == nil {
		t.Fatal("媒体库目录之外的路径应被拒绝")
	}
}

func TestLibraryStreamForRejectsSymlinkEscapingRegisteredDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "outside.mp4")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.mp4")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("当前文件系统不支持符号链接: %v", err)
	}
	st := openLibraryTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.AddDir(root); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.StreamFor(link); err == nil {
		t.Fatal("逃逸到注册目录之外的符号链接应被拒绝")
	}
}
