package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirFiltersAndRecurses(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("sub/a.mp4")
	mustWrite("sub/b.mkv")
	mustWrite("sub/c.txt")
	mustWrite("sub/d.srt")
	mustWrite("deep/e.webm")

	items, err := scanDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%+v", items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Ext] = true
		if item.Dir == "" || item.Name == "" {
			t.Fatalf("item 字段不全: %+v", item)
		}
		if item.Dir != filepath.Dir(item.Path) {
			t.Fatalf("item.Dir=%q, want parent of %q", item.Dir, item.Path)
		}
	}
	for _, ext := range []string{"mp4", "mkv", "webm"} {
		if !seen[ext] {
			t.Fatalf("缺扩展名 %s: %+v", ext, items)
		}
	}
}

func TestScanDirReturnsRootWalkError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := scanDir(root); err == nil {
		t.Fatal("扫描不存在的根目录应返回错误")
	}
}
