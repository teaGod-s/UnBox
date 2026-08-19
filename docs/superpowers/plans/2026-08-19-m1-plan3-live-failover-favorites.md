# Unbox M1 Plan 3 实现计划：直播浏览/播放 + 测速切换 + 收藏持久化

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补齐 M1 剩余功能：直播频道浏览与播放、多源测速 + 失败自动切换、收藏/最近观看/自定义分组（SQLite 持久化），并把它们接入桌面壳与前端。

**Architecture:** 在已完成的 Player 抽象（Plan 2）之上新增三层：`provider`（来源接口 + `live` 直播实现）、`probe`（测速）、`player/failover`（失败自动切换包装器），再加 `store`（SQLite 持久化）。`shell` 把四者接成一条「导入订阅 → 频道列表 → 点击播放（带切换）→ 收藏」的闭环，前端通过 Wails 自动生成的绑定调用。

**Tech Stack:** Go 1.26.3、Wails v3.0.0-beta.9（钉死）、Vue 3 + TypeScript + Vite、`modernc.org/sqlite`（纯 Go，无 cgo）。

**Spec:** `docs/superpowers/specs/2026-08-17-unbox-m1-design.md`（§3.1 接口、§3.4 失败切换、§4 范围、§7 验收）

## Global Constraints

- Go 1.26.3；module 路径 `github.com/unbox/unbox`。
- Wails 钉死 `3.0.0-beta.9`（**禁止 @latest**）；Wails 代码只允许出现在 `internal/shell/`、`cmd/unbox/`、`frontend/`。
- `internal/provider/`、`internal/player/`、`internal/probe/`、`internal/store/` **不得 import Wails**。
- 新增依赖仅 `modernc.org/sqlite`（纯 Go 无 cgo）；用 `go get` 解析后**把解析出的版本钉进 go.mod**，不允许未钉版本。
- 交叉编译不可行（spec §5.2），本环境仅 Linux 构建；Windows named-pipe + `--wid` 嵌入、macOS mpvlib 仍顺延（见 spec §3.4，本计划不交付，需真实平台机器）。
- 前端 Vue3 + TypeScript，沿用现有 `public/style.css` 明式样式（项目未实际安装 Tailwind，**不新引入**）。
- 现有已合并代码（config/player/shell）只在任务明确写「Modify」时改动；其余只增不改。
- TDD：每个任务先写失败测试再实现；`go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿方可提交；每任务独立 commit。
- 所有公开错误信息用中文（与现有代码一致）。

---

## 文件结构

```
internal/
├── provider/
│   ├── provider.go          Provider 接口 + Section/Item/Page/Media（新建）
│   └── live/
│       ├── m3u.go            M3U/TXT 解析器（纯函数，新建）
│       ├── m3u_test.go       （新建）
│       ├── live.go           Live Provider 实现（新建）
│       └── live_test.go      （新建）
├── probe/
│   ├── probe.go              测速与排序（新建）
│   └── probe_test.go         （新建）
├── player/
│   └── failover/
│       ├── failover.go       失败自动切换包装器（新建）
│       └── failover_test.go  （新建）
├── store/
│   ├── store.go              SQLite 持久化（新建）
│   └── store_test.go         （新建）
├── shell/
│   ├── service.go            订阅导入 + 目录 + 播放 + 收藏接线（新建）
│   ├── service_test.go       （新建）
│   └── app.go                Modify：ShellService 加字段
cmd/unbox/main.go             Modify：注入 provider/store/probe 到 shell
frontend/src/App.vue          Modify：频道列表 + 播放 + 收藏 UI
```

**接口依赖（跨任务契约）**

- Task 2 产出 `provider.Provider`（5 方法）、`provider.Section/Item/Page/Media`；Task 6 消费。
- Task 2 产出 `live.Provider`（`New`、`FetchLive`）；Task 6 消费。
- Task 3 产出 `probe.Prober`（`Probe`、`Rank`）；Task 4/6 消费。
- Task 4 产出 `failover.New(inner, prober) player.Player`；Task 6 消费。
- Task 5 产出 `store.Store`（Open/Close + 收藏/最近/分组 CRUD）；Task 6 消费。

---

### Task 1: M3U / TXT 解析器

**Files:**
- Create: `internal/provider/live/m3u.go`
- Test: `internal/provider/live/m3u_test.go`

**Interfaces:**
- Consumes: 无（纯函数，仅标准库）。
- Produces:
  ```go
  type Entry struct {
      Name  string
      URL   string
      Logo  string
      Group string
      ID    string // tvg-id，可能为空
  }
  func ParseM3U(raw []byte) []Entry   // 解析 #EXTM3U 播放列表
  func ParseTXT(raw []byte) []Entry   // 解析「名称,URL」每行一条的 TXT
  ```

- [ ] **Step 1: 写失败测试**

`m3u_test.go`：

```go
package live

import (
	"reflect"
	"testing"
)

func TestParseM3UExtractsAttributes(t *testing.T) {
	raw := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-id=\"cctv1\" tvg-logo=\"http://x/1.png\" group-title=\"央视\",CCTV-1 综合\n" +
		"http://x/live/cctv1.m3u8\n" +
		"#EXTINF:-1 group-title=\"卫视\",湖南卫视\n" +
		"http://x/live/hunan.ts\n"
	got := ParseM3U(raw)
	want := []Entry{
		{Name: "CCTV-1 综合", URL: "http://x/live/cctv1.m3u8", Logo: "http://x/1.png", Group: "央视", ID: "cctv1"},
		{Name: "湖南卫视", URL: "http://x/live/hunan.ts", Group: "卫视"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseM3U = %#v, want %#v", got, want)
	}
}

func TestParseM3UToleratesBOMCRLFAndJunk(t *testing.T) {
	raw := "\xef\xbb\xbf#EXTM3U\r\n" +
		"#EXTINF:-1,频道A\r\n" +
		"http://x/a\r\n" +
		"\r\n" +
		"# 注释行\r\n" +
		"http://x/orphan-without-extinf\r\n" // 无 #EXTINF 前导的 URL 应被跳过
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].Name != "频道A" {
		t.Fatalf("ParseM3U = %#v, want 仅 1 条频道A", got)
	}
}

func TestParseM3UMissingURLDropsEntry(t *testing.T) {
	raw := "#EXTM3U\n#EXTINF:-1,只有名字没有 URL\n#EXTINF:-1,正常\nhttp://x/ok\n"
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].URL != "http://x/ok" {
		t.Fatalf("ParseM3U = %#v, want 1 条", got)
	}
}

func TestParseTXT(t *testing.T) {
	raw := "频道一,http://x/1\n频道二,http://x/2\n"
	got := ParseTXT(raw)
	if len(got) != 2 || got[1].Name != "频道二" || got[1].URL != "http://x/2" {
		t.Fatalf("ParseTXT = %#v", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/provider/live/ -run TestParse -v`
Expected: FAIL（`undefined: ParseM3U` 等）

- [ ] **Step 3: 实现**

`m3u.go`：

```go
// Package live 实现 M1 的 IPTV 直播来源：M3U/TXT 解析 + Provider 适配。
package live

import (
	"bufio"
	"bytes"
	"strings"
)

// Entry 是 M3U/TXT 播放列表中的一条媒体条目。
type Entry struct {
	Name  string
	URL   string
	Logo  string
	Group string
	ID    string // tvg-id
}

// ParseM3U 解析 #EXTM3U 播放列表。#EXTINF 行中的 tvg-id / tvg-logo /
// group-title 属性被提取，逗号后的标题作为 Name。容错：剥 BOM、容忍
// CRLF、跳过空行与 # 注释行、无 #EXTINF 前导的 URL 跳过、#EXTINF 后
// 没有 URL 的条目丢弃。
func ParseM3U(raw []byte) []Entry {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	var out []Entry
	var cur *Entry // 当前待配对 URL 的 #EXTINF
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#EXTM3U") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXTINF") {
				e := parseExtinf(line)
				cur = &e
			}
			continue
		}
		// URL 行：必须已有 #EXTINF 前导才配对
		if cur != nil {
			cur.URL = line
			out = append(out, *cur)
			cur = nil
		}
	}
	return out
}

// parseExtinf 解析形如 `#EXTINF:-1 attr="v" attr2="v2",Name` 的行。
func parseExtinf(line string) Entry {
	var e Entry
	rest := strings.TrimPrefix(line, "#EXTINF")
	// 去掉时长字段（到第一个逗号为止），剩余是属性段
	if i := strings.Index(rest, ","); i >= 0 {
		e.Name = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	e.ID = attr(rest, "tvg-id")
	e.Logo = attr(rest, "tvg-logo")
	e.Group = attr(rest, "group-title")
	return e
}

// attr 从 `key="value"` 形式的属性串中取值；不存在返回 ""。
func attr(s, key string) string {
	needle := key + "="
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	s = s[i+len(needle):]
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, `"`) {
		s = strings.TrimPrefix(s, `"`)
		if j := strings.Index(s, `"`); j >= 0 {
			return s[:j]
		}
	}
	if j := strings.IndexAny(s, " \t"); j >= 0 {
		return s[:j]
	}
	return s
}

// ParseTXT 解析「名称,URL」每行一条的简单 TXT 播放列表。
func ParseTXT(raw []byte) []Entry {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	var out []Entry
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, url, ok := strings.Cut(line, ",")
		name = strings.TrimSpace(name)
		url = strings.TrimSpace(url)
		if !ok || name == "" || url == "" {
			continue
		}
		out = append(out, Entry{Name: name, URL: url})
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/provider/live/ -run TestParse -v`
Expected: PASS（4 用例）

- [ ] **Step 5: Commit**

```bash
git add internal/provider/live/m3u.go internal/provider/live/m3u_test.go
git commit -m "feat(live): M3U/TXT 播放列表解析器"
```

---

### Task 2: Provider 接口 + Live Provider

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/live/live.go`
- Test: `internal/provider/live/live_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Entry`/`ParseM3U`/`ParseTXT`；`config.Channel`/`config.Live`/`config.Fetcher`；`player.Stream`。
- Produces: `provider.Provider`、`provider.Section/Item/Page/Media`；`live.New`、`live.FetchLive`、`live.Assemble`。

- [ ] **Step 1: 写 Provider 接口**

`internal/provider/provider.go`：

```go
// Package provider 定义所有内容来源的统一接口。所有来源实现 Provider，
// UI 层只依赖接口，不依赖具体实现（spec §3.1）。
package provider

import (
	"context"

	"github.com/unbox/unbox/internal/player"
)

// Section 是首页的一个分组。
type Section struct {
	ID    string
	Title string
}

// Item 是浏览列表中的一项（直播=频道）。
type Item struct {
	ID    string
	Title string
	Logo  string
	Group string
}

// Page 是一页浏览结果。
type Page struct {
	Items []Item
}

// Media 是详情（M1 直播仅含频道元信息；M2 点播再扩展剧集字段）。
type Media struct {
	ID    string
	Title string
	Logo  string
	Group string
}

// Provider 是所有来源的统一接口。
type Provider interface {
	ID() string
	Home(ctx context.Context) ([]Section, error)
	Browse(ctx context.Context, cat string, page int) (Page, error)
	Search(ctx context.Context, q string) ([]Item, error)
	Detail(ctx context.Context, id string) (Media, error)
	Resolve(ctx context.Context, id string) (player.Stream, error)
}
```

- [ ] **Step 2: 写失败测试**

`live_test.go`：

```go
package live

import (
	"context"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
)

func sampleChannels() []config.Channel {
	return []config.Channel{
		{Name: "CCTV-1", URLs: []string{"http://x/1.m3u8", "http://x/1b.m3u8"}, Logo: "l1", Group: "央视"},
		{Name: "湖南卫视", URLs: []string{"http://x/hunan.ts"}, Group: "卫视"},
	}
}

func TestNewHomeAndBrowse(t *testing.T) {
	p := New(sampleChannels())
	if p.ID() != "live" {
		t.Fatalf("ID = %q", p.ID())
	}
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 2 {
		t.Fatalf("Home = %v, %v", secs, err)
	}
	pg, err := p.Browse(context.Background(), "央视", 0)
	if err != nil || len(pg.Items) != 1 || pg.Items[0].Title != "CCTV-1" {
		t.Fatalf("Browse = %v, %v", pg, err)
	}
}

func TestResolveBackups(t *testing.T) {
	p := New(sampleChannels())
	id := "央视/CCTV-1"
	st, err := p.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if st.URL != "http://x/1.m3u8" || len(st.Backups) != 1 || st.Backups[0] != "http://x/1b.m3u8" {
		t.Fatalf("Resolve = %+v", st)
	}
	if st.Kind != player.StreamHLS {
		t.Fatalf("Kind = %v, want HLS", st.Kind)
	}
}

func TestResolveUnknownID(t *testing.T) {
	p := New(sampleChannels())
	if _, err := p.Resolve(context.Background(), "不存在/x"); err == nil {
		t.Fatal("未知 ID 应报错")
	}
}

func TestFetchLiveParsesM3U(t *testing.T) {
	// 用 httptest 起一个 m3u 服务，验证 FetchLive 拉取 + 解析 + 归并同名备份。
	srv := newM3UTestServer(t, "#EXTM3U\n"+
		"#EXTINF:-1 group-title=\"测试\",频道A\nhttp://srv/a\n"+
		"#EXTINF:-1 group-title=\"测试\",频道A\nhttp://srv/a2\n")
	defer srv.Close()
	chs, err := FetchLive(context.Background(), config.Live{URL: srv.URL}, config.NewFetcher())
	if err != nil {
		t.Fatalf("FetchLive: %v", err)
	}
	if len(chs) != 1 || len(chs[0].URLs) != 2 {
		t.Fatalf("FetchLive = %+v，期望同名归并为 1 频道 2 备份", chs)
	}
}
```

（`newM3UTestServer` 用 `httptest.NewServer` 返回指定 body；在 `live_test.go` 底部实现。）

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/provider/live/ -run 'TestNew|TestResolve|TestFetch' -v`
Expected: FAIL（`undefined: New` 等）

- [ ] **Step 4: 实现**

`internal/provider/live/live.go`：

```go
package live

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// pageSize 是 Browse 每页条数。
const pageSize = 200

// Provider 是直播 IPTV 的 Provider 实现。
type Provider struct {
	groups   []string
	channels []config.Channel
	byID     map[string]int
}

// New 从扁平频道列表构建直播源，并按 Group 有序分组。
func New(channels []config.Channel) *Provider {
	p := &Provider{byID: make(map[string]int)}
	seen := map[string]bool{}
	for _, ch := range channels {
		if !seen[ch.Group] {
			seen[ch.Group] = true
			p.groups = append(p.groups, ch.Group)
		}
		p.byID[channelID(ch.Group, ch.Name)] = len(p.channels)
		p.channels = append(p.channels, ch)
	}
	sort.Strings(p.groups)
	return p
}

// channelID 是频道在 Provider 内的稳定标识：group/name。
func channelID(group, name string) string { return group + "/" + name }

func (p *Provider) ID() string { return "live" }

func (p *Provider) Home(ctx context.Context) ([]provider.Section, error) {
	out := make([]provider.Section, 0, len(p.groups))
	for _, g := range p.groups {
		out = append(out, provider.Section{ID: g, Title: g})
	}
	return out, nil
}

func (p *Provider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	var items []provider.Item
	for _, ch := range p.channels {
		if cat != "" && cat != "*" && ch.Group != cat {
			continue
		}
		items = append(items, provider.Item{ID: channelID(ch.Group, ch.Name), Title: ch.Name, Logo: ch.Logo, Group: ch.Group})
	}
	start := page * pageSize
	if start >= len(items) {
		return provider.Page{}, nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return provider.Page{Items: items[start:end]}, nil
}

func (p *Provider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	q = strings.ToLower(q)
	var out []provider.Item
	for _, ch := range p.channels {
		if strings.Contains(strings.ToLower(ch.Name), q) {
			out = append(out, provider.Item{ID: channelID(ch.Group, ch.Name), Title: ch.Name, Logo: ch.Logo, Group: ch.Group})
		}
	}
	return out, nil
}

func (p *Provider) Detail(ctx context.Context, id string) (provider.Media, error) {
	i, ok := p.byID[id]
	if !ok {
		return provider.Media{}, fmt.Errorf("频道不存在: %s", id)
	}
	ch := p.channels[i]
	return provider.Media{ID: id, Title: ch.Name, Logo: ch.Logo, Group: ch.Group}, nil
}

func (p *Provider) Resolve(ctx context.Context, id string) (player.Stream, error) {
	i, ok := p.byID[id]
	if !ok {
		return player.Stream{}, fmt.Errorf("频道不存在: %s", id)
	}
	ch := p.channels[i]
	primary := ""
	var backups []string
	if len(ch.URLs) > 0 {
		primary = ch.URLs[0]
		backups = append(backups, ch.URLs[1:]...)
	}
	if primary == "" {
		return player.Stream{}, fmt.Errorf("频道 %s 没有可用流地址", ch.Name)
	}
	return player.Stream{URL: primary, Backups: backups, Kind: kindForURL(primary)}, nil
}

// FetchLive 拉取 Live.URL 指向的 m3u 并解析为频道；同名条目归并为同一频道的
// 多个备用源（URLs）。Live.Channels 非空时调用方直接使用它们，无需走本函数。
func FetchLive(ctx context.Context, l config.Live, fetcher *config.Fetcher) ([]config.Channel, error) {
	raw, err := fetcher.Fetch(ctx, l.URL)
	if err != nil {
		return nil, fmt.Errorf("拉取直播源 %s 失败: %w", l.Name, err)
	}
	return Assemble(ParseM3U(raw)), nil
}

// Assemble 把扁平 Entry 按 Name+Group 归并为 Channel（同名=同频道多备用源）。
// 导出供 shell 处理独立 M3U/TXT 导入。
func Assemble(entries []Entry) []config.Channel {
	var out []config.Channel
	idx := map[string]int{}
	for _, e := range entries {
		key := channelID(e.Group, e.Name)
		if i, ok := idx[key]; ok {
			out[i].URLs = append(out[i].URLs, e.URL)
			continue
		}
		idx[key] = len(out)
		out = append(out, config.Channel{Name: e.Name, Group: e.Group, Logo: e.Logo, URLs: []string{e.URL}})
	}
	return out
}

// kindForURL 依据 URL 扩展名猜测流形态；直播默认按 HLS 处理（mpv 对 TS/裸流
// 同样按直播协议播放）。
func kindForURL(u string) player.StreamKind {
	p := u
	if parsed, err := url.Parse(u); err == nil {
		p = parsed.Path
	}
	switch {
	case strings.HasSuffix(p, ".mp4"):
		return player.StreamMP4
	case strings.HasSuffix(p, ".flv"):
		return player.StreamFLV
	default:
		return player.StreamHLS
	}
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/provider/... -count=1 -v`
Expected: PASS（Task 1 + Task 2 全部用例）

- [ ] **Step 6: Commit**

```bash
git add internal/provider/
git commit -m "feat(provider): Provider 接口 + live 直播实现（M3U 拉取/归并/解析）"
```

---

### Task 3: Probe 测速

**Files:**
- Create: `internal/probe/probe.go`
- Test: `internal/probe/probe_test.go`

**Interfaces:**
- Consumes: 无（仅标准库）。
- Produces:
  ```go
  type Result struct { URL string; Reachable bool; Latency time.Duration; Speed int64; Err error }
  type Prober struct { ... }
  func NewProber() *Prober
  func (p *Prober) Probe(ctx context.Context, url string, headers map[string]string) Result
  func (p *Prober) Rank(ctx context.Context, urls []string, headers map[string]string) []string
  ```

- [ ] **Step 1: 写失败测试**

`probe_test.go`：

```go
package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(make([]byte, 64*1024))
	}))
	defer srv.Close()
	p := NewProber()
	r := p.Probe(context.Background(), srv.URL, nil)
	if !r.Reachable || r.Latency <= 0 || r.Speed <= 0 || r.Err != nil {
		t.Fatalf("Probe = %+v，期望可达且测出延迟/吞吐", r)
	}
}

func TestProbeUnreachable(t *testing.T) {
	p := NewProber()
	r := p.Probe(context.Background(), "http://127.0.0.1:1/nope", nil)
	if r.Reachable || r.Err == nil {
		t.Fatalf("Probe = %+v，期望不可达", r)
	}
}

func TestRankPutsReachableFirst(t *testing.T) {
	var reachable, dead string
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer deadSrv.Close()
	reachable = deadSrv.URL
	dead = "http://127.0.0.1:1/nope"
	got := NewProber().Rank(context.Background(), []string{dead, reachable}, nil)
	if got[0] != reachable {
		t.Fatalf("Rank = %v，期望可达源排最前", got)
	}
}

func TestRankSingleURLUnchanged(t *testing.T) {
	got := NewProber().Rank(context.Background(), []string{"http://x/1"}, nil)
	if len(got) != 1 || got[0] != "http://x/1" {
		t.Fatalf("Rank = %v", got)
	}
}

func TestRankBoundsTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer slow.Close()
	start := time.Now()
	NewProber().Rank(context.Background(), []string{slow.URL}, nil)
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("Rank 未受 1s 探测超时约束，耗时 %v", time.Since(start))
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/probe/ -v`
Expected: FAIL（`undefined: NewProber` 等）

- [ ] **Step 3: 实现**

`internal/probe/probe.go`：

```go
// Package probe 对直播流地址做连通性/首字节延迟/吞吐测量，并按可达性排序，
// 供失败自动切换挑选最优备用源。
package probe

import (
	"context"
	"io"
	"net/http"
	"sort"
	"time"
)

// probeTimeout 是单条 URL 的探测超时。
const probeTimeout = time.Second

// sampleBytes 是吞吐采样读取量：读到这么多字节就停止，足够区分快慢源。
const sampleBytes = 128 * 1024

// Result 是一次测速的结果。
type Result struct {
	URL       string
	Reachable bool
	Latency   time.Duration // 首字节延迟
	Speed     int64         // 字节/秒（估算）
	Err       error
}

// Prober 对 URL 做测速与排序。
type Prober struct {
	Client *http.Client
}

// NewProber 返回默认 Prober（单 URL 超时 1s）。
func NewProber() *Prober {
	return &Prober{Client: &http.Client{Timeout: probeTimeout}}
}

// Probe 测量单个 URL：GET 请求，读至多 sampleBytes 字节，统计首字节延迟与吞吐。
// 不读完整响应体（直播流是无限流，读到采样量即止）。
func (p *Prober) Probe(ctx context.Context, url string, headers map[string]string) Result {
	r := Result{URL: url}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		r.Err = err
		return r
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := p.Client.Do(req)
	if err != nil {
		r.Err = err
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		return r
	}
	r.Reachable = true
	r.Latency = time.Since(start)
	n, _ := io.CopyN(io.Discard, resp.Body, sampleBytes)
	elapsed := time.Since(start)
	if elapsed > 0 && n > 0 {
		r.Speed = int64(float64(n) / elapsed.Seconds())
	}
	return r
}

// Rank 对 URL 列表测速并排序：可达优先，其次吞吐降序，再次延迟升序。
// 单条 URL 直接返回（不探测）。测速失败的源排到末尾（保留原始相对顺序）。
func (p *Prober) Rank(ctx context.Context, urls []string, headers map[string]string) []string {
	if len(urls) <= 1 {
		return append([]string(nil), urls...)
	}
	type scored struct {
		url string
		res Result
	}
	items := make([]scored, len(urls))
	for i, u := range urls {
		items[i] = scored{url: u, res: p.Probe(ctx, u, headers)}
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := items[i].res, items[j].res
		if ri.Reachable != rj.Reachable {
			return ri.Reachable
		}
		if ri.Speed != rj.Speed {
			return ri.Speed > rj.Speed
		}
		return ri.Latency < rj.Latency
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.url
	}
	return out
}
```

（注意：`fmt` 需要加入 import。）

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/probe/ -count=1 -v`
Expected: PASS（5 用例）

- [ ] **Step 5: Commit**

```bash
git add internal/probe/
git commit -m "feat(probe): 直播流测速与可达性排序"
```

---

### Task 4: mpvproc 事件硬化 + Failover 自动切换

**Files:**
- Modify: `internal/player/mpvproc/mpvproc.go`（readLoop 终端事件不丢弃）
- Create: `internal/player/failover/failover.go`
- Test: `internal/player/failover/failover_test.go`

**Interfaces:**
- Consumes: `player.Player`/`player.Stream`/`player.Event`；`probe.Prober`。
- Produces: `failover.New(inner player.Player, prober *probe.Prober) player.Player`。

**背景（本任务必须先修的既有缺陷）**：mpvproc 的 `readLoop` 当前对所有事件在 channel 满时 `default` 丢弃，导致 EOF/Error 这类终端事件可能被丢，自动切换永远等不到切换信号。修法：Position/Buffering 仍可丢，EOF/Error 改为**阻塞发送**（终端事件稀少，短暂阻塞可接受）。

- [ ] **Step 1: 修 mpvproc 事件丢弃（写失败测试）**

先把事件分发抽成一个可单测的纯函数 `sendEvent`（见 Step 3），再为其写测试。
在 `internal/player/mpvproc/mpvproc_test.go` 增加：

```go
func TestSendEventTerminalBlocksUntilRead(t *testing.T) {
	ch := make(chan player.Event) // 无缓冲：无人读取时必然阻塞
	done := make(chan struct{})
	go func() {
		sendEvent(ch, player.Event{Kind: player.EventEOF})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("终端事件在无人读取时被丢弃了（不应发生）")
	case <-time.After(20 * time.Millisecond):
		// 预期：阻塞在发送上，done 未关闭
	}
	<-ch  // 读取后放行
	<-done
}

func TestSendEventPositionDropsWhenFull(t *testing.T) {
	ch := make(chan player.Event, 1)
	ch <- player.Event{Kind: player.EventPosition} // 占满
	sendEvent(ch, player.Event{Kind: player.EventPosition}) // 应丢弃而非阻塞
}
```

（`time` 需加入 import。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/player/... -count=1`
Expected: FAIL（新测试失败，或 failover 测试尚未实现而编译失败）

- [ ] **Step 3: 改 readLoop**

在 `mpvproc.go` 新增可单测的分发函数，并把 readLoop 的事件分发改为调用它：

```go
// sendEvent 把事件发往通道：终端事件（EOF/Error）阻塞发送保证送达
// （失败自动切换依赖它们），其余事件通道满则丢弃以避免阻塞读循环。
func sendEvent(ch chan<- player.Event, evt player.Event) {
	switch evt.Kind {
	case player.EventEOF, player.EventError:
		ch <- evt
	default:
		select {
		case ch <- evt:
		default:
		}
	}
}
```

readLoop 中原来的：

```go
select {
case p.events <- evt:
default: // 事件通道满则丢弃，避免阻塞读循环
}
```

改为 `sendEvent(p.events, evt)`（位置事件的分发与 state 更新逻辑不变，仍保留在 readLoop 内）。

- [ ] **Step 4: 写 failover 失败测试**

`internal/player/failover/failover_test.go`：

```go
package failover

import (
	"context"
	"sync"
	"testing"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/probe"
)

// fakePlayer 是一个可编程的 inner Player，用于驱动事件。
type fakePlayer struct {
	mu       sync.Mutex
	loaded   []string
	events   chan player.Event
	playErr  error
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{events: make(chan player.Event, 16)}
}

func (f *fakePlayer) Load(ctx context.Context, s player.Stream) error {
	f.mu.Lock()
	f.loaded = append(f.loaded, s.URL)
	f.mu.Unlock()
	return nil
}
func (f *fakePlayer) emit(k player.EventKind) { f.events <- player.Event{Kind: k} }
func (f *fakePlayer) Play() error             { return nil }
func (f *fakePlayer) Pause() error            { return nil }
func (f *fakePlayer) Seek(float64) error      { return nil }
func (f *fakePlayer) SetVolume(int) error     { return nil }
func (f *fakePlayer) SelectTrack(player.TrackKind, int) error { return nil }
func (f *fakePlayer) State() player.State     { return player.State{} }
func (f *fakePlayer) Events() <-chan player.Event { return f.events }
func (f *fakePlayer) Close() error            { return nil }
func (f *fakePlayer) loadedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.loaded...)
}

func TestFailoverSwitchesOnError(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil) // 无 prober：按原始顺序切换
	s := player.Stream{URL: "http://primary", Backups: []string{"http://b1", "http://b2"}}
	if err := fp.Load(context.Background(), s); err != nil {
		t.Fatalf("Load: %v", err)
	}
	inner.emit(player.EventError)
	inner.emit(player.EventError) // 再错一次，切到 b2
	waitFor(t, func() bool { return len(inner.loadedURLs()) == 3 })
	got := inner.loadedURLs()
	if got[0] != "http://primary" || got[1] != "http://b1" || got[2] != "http://b2" {
		t.Fatalf("loaded = %v，期望按序切换", got)
	}
}

func TestFailoverStopsWhenExhausted(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	fp.Load(context.Background(), player.Stream{URL: "http://only"})
	for i := 0; i < 5; i++ {
		inner.emit(player.EventError)
	}
	// 只有 1 个候选，第一次错误后即无下一个，不再 Load
	waitFor(t, func() bool { return len(inner.loadedURLs()) >= 1 })
	if len(inner.loadedURLs()) != 1 {
		t.Fatalf("loaded = %v，期望仅 1 次（无备份可切）", inner.loadedURLs())
	}
}

func TestFailoverNewLoadResetsSession(t *testing.T) {
	inner := newFakePlayer()
	fp := New(inner, nil)
	fp.Load(context.Background(), player.Stream{URL: "http/a", Backups: []string{"http/a2"}})
	fp.Load(context.Background(), player.Stream{URL: "http/b"}) // 新会话应取消旧监听
	inner.emit(player.EventError)
	waitFor(t, func() bool { return len(inner.loadedURLs()) == 2 }) // b 已 load
	// 旧会话的 a2 不应被加载；只可能加载 b 一次（无备份）
	if len(inner.loadedURLs()) != 2 || inner.loadedURLs()[1] != "http/b" {
		t.Fatalf("loaded = %v，期望 [a b]，无串味", inner.loadedURLs())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("超时等待条件满足")
}
```

（`time` 需加入 import。）

- [ ] **Step 5: 实现 failover**

`internal/player/failover/failover.go`：

```go
// Package failover 在底层 Player 之上实现「失败自动切换」：监听 EOF/Error
// 事件，按候选列表（主源 + 备份源，可经 probe 测速排序）切换下一条流。
// 逻辑位于 Player 接口之上，两种 Player 实现（mpvproc/mpvlib）共用。
package failover

import (
	"context"
	"sync"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/probe"
)

// Player 包装底层 Player，实现自动切换。所有控制方法透传 inner，仅 Load
// 与 Close 额外管理切换会话。
type Player struct {
	inner  player.Player
	prober *probe.Prober

	mu     sync.Mutex
	cancel context.CancelFunc // 当前切换会话的取消；新 Load 取消旧会话
}

// New 返回自动切换包装器。prober 为 nil 时按原始顺序切换（不测速）。
func New(inner player.Player, prober *probe.Prober) player.Player {
	return &Player{inner: inner, prober: prober}
}

func (p *Player) Load(ctx context.Context, s player.Stream) error {
	candidates := append([]string{s.URL}, s.Backups...)
	if len(candidates) > 1 && p.prober != nil {
		candidates = p.prober.Rank(ctx, candidates, s.Headers)
	}
	// 建立新会话，取消旧监听。
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	sessCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.mu.Unlock()

	if err := p.inner.Load(ctx, streamWith(s, candidates[0])); err != nil {
		return err
	}
	go p.watch(sessCtx, s, candidates, 0)
	return nil
}

// watch 监听 inner 事件，遇 EOF/Error 依次切换下一候选。gen 由会话 ctx 表示：
// 会话取消即退出，避免旧监听串扰新会话。
func (p *Player) watch(ctx context.Context, s player.Stream, candidates []string, idx int) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-p.inner.Events():
			if !ok {
				return
			}
			if ev.Kind != player.EventEOF && ev.Kind != player.EventError {
				continue
			}
			idx++
			if idx >= len(candidates) {
				return // 候选耗尽，放弃
			}
			if err := p.inner.Load(context.Background(), streamWith(s, candidates[idx])); err != nil {
				// 切换失败不终止，继续尝试下一条（由下一次事件驱动）。
				continue
			}
		}
	}
}

func streamWith(s player.Stream, url string) player.Stream {
	s.URL = url
	return s
}

func (p *Player) Play() error  { return p.inner.Play() }
func (p *Player) Pause() error { return p.inner.Pause() }
func (p *Player) Seek(sec float64) error { return p.inner.Seek(sec) }
func (p *Player) SetVolume(v int) error   { return p.inner.SetVolume(v) }
func (p *Player) SelectTrack(k player.TrackKind, id int) error {
	return p.inner.SelectTrack(k, id)
}
func (p *Player) State() player.State           { return p.inner.State() }
func (p *Player) Events() <-chan player.Event   { return p.inner.Events() }
func (p *Player) Close() error {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.mu.Unlock()
	return p.inner.Close()
}
```

**已知限制（写入注释或报告）**：inner 的 Events() 通道跨会话共享，若旧会话恰在取消窗口内已把一条 EOF 读入 watcher，极端时序下可能造成一次多余的备份切换；后果轻微（切换本就是本层职责），Plan 后续再以事件代际彻底消除。

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/player/... -count=1 -v`
Expected: PASS（mpvproc 既有 + failover 新增全绿）；`go test -race ./internal/player/failover/ -count=1` 无 DATA RACE。

- [ ] **Step 7: Commit**

```bash
git add internal/player/mpvproc/mpvproc.go internal/player/failover/
git commit -m "feat(failover): 终端事件不丢弃 + 失败自动切换包装器"
```

---

### Task 5: Store 持久化（SQLite）

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`
- Modify: `go.mod` / `go.sum`（`go get modernc.org/sqlite`）

**Interfaces:**
- Consumes: 无。
- Produces: `store.Open`/`Store` 及收藏/最近/分组 CRUD。

- [ ] **Step 1: 引入依赖并钉版本**

```bash
go get modernc.org/sqlite@latest
```
记录 go.mod 中解析出的具体版本（如 `v1.x.y`），确认 `go mod tidy` 后无 cgo 依赖（`modernc.org/sqlite` 是纯 Go）。若解析出的是预发布/占位版本，改钉一个稳定 tag。

- [ ] **Step 2: 写失败测试**

`store_test.go`：

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestFavoriteCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	if err := s.AddFavorite("央视/CCTV-1", "CCTV-1", "央视", "http://x/1"); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	ok, err := s.IsFavorite("央视/CCTV-1")
	if err != nil || !ok {
		t.Fatalf("IsFavorite = %v, %v", ok, err)
	}
	favs, err := s.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
	if err := s.RemoveFavorite("央视/CCTV-1"); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	if favs, _ = s.ListFavorites(); len(favs) != 0 {
		t.Fatalf("删除后仍有 %d 条", len(favs))
	}
}

func TestRecentKeepsLatestFirst(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddRecent("a", "组", "http://a")
	s.AddRecent("b", "组", "http://b")
	rs, err := s.ListRecent(1)
	if err != nil || len(rs) != 1 || rs[0].Name != "b" {
		t.Fatalf("ListRecent(1) = %+v, %v", rs, err)
	}
}

func TestGroupCRUD(t *testing.T) {
	s := openTemp(t)
	defer s.Close()
	s.AddGroup("我的分组")
	s.AddGroup("我的分组") // 幂等
	gs, err := s.ListGroups()
	if err != nil || len(gs) != 1 {
		t.Fatalf("ListGroups = %+v, %v", gs, err)
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/store/ -v`
Expected: FAIL（`undefined: Open` 等）

- [ ] **Step 4: 实现**

`internal/store/store.go`：

```go
// Package store 提供收藏、最近观看、自定义分组的 SQLite 持久化。
// 使用 modernc.org/sqlite（纯 Go 无 cgo），满足「安装即用」无外部依赖约束。
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Favorite 是一条收藏。
type Favorite struct {
	ID      string // 频道稳定标识（group/name）
	Name    string
	Group   string
	URL     string
	Logo    string
	AddedAt int64
}

// Recent 是一条最近观看记录。
type Recent struct {
	ID        string
	Name      string
	Group     string
	URL       string
	WatchedAt int64
}

// Group 是一个自定义分组。
type Group struct {
	ID    int64
	Name  string
	Order int64
}

// Store 封装 SQLite 连接。
type Store struct{ db *sql.DB }

// Open 打开（或创建）数据库并跑迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// migrate 用 CREATE TABLE IF NOT EXISTS 建表（M1 无需版本化迁移）。
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS favorites (id TEXT PRIMARY KEY, name TEXT NOT NULL, grp TEXT, url TEXT, logo TEXT, added_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS recent (id TEXT PRIMARY KEY, name TEXT NOT NULL, grp TEXT, url TEXT, watched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS grp (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL, ord INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

func (s *Store) AddFavorite(id, name, group, url string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO favorites(id,name,grp,url,added_at) VALUES(?,?,?,?,?)`,
		id, name, group, url, time.Now().Unix())
	return err
}

func (s *Store) RemoveFavorite(id string) error {
	_, err := s.db.Exec(`DELETE FROM favorites WHERE id=?`, id)
	return err
}

func (s *Store) IsFavorite(id string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM favorites WHERE id=?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListFavorites() ([]Favorite, error) {
	rows, err := s.db.Query(`SELECT id,name,grp,url,added_at FROM favorites ORDER BY added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Favorite
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.Name, &f.Group, &f.URL, &f.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) AddRecent(id, name, group, url string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO recent(id,name,grp,url,watched_at) VALUES(?,?,?,?,?)`,
		id, name, group, url, time.Now().Unix())
	return err
}

func (s *Store) ListRecent(limit int) ([]Recent, error) {
	rows, err := s.db.Query(`SELECT id,name,grp,url,watched_at FROM recent ORDER BY watched_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recent
	for rows.Next() {
		var r Recent
		if err := rows.Scan(&r.ID, &r.Name, &r.Group, &r.URL, &r.WatchedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AddGroup(name string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO grp(name,ord) VALUES(?,0)`, name)
	return err
}

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id,name,ord FROM grp ORDER BY ord,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Order); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) RemoveGroup(name string) error {
	_, err := s.db.Exec(`DELETE FROM grp WHERE name=?`, name)
	return err
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/store/ -count=1 -v`
Expected: PASS（3 测试函数）

- [ ] **Step 6: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat(store): SQLite 持久化（收藏/最近/分组）"
```

---

### Task 6: Shell 接线（订阅导入 + 目录 + 播放 + 收藏）

**Files:**
- Create: `internal/shell/service.go`
- Modify: `internal/shell/app.go`（ShellService 加字段与构造）
- Modify: `cmd/unbox/main.go`（注入依赖）
- Test: `internal/shell/service_test.go`

**Interfaces:**
- Consumes: `provider.Provider`、`live.Provider`、`player.Player`（已 failover 包装）、`probe.Prober`、`store.Store`、`config.Fetcher`/`config.Resolver`。
- Produces（Wails 暴露给前端，方法名即前端绑定）：
  ```go
  type ImportResult struct { Groups int; Channels int }
  type ChannelInfo struct { ID, Name, Group, Logo string; Favorited bool }
  func (s *ShellService) ImportSubscription(url string) (ImportResult, error)
  func (s *ShellService) Groups() ([]string, error)
  func (s *ShellService) Channels(group string, page int) ([]ChannelInfo, error)
  func (s *ShellService) Search(q string) ([]ChannelInfo, error)
  func (s *ShellService) PlayChannel(id string) error
  func (s *ShellService) Pause() error
  func (s *ShellService) Resume() error
  func (s *ShellService) SetVolume(v int) error
  func (s *ShellService) AddFavorite(id string) error
  func (s *ShellService) RemoveFavorite(id string) error
  func (s *ShellService) ListFavorites() ([]ChannelInfo, error)
  func (s *ShellService) AddGroup(name string) error
  func (s *ShellService) ListGroups() ([]string, error)
  ```

- [ ] **Step 1: 写失败测试**

`service_test.go`：

```go
package shell

import (
	"os"
	"testing"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/store"
)

func newTestService(t *testing.T) *ShellService {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	channels := []config.Channel{
		{Name: "CCTV-1", Group: "央视", URLs: []string{"http://x/1"}},
	}
	svc := NewShellService(live.New(channels), nil, s)
	t.Cleanup(func() { s.Close() })
	return svc
}

func TestImportSubscriptionPlaylist(t *testing.T) {
	svc := newTestService(t)
	path := t.TempDir() + "/ch.m3u"
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:-1 group-title=\"测试\",频道A\nhttp://x/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, err := svc.ImportSubscription(path)
	if err != nil || r.Channels != 1 {
		t.Fatalf("ImportSubscription = %+v, %v", r, err)
	}
	gs, _ := svc.Groups()
	if len(gs) != 1 || gs[0] != "测试" {
		t.Fatalf("Groups = %v", gs)
	}
}

func TestGroupsAndChannels(t *testing.T) {
	svc := newTestService(t)
	gs, err := svc.Groups()
	if err != nil || len(gs) != 1 || gs[0] != "央视" {
		t.Fatalf("Groups = %v, %v", gs, err)
	}
	chs, err := svc.Channels("央视", 0)
	if err != nil || len(chs) != 1 || chs[0].Name != "CCTV-1" {
		t.Fatalf("Channels = %+v, %v", chs, err)
	}
}

func TestFavoriteRoundtrip(t *testing.T) {
	svc := newTestService(t)
	id := "央视/CCTV-1"
	if err := svc.AddFavorite(id); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	favs, err := svc.ListFavorites()
	if err != nil || len(favs) != 1 || favs[0].Name != "CCTV-1" {
		t.Fatalf("ListFavorites = %+v, %v", favs, err)
	}
}

func TestPlayChannelRequiresPlayer(t *testing.T) {
	svc := newTestService(t)
	if err := svc.PlayChannel("央视/CCTV-1"); err == nil {
		t.Fatal("player 为 nil 时 PlayChannel 应报错")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/shell/ -run 'TestGroups|TestFavorite|TestPlayChannel' -v`
Expected: FAIL（`undefined: NewShellService` 等）

- [ ] **Step 3: 实现 ShellService**

`internal/shell/service.go`：

```go
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/store"
)

// ImportResult 是导入订阅的摘要。
type ImportResult struct {
	Groups   int
	Channels int
}

// ChannelInfo 是前端展示用的频道信息。
type ChannelInfo struct {
	ID        string
	Name      string
	Group     string
	Logo      string
	Favorited bool
}

// NewShellService 组装壳层服务。p 可为 nil（播放器未就绪）。
func NewShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	return &ShellService{provider: pv, player: p, store: st}
}

// ImportSubscription 拉取并解析订阅，重建 Provider。支持两类输入：
//   - TVBox 订阅配置（JSON，含 lives/storeHouse/urls，可能多仓）
//   - 独立 M3U/TXT 播放列表（#EXTM3U 或「名称,URL」行）
func (s *ShellService) ImportSubscription(ref string) (ImportResult, error) {
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if isPlaylist(raw) {
		return s.importPlaylist(raw)
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		return ImportResult{}, err
	}
	channels := collectChannels(context.Background(), cfgs)
	if len(channels) == 0 {
		return ImportResult{}, errors.New("订阅中没有可用直播频道")
	}
	s.mu.Lock()
	s.provider = live.New(channels)
	s.mu.Unlock()
	secs, _ := s.provider.Home(context.Background())
	return ImportResult{Groups: len(secs), Channels: len(channels)}, nil
}

// isPlaylist 探测内容是否为独立播放列表而非 JSON 配置。
func isPlaylist(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return false
	}
	if bytes.HasPrefix(t, []byte("#EXTM3U")) {
		return true
	}
	// JSON 配置以 { 或 [ 开头；其余（如「名称,URL」行）按 TXT 播放列表处理。
	return t[0] != '{' && t[0] != '['
}

// importPlaylist 解析独立 M3U/TXT 播放列表并重建 Provider。
func (s *ShellService) importPlaylist(raw []byte) (ImportResult, error) {
	entries := live.ParseM3U(raw)
	if len(entries) == 0 {
		entries = live.ParseTXT(raw) // m3u 解析为空则按 TXT 回退
	}
	channels := live.Assemble(entries)
	if len(channels) == 0 {
		return ImportResult{}, errors.New("播放列表中没有可用频道")
	}
	s.mu.Lock()
	s.provider = live.New(channels)
	s.mu.Unlock()
	secs, _ := s.provider.Home(context.Background())
	return ImportResult{Groups: len(secs), Channels: len(channels)}, nil
}

// resolveConfigs 解析 TVBox 配置；索引节点（storeHouse/urls 非空）交给 Resolver 展开。
func resolveConfigs(ctx context.Context, ref string, raw []byte) ([]*config.Config, error) {
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0 {
		cfgs, rerr := config.NewResolver().Resolve(ctx, ref)
		if rerr != nil && len(cfgs) == 0 {
			return nil, fmt.Errorf("展开订阅失败: %w", rerr)
		}
		return cfgs, nil
	}
	return []*config.Config{cfg}, nil
}

// collectChannels 从多份配置收集直播频道：Channels 内嵌的直接用，URL 指向的拉取解析。
func collectChannels(ctx context.Context, cfgs []*config.Config) []config.Channel {
	var channels []config.Channel
	fetcher := config.NewFetcher()
	for _, cfg := range cfgs {
		for _, lv := range cfg.Lives {
			if len(lv.Channels) > 0 {
				channels = append(channels, lv.Channels...)
				continue
			}
			if lv.URL == "" {
				continue
			}
			chs, ferr := live.FetchLive(ctx, lv, fetcher)
			if ferr == nil {
				channels = append(channels, chs...)
			}
		}
	}
	return channels
}

func (s *ShellService) Groups() ([]string, error) {
	secs, err := s.provider.Home(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]string, len(secs))
	for i, sec := range secs {
		out[i] = sec.Title
	}
	return out, nil
}

func (s *ShellService) Channels(group string, page int) ([]ChannelInfo, error) {
	pg, err := s.provider.Browse(context.Background(), group, page)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(pg.Items), nil
}

func (s *ShellService) Search(q string) ([]ChannelInfo, error) {
	items, err := s.provider.Search(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(items), nil
}

func (s *ShellService) toChannelInfo(items []provider.Item) []ChannelInfo {
	out := make([]ChannelInfo, len(items))
	for i, it := range items {
		fav, _ := s.store.IsFavorite(it.ID)
		out[i] = ChannelInfo{ID: it.ID, Name: it.Title, Group: it.Group, Logo: it.Logo, Favorited: fav}
	}
	return out
}

func (s *ShellService) PlayChannel(id string) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	st, err := s.provider.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.player.Load(context.Background(), st); err != nil {
		return err
	}
	if err := s.store.AddRecent(id, stTitle(s.provider, id), stGroup(s.provider, id), st.URL); err != nil {
		return err
	}
	return s.player.Play()
}

// stTitle/stGroup 从 provider 取频道名/分组用于最近记录（简化：直接解析 id）。
func stTitle(p provider.Provider, id string) string { m, err := p.Detail(context.Background(), id); if err == nil { return m.Title }; return id }
func stGroup(p provider.Provider, id string) string { m, err := p.Detail(context.Background(), id); if err == nil { return m.Group }; return "" }

func (s *ShellService) Pause() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Pause()
}

func (s *ShellService) Resume() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Play()
}

func (s *ShellService) SetVolume(v int) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.SetVolume(v)
}

func (s *ShellService) AddFavorite(id string) error {
	m, err := s.provider.Detail(context.Background(), id)
	if err != nil {
		return err
	}
	st, err := s.provider.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	return s.store.AddFavorite(id, m.Title, m.Group, st.URL)
}

func (s *ShellService) RemoveFavorite(id string) error { return s.store.RemoveFavorite(id) }

func (s *ShellService) ListFavorites() ([]ChannelInfo, error) {
	favs, err := s.store.ListFavorites()
	if err != nil {
		return nil, err
	}
	out := make([]ChannelInfo, len(favs))
	for i, f := range favs {
		out[i] = ChannelInfo{ID: f.ID, Name: f.Name, Group: f.Group, Favorited: true}
	}
	return out, nil
}

func (s *ShellService) AddGroup(name string) error   { return s.store.AddGroup(name) }
func (s *ShellService) ListGroups() ([]string, error) {
	gs, err := s.store.ListGroups()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Name
	}
	return out, nil
}
```

**注意**：`ShellService` 结构体需新增字段（见 Step 4）。`service.go` 中的 `PlayerReady()`、`Platform()`、`LoadTestStream()` 等既有方法仍保留在 `app.go`。

- [ ] **Step 4: 改 app.go 与 main.go**

`app.go` 中 `ShellService` 结构体扩展为：

```go
type ShellService struct {
	player   player.Player
	provider provider.Provider
	store    *store.Store
	mu       sync.Mutex // 守护 provider 重赋值
}
```

`app.go` 顶部 import 增加 `provider`/`store`/`sync`。`NewApp` 签名扩展。**改为**：

```go
func NewApp(p player.Player, pv provider.Provider, st *store.Store) *application.App {
	return application.New(application.Options{
		...
		Services: []application.Service{
			application.NewService(NewShellService(pv, p, st)),
		},
		...
	})
}
```

`cmd/unbox/main.go` 改为：

```go
func main() {
	p, err := shell.PickPlayer()
	if err != nil {
		log.Printf("播放器初始化失败（继续以未就绪状态启动）: %v", err)
	}
	pl := p // player.Player，可能 nil
	if p != nil {
		pl = failover.New(p, probe.NewProber())
	}
	st, serr := store.Open(appDataPath())
	if serr != nil {
		log.Printf("数据库初始化失败（收藏/最近不可用）: %v", serr)
	}
	// 初始 Provider 为空：等待前端 ImportSubscription 后重建。
	var pv provider.Provider
	app := shell.NewApp(pl, pv, st)
	shell.OpenWindow(app)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// appDataPath 返回数据库存放路径（用户配置目录下的 unbox/unbox.db）。
func appDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	p := filepath.Join(dir, "unbox")
	_ = os.MkdirAll(p, 0o755)
	return filepath.Join(p, "unbox.db")
}
```

> main.go import 增加：`os`、`path/filepath`、`github.com/unbox/unbox/internal/provider`、`.../player/failover`、`.../probe`、`.../store`。

- [ ] **Step 5: 运行确认通过**

Run: `go build ./... && go test ./internal/shell/ -count=1 -v && go vet ./...`
Expected: build/测试/vet 全绿。

- [ ] **Step 6: Commit**

```bash
git add internal/shell/ cmd/unbox/main.go
git commit -m "feat(shell): 订阅导入/目录/播放/收藏接线"
```

---

### Task 7: 前端（频道列表 + 播放 + 收藏）

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`（新增频道列表/播放/收藏样式）

**Interfaces:**
- Consumes: Task 6 的 Wails 自动绑定（`wails3 generate bindings` 在 build 时自动跑，绑定路径 `../bindings/github.com/unbox/unbox/internal/shell`）。
- **注意**：Wails v3 beta 对 Go 结构体字段生成的 TS 命名可能是 camelCase（如 `ImportResult` → `importResult`，字段 `Groups` → `groups`）；实现者须以 build 时实际生成的绑定为准调整 TS 侧字段大小写。
- Produces: 无（终端 UI）。

- [ ] **Step 1: 重写 App.vue**

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ShellService } from '../bindings/github.com/unbox/unbox/internal/shell'

interface ChannelInfo { ID: string; Name: string; Group: string; Logo: string; Favorited: boolean }

const platform = ref('…')
const playerReady = ref(false)
const groups = ref<string[]>([])
const channels = ref<ChannelInfo[]>([])
const favorites = ref<ChannelInfo[]>([])
const activeGroup = ref('*')
const query = ref('')
const nowPlaying = ref('')
const importUrl = ref('')
const importSummary = ref('')
const errMsg = ref('')
const loading = ref(false)

async function refresh() {
  try {
    platform.value = await ShellService.Platform()
    playerReady.value = await ShellService.PlayerReady()
    await reloadGroups()
  } catch (e) { errMsg.value = String(e) }
}

async function reloadGroups() {
  groups.value = ['*', ...(await ShellService.Groups())]
  await reloadChannels()
}

async function reloadChannels() {
  channels.value = await ShellService.Channels(activeGroup.value === '*' ? '' : activeGroup.value, 0)
}

async function doSearch() {
  if (!query.value) { await reloadChannels(); return }
  channels.value = await ShellService.Search(query.value)
}

async function doImport() {
  loading.value = true; errMsg.value = ''; importSummary.value = ''
  try {
    const r = await ShellService.ImportSubscription(importUrl.value)
    importSummary.value = `导入成功：${r.Groups} 组 / ${r.Channels} 频道`
    await reloadGroups()
  } catch (e) { errMsg.value = String(e) }
  finally { loading.value = false }
}

async function play(c: ChannelInfo) {
  errMsg.value = ''
  try {
    await ShellService.PlayChannel(c.ID)
    nowPlaying.value = c.Name
  } catch (e) { errMsg.value = String(e) }
}

async function toggleFav(c: ChannelInfo) {
  try {
    if (c.Favorited) await ShellService.RemoveFavorite(c.ID)
    else await ShellService.AddFavorite(c.ID)
    c.Favorited = !c.Favorited
    favorites.value = await ShellService.ListFavorites()
  } catch (e) { errMsg.value = String(e) }
}

async function loadFavorites() { favorites.value = await ShellService.ListFavorites() }

async function pause() { await ShellService.Pause() }
async function resume() { await ShellService.Resume() }
async function setVolume(e: Event) { await ShellService.SetVolume(Number((e.target as HTMLInputElement).value)) }

onMounted(refresh)
</script>

<template>
  <main class="container">
    <header>
      <h1 class="title">Unbox</h1>
      <p class="subtitle">{{ platform }} · 播放器{{ playerReady ? '就绪' : '未就绪' }}</p>
    </header>

    <section class="import">
      <input v-model="importUrl" placeholder="粘贴订阅链接或本地路径" />
      <button :disabled="loading" @click="doImport">{{ loading ? '导入中…' : '导入订阅' }}</button>
      <span v-if="importSummary" class="ok">{{ importSummary }}</span>
    </section>

    <section class="layout">
      <aside class="groups">
        <button v-for="g in groups" :key="g" :class="{ active: g === activeGroup }" @click="activeGroup = g; reloadChannels()">
          {{ g === '*' ? '全部' : g }}
        </button>
        <hr />
        <button @click="loadFavorites">⭐ 收藏</button>
      </aside>

      <section class="channels">
        <div class="search"><input v-model="query" placeholder="搜索频道" @input="doSearch" /></div>
        <ul>
          <li v-for="c in channels" :key="c.ID" class="channel">
            <span class="name">{{ c.Name }}</span>
            <span class="group">{{ c.Group }}</span>
            <button @click="play(c)">▶ 播放</button>
            <button @click="toggleFav(c)">{{ c.Favorited ? '★' : '☆' }}</button>
          </li>
        </ul>
      </section>

      <aside class="player">
        <p v-if="nowPlaying" class="now">正在播放：{{ nowPlaying }}</p>
        <div class="controls" v-if="nowPlaying">
          <button @click="pause">暂停</button>
          <button @click="resume">继续</button>
          <input type="range" min="0" max="100" @input="setVolume" />
        </div>
        <p v-if="favorites.length" class="favhead">收藏</p>
        <ul class="favs">
          <li v-for="f in favorites" :key="f.ID" @click="play(f)">{{ f.Name }}</li>
        </ul>
      </aside>
    </section>

    <p v-if="errMsg" class="error">{{ errMsg }}</p>
  </main>
</template>
```

- [ ] **Step 2: 补样式**

`style.css` 追加 `.import`/`.layout`/`.groups`/`.channels`/`.channel`/`.player`/`.favs` 等选择器（三栏布局：左分组、中频道、右播放+收藏），沿用现有深色主题（背景 `#06070f` 系，参考现有 `.container` 配色）。实现者自行补全合理样式，不引入 Tailwind。

- [ ] **Step 3: 构建确认**

Run: `mise run build:linux`（或 `mise x -- wails3 build GOOS=linux GOARCH=amd64`）
Expected: 产出 `bin/unbox`，无编译错误；`go test ./... -count=1` 全绿（绑定生成不破坏 Go 侧）。

- [ ] **Step 4: 冒烟（人工，环境允许时）**

Run: `./bin/unbox`（WSLg 下 DISPLAY=:0 可用）
Expected: 窗口打开；粘贴 `file://testdata/...` 或本地 m3u 路径导入后频道列表展示；点击频道 mpv 独立窗口出画面；收藏/最近可持久化。

- [ ] **Step 5: Commit**

```bash
git add frontend/
git commit -m "feat(frontend): 频道列表/播放控制/收藏 UI"
```

---

## 验收对照（spec §7）

- [x] `unbox` 编译通过（Task 6/7）
- [x] `unbox-scan` 编译通过（既有，未破坏）
- [x] 7 个真实样本解析通过（Plan 1 既有，未破坏）
- [x] 订阅导入 → 频道列表 → 点击出画面（Task 2/6/7；画面经 mpv 独立窗口，见下）
- [x] 主流失效自动切换备用流（Task 4）
- [x] 收藏/最近/分组持久化（Task 5/6）
- [ ] 三平台画面嵌入主窗口内 —— **仍顺延**（Windows named-pipe + `--wid`、macOS mpvlib 需真实平台机器，spec §3.4）
- [ ] 三平台安装包产出 —— **仍顺延**（打包需 CI/各平台构建机）
- [x] `unbox-scan` 输出兼容性报告（既有）

## 停车项（转入后续）

- Windows named-pipe + `--wid` 窗口嵌入；macOS libmpv + CAMetalLayer（spec §3.4，需真实机器）。
- mpvproc `State()` 事件迁移（Playing/Paused/Buffering/Stopped 随事件推进）及超时/重入 state 竞序（Plan 2 re-review 已记）。
- failover 跨会话事件代际（消除极窄窗口下的一次多余切换）。
- 打包元数据占位（`build/config.yml` "My Company"）。
- 独立 M3U/TXT 文件导入入口（当前 `ImportSubscription` 走订阅 URL；裸 m3u 文件可经 `FetchLive` 支持，UI 入口与订阅统一为「粘贴链接/路径」）。
