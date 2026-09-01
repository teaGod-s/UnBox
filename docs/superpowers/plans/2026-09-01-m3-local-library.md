# M3 本地媒体库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现本地媒体库：多目录扫描 → 独立「媒体库」tab 浏览 → 复用 Web/mpv 路由播放 → 本地观看进度 + 首页续播。

**Architecture:** 新增 `internal/library` 包（纯 Go，扫描 + 片名/海报 + 本地文件 HTTP 服务 + 门面），`internal/store` 加两张表；`ShellService` 注入 Library 并暴露 Wails 方法；前端 `App.vue` 加 tab。播放复用 `playback.Controller.Prepare`，进度复用 `vod_history`（`site="local"`）。

**Tech Stack:** Go 1.26 + modernc.org/sqlite（纯 Go）+ net/http 本地文件服务 + Vue3/TS。

**Spec:** `docs/superpowers/specs/2026-09-01-unbox-m3-local-library-design.md`

## Global Constraints

- 模块路径 `github.com/unbox/unbox`；Go 1.26.3，Wails v3 钉死 `3.0.0-beta.9`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/library`/`store`/`playback`/`player` 不得 import Wails。
- 公开错误信息 / 注释用中文。
- TDD：改代码先写失败测试。
- 提交前：`go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；Linux 额外 `CGO_ENABLED=1 go build ./...`；前端 `vue-tsc --noEmit` + `vitest run`。
- 播放路由（D1 已定案 B）：mp4/m4v/webm → 本地文件服务 URL + `StreamMP4` → Web；其余 → `file://` + `StreamLocal` → mpv。

---

### Task 1: store 新增媒体库表与类型

**Files:**
- Modify: `internal/store/store.go`（`migrate()` 加两张表；新增 `LibraryDir`/`LibraryItem` 类型与 5 个方法）
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces: `store.LibraryDir{Path string; AddedAt int64}`、`store.LibraryItem{Path,Name,Dir,Ext string; Size,MTime int64; Poster string}`；
  `AddLibraryDir(path) error`、`RemoveLibraryDir(path) error`、`ListLibraryDirs() ([]LibraryDir, error)`、
  `ReplaceLibraryItems(dir string, items []LibraryItem) error`、`ListLibraryItems() ([]LibraryItem, error)`。

- [ ] **Step 1: 写失败测试**

在 `store_test.go` 追加：

```go
func TestLibraryDirs(t *testing.T) {
    s := openTest(t)
    defer s.Close()

    if err := s.AddLibraryDir("/mnt/movies"); err != nil {
        t.Fatal(err)
    }
    dirs, err := s.ListLibraryDirs()
    if err != nil || len(dirs) != 1 || dirs[0].Path != "/mnt/movies" {
        t.Fatalf("dirs=%+v err=%v", dirs, err)
    }
    if err := s.RemoveLibraryDir("/mnt/movies"); err != nil {
        t.Fatal(err)
    }
    dirs, _ = s.ListLibraryDirs()
    if len(dirs) != 0 {
        t.Fatalf("删除后应空，got %+v", dirs)
    }
}

func TestLibraryItemsReplacePerDir(t *testing.T) {
    s := openTest(t)
    defer s.Close()

    a := []LibraryItem{{Path: "/a/1.mp4", Name: "1", Dir: "/a", Ext: "mp4", Size: 10}}
    b := []LibraryItem{{Path: "/b/2.mkv", Name: "2", Dir: "/b", Ext: "mkv", Size: 20}}
    if err := s.ReplaceLibraryItems("/a", a); err != nil {
        t.Fatal(err)
    }
    if err := s.ReplaceLibraryItems("/b", b); err != nil {
        t.Fatal(err)
    }
    // 重扫 /a：1.mp4 删除、1b.mkv 新增
    a2 := []LibraryItem{{Path: "/a/1b.mkv", Name: "1b", Dir: "/a", Ext: "mkv", Size: 11}}
    if err := s.ReplaceLibraryItems("/a", a2); err != nil {
        t.Fatal(err)
    }
    items, err := s.ListLibraryItems()
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 2 { // /a 只剩 1b.mkv，/b 的 2.mkv 保留
        t.Fatalf("items=%+v", items)
    }
    for _, it := range items {
        if it.Path == "/a/1.mp4" {
            t.Fatalf("已删文件仍在: %+v", items)
        }
    }
}
```

> `openTest(t)` 是既有 helper（本文件内已有，复用即可；若无则 `Open(t.TempDir()+"/unbox.db")`）。

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/store/ -run 'TestLibrary' -v`
Expected: FAIL — `s.AddLibraryDir` 未定义 / 表不存在。

- [ ] **Step 3: 最小实现**

`store.go` 顶部类型区追加：

```go
// LibraryDir 是一个媒体库目录。
type LibraryDir struct {
    Path    string
    AddedAt int64
}

// LibraryItem 是扫描出的一个视频文件。
type LibraryItem struct {
    Path   string
    Name   string
    Dir    string
    Ext    string
    Size   int64
    MTime  int64
    Poster string
}
```

`migrate()` 的 `stmts` 追加两条：

```go
`CREATE TABLE IF NOT EXISTS library_dirs (path TEXT PRIMARY KEY, added_at INTEGER NOT NULL)`,
`CREATE TABLE IF NOT EXISTS library_items (path TEXT PRIMARY KEY, name TEXT NOT NULL, dir TEXT NOT NULL, ext TEXT NOT NULL, size INTEGER NOT NULL DEFAULT 0, mtime INTEGER NOT NULL DEFAULT 0, poster TEXT)`,
```

`store.go` 底部追加方法：

```go
func (s *Store) AddLibraryDir(path string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO library_dirs(path, added_at) VALUES(?,?)`, path, time.Now().Unix())
	return err
}

func (s *Store) RemoveLibraryDir(path string) error {
	_, err := s.db.Exec(`DELETE FROM library_dirs WHERE path=?`, path)
	if err == nil {
		_, err = s.db.Exec(`DELETE FROM library_items WHERE dir=?`, path)
	}
	return err
}

func (s *Store) ListLibraryDirs() ([]LibraryDir, error) {
	rows, err := s.db.Query(`SELECT path, added_at FROM library_dirs ORDER BY added_at, path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LibraryDir
	for rows.Next() {
		var d LibraryDir
		if err := rows.Scan(&d.Path, &d.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ReplaceLibraryItems 以事务整体替换某目录的扫描结果：先删该目录旧条目再插入新条目。
func (s *Store) ReplaceLibraryItems(dir string, items []LibraryItem) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM library_items WHERE dir=?`, dir); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(
			`INSERT INTO library_items(path,name,dir,ext,size,mtime,poster) VALUES(?,?,?,?,?,?,?)`,
			it.Path, it.Name, it.Dir, it.Ext, it.Size, it.MTime, it.Poster); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListLibraryItems() ([]LibraryItem, error) {
	rows, err := s.db.Query(`SELECT path,name,dir,ext,size,mtime,poster FROM library_items ORDER BY dir, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LibraryItem
	for rows.Next() {
		var it LibraryItem
		var poster sql.NullString
		if err := rows.Scan(&it.Path, &it.Name, &it.Dir, &it.Ext, &it.Size, &it.MTime, &poster); err != nil {
			return nil, err
		}
		it.Poster = poster.String
		out = append(out, it)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/store/ -run 'TestLibrary' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): 媒体库目录/条目表与方法"
```

---

### Task 2: library 包 — 片名清洗与海报匹配

**Files:**
- Create: `internal/library/model.go`
- Test: `internal/library/model_test.go`

**Interfaces:**
- Produces: `func displayName(path string) string`（去扩展名 + 去季集后缀）；
  `func findPoster(dir, stem string) string`（返回海报绝对路径或空）。

- [ ] **Step 1: 写失败测试**

```go
package library

import "testing"

func TestDisplayName(t *testing.T) {
    cases := map[string]string{
        "/mnt/电影/流浪地球.2019.BluRay.1080p.mp4": "流浪地球",
        "/mnt/剧/庆余年.S01E01.mkv":            "庆余年",
        "/mnt/剧/漫长的季节 第03集.mp4":          "漫长的季节",
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
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/library/ -run 'TestDisplayName|TestFindPoster' -v`
Expected: FAIL — `displayName` 未定义。

- [ ] **Step 3: 最小实现**

```go
// Package library 实现本地媒体库：目录扫描、片名/海报识别、本地文件服务与播放路由。
package library

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// seasonEpRe 匹配常见季集后缀（S01E01、第03集、EP01、第1话 等）。
var seasonEpRe = regexp.MustCompile(`(?i)[. _-]*(s\d{1,2}e\d{1,2}|第\s*\d{1,3}\s*[集话]|ep?\s*\d{1,3}|第\s*\d{1,3}\s*[话集])\b.*$`)

// displayName 从文件路径提取展示片名：去扩展名，去掉季集/分辨率等尾缀。
func displayName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	// 去掉季集后缀（保留片名主体）
	if m := seasonEpRe.FindStringIndex(stem); m != nil {
		stem = strings.TrimSpace(stem[:m[0]])
	}
	// 去掉尾部 year/分辨率/来源等点段（如 .2019.BluRay.1080p）
	if i := strings.IndexAny(stem, ". "); i > 0 {
		stem = strings.TrimSpace(stem[:i])
	}
	if stem == "" {
		return base
	}
	return stem
}

// findPoster 在同目录找海报：<stem>-poster.jpg / poster.jpg / folder.jpg / 目录名.jpg。
func findPoster(dir, stem string) string {
	candidates := []string{
		filepath.Join(dir, stem+"-poster.jpg"),
		filepath.Join(dir, stem+"-poster.png"),
		filepath.Join(dir, "poster.jpg"),
		filepath.Join(dir, "folder.jpg"),
		filepath.Join(dir, "cover.jpg"),
		filepath.Join(dir, filepath.Base(dir)+".jpg"),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}
```

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/library/ -run 'TestDisplayName|TestFindPoster' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/library/model.go internal/library/model_test.go
git commit -m "feat(library): 片名清洗与本地海报匹配"
```

---

### Task 3: library 包 — 递归扫描

**Files:**
- Create: `internal/library/scan.go`
- Test: `internal/library/scan_test.go`

**Interfaces:**
- Consumes: `store.LibraryItem`（Task 1）、`displayName`/`findPoster`（Task 2）。
- Produces: `var videoExts = map[string]bool{...}`；`func scanDir(root string) ([]store.LibraryItem, error)`。

- [ ] **Step 1: 写失败测试**

```go
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
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	mustWrite("sub/a.mp4")
	mustWrite("sub/b.mkv")
	mustWrite("sub/c.txt")   // 非视频，忽略
	mustWrite("sub/d.srt")   // 非视频，忽略
	mustWrite("deep/e/webm")

	items, err := scanDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%+v", items)
	}
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Ext] = true
		if it.Dir == "" || it.Name == "" {
			t.Fatalf("item 字段不全: %+v", it)
		}
	}
	for _, ext := range []string{"mp4", "mkv", "webm"} {
		if !seen[ext] {
			t.Fatalf("缺扩展名 %s: %+v", ext, items)
		}
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/library/ -run TestScanDirFiltersAndRecurses -v`
Expected: FAIL — `scanDir` 未定义。

- [ ] **Step 3: 最小实现**

```go
// videoExts 是被识别的视频容器扩展名（D5 白名单）。
var videoExts = map[string]bool{
	"mp4": true, "mkv": true, "m4v": true, "mov": true, "avi": true,
	"flv": true, "ts": true, "m2ts": true, "wmv": true, "rmvb": true,
	"rm": true, "webm": true, "mpg": true, "mpeg": true, "3gp": true, "vob": true,
}

// scanDir 递归扫描 root 下的视频文件，返回规范化条目。
func scanDir(root string) ([]store.LibraryItem, error) {
	var items []store.LibraryItem
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// 权限不足/符号链接失效：跳过该项，不中断整次扫描。
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if !videoExts[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		items = append(items, store.LibraryItem{
			Path:   path,
			Name:   displayName(path),
			Dir:    filepath.Dir(path),
			Ext:    ext,
			Size:   info.Size(),
			MTime:  info.ModTime().Unix(),
			Poster: findPoster(filepath.Dir(path), stem),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
```

> 注：`scanDir` 返回的 `Dir` 是**父目录**（分组键）。Task 5 的门面会把它与「注册目录」对齐。
> 需在 `scan.go` import `store` 与 `strings`。

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/library/ -run TestScanDirFiltersAndRecurses -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/library/scan.go internal/library/scan_test.go
git commit -m "feat(library): 递归扫描与视频识别"
```

---

### Task 4: library 包 — 本地文件 HTTP 服务

**Files:**
- Create: `internal/library/serve.go`
- Test: `internal/library/serve_test.go`

**Interfaces:**
- Produces: `func newServer() *server`；`func (s *server) register(path string) (url string)`；
  `func (s *server) handler() http.Handler`；`func (s *server) close() error`。
  设计：URL 形如 `http://127.0.0.1:PORT/v/<id>?t=<token>`，`id` 是进程内自增映射到绝对路径，
  **不把文件路径暴露进 URL** → 天然防目录穿越。

- [ ] **Step 1: 写失败测试**

```go
package library

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerServesRegisteredFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp4")
	os.WriteFile(p, []byte("fake-video-bytes"), 0o644)

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
	os.WriteFile(p, []byte("x"), 0o644)
	u := s.register(p)
	bad := strings.Replace(u, "?t=", "?t=wrong", 1)
	resp, err := http.Get(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("无 token 应 403, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/library/ -run 'TestServer' -v`
Expected: FAIL — `newServer` 未定义。

- [ ] **Step 3: 最小实现**

```go
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// server 是只监听 127.0.0.1、按进程内 id 映射文件的本地 HTTP 服务（D1）。
type server struct {
	mu    sync.Mutex
	ln    net.Listener
	ids   map[string]string // id -> 绝对路径
	seq   int
	token string
}

func newServer() *server {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &server{ids: map[string]string{}, token: hex.EncodeToString(b)}
}

// register 登记一个文件路径并返回可访问的 URL（懒启动监听）。
func (s *server) register(path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("%d", s.seq)
	s.ids[id] = path
	if s.ln == nil {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err == nil {
			s.ln = ln
			go http.Serve(ln, s.handler())
		}
	}
	host := "127.0.0.1"
	if s.ln != nil {
		host = s.ln.Addr().String()
	}
	return fmt.Sprintf("http://%s/v/%s?t=%s", host, id, s.token)
}

func (s *server) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != s.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		s.mu.Lock()
		path, ok := s.ids[id]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path) // 自带 Range 支持，视频可拖动
	})
}

func (s *server) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}
```

> 注意：`newServer().register` 用懒启动监听，但测试 `TestServerServesRegisteredFile` 里
> `http.Get(u)` 需要 `s.ln` 已就绪——懒启动在 `register` 内同步完成监听绑定后再返回，满足此要求。

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/library/ -run 'TestServer' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/library/serve.go internal/library/serve_test.go
git commit -m "feat(library): 本地文件 HTTP 服务（token 鉴权 + 防穿越）"
```

---

### Task 5: library 包 — Library 门面（目录/扫描/进度/播放流）

**Files:**
- Create: `internal/library/library.go`
- Test: `internal/library/library_test.go`

**Interfaces:**
- Consumes: `store` 方法（Task 1）、`scanDir`（Task 3）、`newServer`（Task 4）、`player.Stream/StreamKind`。
- Produces: `func New(st *store.Store) *Library`；
  `(l *Library) AddDir(path string) error`、`RemoveDir(path) error`、`ListDirs() ([]store.LibraryDir, error)`、
  `Scan() (ScanResult, error)`、`List() ([]store.LibraryItem, error)`、
  `StreamFor(path string) (player.Stream, error)`、`RecordProgress(path string, progress, duration int) error`。

- [ ] **Step 1: 写失败测试**

```go
package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/store"
)

func openTest(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "unbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLibraryScanAndList(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.mp4"), []byte("x"), 0o644)

	st := openTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.AddDir(root); err != nil {
		t.Fatal(err)
	}
	res, err := lib.Scan()
	if err != nil || res.Added != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	items, err := lib.List()
	if err != nil || len(items) != 1 || items[0].Name != "a" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestLibraryStreamForRouting(t *testing.T) {
	lib := New(nil) // StreamFor 不碰 store
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
}

func TestLibraryRecordProgress(t *testing.T) {
	st := openTest(t)
	defer st.Close()
	lib := New(st)
	if err := lib.RecordProgress("/x/a.mp4", 42, 100); err != nil {
		t.Fatal(err)
	}
	h, err := st.ListVodHistory(10)
	if err != nil || len(h) != 1 || h[0].Site != "local" || h[0].Progress != 42 {
		t.Fatalf("h=%+v err=%v", h, err)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/library/ -run 'TestLibrary' -v`
Expected: FAIL — `New` 未定义。

- [ ] **Step 3: 最小实现**

```go
import (
	"os"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/store"
)

// ScanResult 是一次扫描的汇总。
type ScanResult struct {
	Dirs        int
	Added       int
	Removed     int
	Unavailable []string // 注册但当前不可访问的目录
}

// Library 是本地媒体库门面：目录管理 + 扫描 + 播放流 + 进度。
type Library struct {
	store  *store.Store
	server *server
}

func New(st *store.Store) *Library {
	return &Library{store: st, server: newServer()}
}

func (l *Library) AddDir(path string) error {
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("目录不存在或不可访问: %s", path)
	}
	return l.store.AddLibraryDir(path)
}

func (l *Library) RemoveDir(path string) error { return l.store.RemoveLibraryDir(path) }
func (l *Library) ListDirs() ([]store.LibraryDir, error) { return l.store.ListLibraryDirs() }

// Scan 逐个注册目录扫描并整体替换其条目。
func (l *Library) Scan() (ScanResult, error) {
	dirs, err := l.store.ListLibraryDirs()
	if err != nil {
		return ScanResult{}, err
	}
	var res ScanResult
	for _, d := range dirs {
		res.Dirs++
		items, scanErr := scanDir(d.Path)
		if scanErr != nil {
			res.Unavailable = append(res.Unavailable, d.Path)
			_ = l.store.ReplaceLibraryItems(d.Path, nil)
			continue
		}
		// 海报路径登记进文件服务，换成服务 URL 存库
		for i := range items {
			if items[i].Poster != "" {
				items[i].Poster = l.server.register(items[i].Poster)
			}
		}
		if err := l.store.ReplaceLibraryItems(d.Path, items); err != nil {
			return res, err
		}
		res.Added += len(items)
	}
	return res, nil
}

func (l *Library) List() ([]store.LibraryItem, error) { return l.store.ListLibraryItems() }

// StreamFor 按扩展名分流（D1 方案 B）：mp4/m4v/webm 走本地 HTTP + Web，其余走 file:// + mpv。
func (l *Library) StreamFor(path string) (player.Stream, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "mp4", "m4v", "webm":
		return player.Stream{URL: l.server.register(path), Kind: player.StreamMP4}, nil
	default:
		return player.Stream{URL: "file://" + filepath.ToSlash(path), Kind: player.StreamLocal}, nil
	}
}

// RecordProgress 以 site="local" 复用 vod_history，供首页续播。
func (l *Library) RecordProgress(path string, progress, duration int) error {
	return l.store.UpsertVodHistory(store.VodHistory{
		Site: "local", VodID: path, VodTitle: displayName(path),
		Source: "local", Progress: progress, Duration: duration,
	})
}
```

> `library.go` 需要 import `fmt`、`path/filepath`、`player`、`store`。`TestLibraryStreamForRouting`
> 用 `New(nil)` 不会触发 store，安全（`StreamFor` 只碰 `server`）。

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/library/ -run 'TestLibrary' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/library/library.go internal/library/library_test.go
git commit -m "feat(library): 目录管理/扫描/播放流/进度门面"
```

---

### Task 6: ShellService 集成与 Wails 绑定

**Files:**
- Modify: `internal/shell/service.go`（`ShellService` 加 `library *library.Library` 字段；`NewShellService` 里 `library.New(st)`；新增 7 个公开方法）
- Test: `internal/shell/service_test.go`

**Interfaces:**
- Consumes: `library.Library`（Task 5）、`playback.Plan`。
- Produces（前端绑定面）：`AddLibraryDir(path string) error`、`RemoveLibraryDir(path string) error`、
  `ListLibraryDirs() ([]library.LibraryDir, error)`、`ScanLibrary() (library.ScanResult, error)`、
  `ListLibrary() ([]library.LibraryItem, error)`、`PrepareLibrary(path string) (playback.Plan, error)`、
  `RecordLibraryProgress(path string, progress, duration float64) error`。

- [ ] **Step 1: 写失败测试**

```go
func TestLibraryDirRoundTrip(t *testing.T) {
    svc := newTestService(t) // 既有 helper；若无则 NewShellService(nil, nil, openTest(t))
    if err := svc.AddLibraryDir("/tmp"); err != nil {
        t.Fatal(err)
    }
    dirs, err := svc.ListLibraryDirs()
    if err != nil || len(dirs) != 1 {
        t.Fatalf("dirs=%+v err=%v", dirs, err)
    }
    if err := svc.RemoveLibraryDir("/tmp"); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: 运行验证失败**

Run: `go test ./internal/shell/ -run TestLibraryDirRoundTrip -v`
Expected: FAIL — `svc.AddLibraryDir` 未定义。

- [ ] **Step 3: 最小实现**

`service.go` 的 `ShellService` 结构体加字段 `library *library.Library`；`NewShellService` 内、
`controller := ...` 之后加：

```go
lib := library.New(st)
```

return 结构体里加 `library: lib,`。文件底部追加：

```go
func (s *ShellService) AddLibraryDir(path string) error { return s.library.AddDir(path) }
func (s *ShellService) RemoveLibraryDir(path string) error { return s.library.RemoveDir(path) }
func (s *ShellService) ListLibraryDirs() ([]library.LibraryDir, error) { return s.library.ListDirs() }
func (s *ShellService) ScanLibrary() (library.ScanResult, error) { return s.library.Scan() }
func (s *ShellService) ListLibrary() ([]library.LibraryItem, error) { return s.library.List() }

func (s *ShellService) PrepareLibrary(path string) (playback.Plan, error) {
	stream, err := s.library.StreamFor(path)
	if err != nil {
		return playback.Plan{}, err
	}
	return s.playback.Prepare(context.Background(), stream)
}

func (s *ShellService) RecordLibraryProgress(path string, progress, duration float64) error {
	return s.library.RecordProgress(path, int(progress), int(duration))
}
```

> `ShellService` 已有 `playback *playback.Controller`、`store *store.Store` 字段；`library` 与之并列。
> `service.go` 需 import `github.com/unbox/unbox/internal/library`。

- [ ] **Step 4: 运行验证通过**

Run: `go test ./internal/shell/ -run TestLibraryDirRoundTrip -v`
Expected: PASS。

- [ ] **Step 5: 全量回归 + Commit**

Run: `go test ./... -count=1`、`go vet ./...`、`gofmt -l .`
Expected: 全绿。

```bash
git add internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(shell): 媒体库 Wails 绑定方法"
```

---

### Task 7: 前端「媒体库」tab

**Files:**
- Modify: `frontend/src/App.vue`（nav 加「媒体库」按钮 + `mode==='library'` 视图）
- Create: `frontend/src/library.ts`（可选：绑定封装与类型）
- Regen: `frontend/bindings`（`wails3 generate bindings` 或 project 既有命令）

**Interfaces:**
- Consumes: 前端既有 `switchMode`/`mode` 状态机、`plan.Backend` 播放分支、`backends.*` 播放调用。

- [ ] **Step 1: 加 tab 按钮**

`App.vue` 的 `<nav class="tabs">`（约 506 行）在「直播」与「设置」之间插入：

```html
<button :class="{ active: mode === 'library' }" @click="switchMode('library')">媒体库</button>
```

`mode` 的联合类型（`switchMode` 的入参）补 `'library'`。

- [ ] **Step 2: 加视图与状态**

模板区（「直播」块之后）加：

```html
<!-- 媒体库 -->
<section v-if="mode === 'library'" class="library-view">
  <div class="library-dirs">
    <h3>媒体目录</h3>
    <button @click="addLibraryDir">添加目录</button>
    <button @click="rescanLibrary">重新扫描</button>
    <ul>
      <li v-for="d in libraryDirs" :key="d.Path">
        {{ d.Path }} <button @click="removeLibraryDir(d.Path)">移除</button>
      </li>
    </ul>
    <input v-if="addingDir" v-model="newDir" placeholder="输入目录绝对路径" />
    <button v-if="addingDir" @click="confirmAddDir">确定</button>
  </div>
  <div class="library-list">
    <h3>影片</h3>
    <p v-if="!libraryItems.length" class="home-empty">尚未扫描到视频</p>
    <div v-for="it in libraryItems" :key="it.Path" class="library-item" @click="playLibraryItem(it.Path)">
      <img v-if="it.Poster" :src="it.Poster" alt="" />
      <span>{{ it.Name }}</span>
    </div>
  </div>
</section>
```

script 区加：

```ts
const libraryDirs = ref<any[]>([])
const libraryItems = ref<any[]>([])
const addingDir = ref(false)
const newDir = ref('')

async function loadLibrary() {
  libraryDirs.value = await ListLibraryDirs()
  libraryItems.value = await ListLibrary()
}
async function addLibraryDir() { addingDir.value = true }
async function confirmAddDir() {
  await AddLibraryDir(newDir.value)
  newDir.value = ''; addingDir.value = false
  await loadLibrary()
}
async function removeLibraryDir(path: string) {
  await RemoveLibraryDir(path); await loadLibrary()
}
async function rescanLibrary() {
  await ScanLibrary(); await loadLibrary()
}
async function playLibraryItem(path: string) {
  const plan = await PrepareLibrary(path)
  // 复用现有点播播放分支：plan.Backend === 'web' 走 Web，否则走 mpv
}
```

`switchMode` 里 `mode === 'library'` 时调 `loadLibrary()`。

- [ ] **Step 3: 播放分支复用**

`playLibraryItem` 内按 `plan.Backend` 复用现有 `playVod`/`prepareVod` 之后的 Web/mpv 分支
（既有 `playback-view` 已支持 `plan` 驱动）。进度回传复用现有 10s 轮询，在轮询回调里
当 `mode==='library'` 时改调 `RecordLibraryProgress(当前路径, pos, dur)`。

- [ ] **Step 4: 校验**

Run: `cd frontend && npm run build`（含 `vue-tsc --noEmit`）、`npm test`
Expected: 通过。手动 `mise run dev` 验证：添加目录 → 扫描 → 列表 → 播放 MP4（Web）/MKV（mpv）。

- [ ] **Step 5: Commit**

```bash
git add frontend/
git commit -m "feat(frontend): 媒体库 tab 与浏览/播放"
```

---

## Self-Review

**1. Spec 覆盖**：目录管理→Task 5/6；扫描+失效→Task 3/5；浏览→Task 7；播放分流 D1→Task 4/5；
进度+首页续播→Task 5/6/7；元数据仅文件名/NFO→Task 2；验收标准各条映射到对应 Task。

**2. Placeholder 扫描**：无 TBD/TODO；每个步骤都有可运行代码。

**3. 类型一致**：`store.LibraryDir/LibraryItem`（Task 1）被 Task 3/5 消费；
`scanDir` 返回 `[]store.LibraryItem`；`New(st)` 门面统一入口；`ScanResult` 字段在 Task 5 定义、
Task 6 透传。一致。

**执行交接**：任务间有依赖（1→2→3→4→5→6→7），须串行。Task 7 依赖 `frontend/bindings`
由 `wails3 generate bindings` 重新生成（ShellService 新增方法后）。
