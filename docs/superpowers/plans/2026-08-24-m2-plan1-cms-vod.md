# Unbox M2 实现计划：TVBox 点播（CMS JSON）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 导入订阅后，在「点播」页按 站点 → 分类 → 影片列表 → 详情（剧集）→ 播放 全链路观看 CMS JSON（type=1）站点的影片。

**Architecture:** 每个 CMS 站点一个 `tvbox.Provider` 实例；`provider` 接口增量扩展 `Media`/新增 `Episode`；壳层从「单 provider」改为「直播 + 多站点」；前端顶层加「直播/点播」切换。

**Tech Stack:** Go 1.26.3，`net/http` + `encoding/json`（无第三方依赖）；复用 `config` 包的 `Site` 模型。

**Spec:** `docs/superpowers/specs/2026-08-24-unbox-m2-design.md`

## Global Constraints

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/provider/tvbox/` 不 import Wails。
- `internal/provider/tvbox/` 只依赖 `provider`、`player`、`config` 三个包。
- 公开错误信息 / 注释用中文。
- TDD：先写失败测试再实现。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；Linux 额外 `CGO_ENABLED=1 go build ./...`。
- 真实 fixture 已入库 `testdata/cms/`（非凡资源：`list.json` / `detail.json` / `search.json`），测试不得访问外网。

---

## Task 1: provider 类型扩展 + 剧集拆分纯函数

**Files:**
- Modify: `internal/provider/provider.go`
- Create: `internal/provider/tvbox/episodes.go`
- Test: `internal/provider/tvbox/episodes_test.go`

**Interfaces:**
- Produces: `provider.Episode`、`provider.Media` 新增字段；`tvbox.splitSources`、`tvbox.parseEpisodes`（后续 Task 3 的 `Detail` 调用）。

- [ ] **Step 1: 扩展 provider 类型（写失败测试）**

在 `internal/provider/provider.go` 的 `Media` 结构体后追加字段，并在同文件新增 `Episode`：

```go
// Episode 是点播的一集。ID 为稳定标识（供 Resolve 定位）。
type Episode struct {
	ID     string // 稳定标识
	Source string // 线路名
	Name   string // 第 N 集
	URL    string // 播放地址
}
```

`Media` 在现有 `ID/Title/Logo/Group` 之后追加：

```go
	Description string     // 简介（vod_content）
	Year        string     // vod_year
	Area        string     // vod_area
	Type        string     // type_name
	Remarks     string     // vod_remarks
	Sources     []string   // 线路名列表
	Episodes    []Episode  // 剧集列表
```

此改动纯增量，现有 `live` 测试无需改动（新字段零值）。运行 `go test ./internal/provider/...` 应绿——这一步不引入新失败测试（类型无行为），但作为 Task 3/4 的前置，先跑绿确认无破坏。

- [ ] **Step 2: 写 episodes 失败测试**

`internal/provider/tvbox/episodes_test.go`：

```go
package tvbox

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/unbox/unbox/internal/provider"
)

func TestSplitSources(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"feifan$$$ffm3u8", []string{"feifan", "ffm3u8"}},
		{"feifan,ffm3u8", []string{"feifan", "ffm3u8"}},
		{"single", []string{"single"}},
		{"", nil},
		{"a$$$b$$$c", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitSources(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitSources(%q) = %v, 期望 %v", c.in, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("splitSources(%q)[%d] = %q, 期望 %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// detailJSON 从真实 fixture 读详情，返回 vod_play_from / vod_play_url。
func loadDetailFixture(t *testing.T) (from, playURL string) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/cms/detail.json")
	if err != nil {
		t.Fatalf("读 fixture 失败: %v", err)
	}
	var resp struct {
		List []struct {
			VodPlayFrom string `json:"vod_play_from"`
			VodPlayURL  string `json:"vod_play_url"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("解析 fixture 失败: %v", err)
	}
	return resp.List[0].VodPlayFrom, resp.List[0].VodPlayURL
}

func TestParseEpisodesRealFixture(t *testing.T) {
	from, playURL := loadDetailFixture(t)
	sources := splitSources(from) // 期望 ["feifan","ffm3u8"]
	if len(sources) != 2 {
		t.Fatalf("fixture 线路数 = %d, 期望 2", len(sources))
	}
	eps := parseEpisodes("98823", playURL, sources)
	if len(eps) != 14 {
		t.Fatalf("剧集数 = %d, 期望 14（2 线路 × 7 集）", len(eps))
	}
	if eps[0].ID != "98823/0/0" || eps[0].Source != "feifan" || eps[0].Name != "第01集" || eps[0].URL == "" {
		t.Fatalf("首集解析错误: %+v", eps[0])
	}
	if eps[7].ID != "98823/1/0" || eps[7].Source != "ffm3u8" {
		t.Fatalf("第二线路首集解析错误: %+v", eps[7])
	}
	if eps[13].ID != "98823/1/6" || eps[13].Source != "ffm3u8" {
		t.Fatalf("末集解析错误: %+v", eps[13])
	}
}

func TestParseEpisodesEdgeCases(t *testing.T) {
	// 两集一路线
	eps := parseEpisodes("1", "第01集$a#第02集$b", []string{"x"})
	if len(eps) != 2 || eps[0].Name != "第01集" || eps[1].URL != "b" {
		t.Fatalf("两集解析错误: %+v", eps)
	}
	// 空 playURL
	if n := len(parseEpisodes("1", "", nil)); n != 0 {
		t.Fatalf("空 playURL 应得 0 集, 实得 %d", n)
	}
	// 无名字的裸地址
	eps = parseEpisodes("1", "https://x/a.m3u8", nil)
	if len(eps) != 1 || eps[0].Name != "" || eps[0].URL != "https://x/a.m3u8" {
		t.Fatalf("裸地址解析错误: %+v", eps)
	}
}
```

注意：测试路径 `../../testdata/cms/` 相对 `internal/provider/tvbox/` 包目录（Go test 的 cwd 是包目录）。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/provider/tvbox/ -run 'TestSplitSources|TestParseEpisodes' -count=1`
Expected: FAIL（`undefined: splitSources` 等）。

- [ ] **Step 4: 实现 episodes.go**

```go
package tvbox

import (
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/provider"
)

// sourceSep 是详情接口里线路名 / 线路播放段的统一分隔符（实测确认）。
const sourceSep = "$$$"

// splitSources 拆分线路名（vod_play_from）。详情接口用 $$$，列表接口用 ,，二者兼容。
func splitSources(from string) []string {
	from = strings.TrimSpace(from)
	if from == "" {
		return nil
	}
	if strings.Contains(from, sourceSep) {
		return splitNonEmpty(from, sourceSep)
	}
	if strings.Contains(from, ",") {
		return splitNonEmpty(from, ",")
	}
	return []string{from}
}

// parseEpisodes 把 vod_play_url 拆成剧集列表。
// playURL：线路间 $$$、集间 #、集名与地址间第一个 $。sources 为对应线路名。
// 每条剧集的 ID 编码为 "<vodID>/<线路下标>/<集下标>"，供 Resolve 反查。
func parseEpisodes(vodID, playURL string, sources []string) []provider.Episode {
	var out []provider.Episode
	for si, seg := range strings.Split(playURL, sourceSep) {
		src := ""
		if si < len(sources) {
			src = sources[si]
		}
		for ei, ep := range strings.Split(seg, "#") {
			name, url := splitEpisode(ep)
			if url == "" {
				continue
			}
			out = append(out, provider.Episode{
				ID:     fmt.Sprintf("%s/%d/%d", vodID, si, ei),
				Source: src,
				Name:   name,
				URL:    url,
			})
		}
	}
	return out
}

// splitEpisode 拆 "集名$地址"；无 $ 时整串视为地址。
func splitEpisode(ep string) (name, url string) {
	if i := strings.Index(ep, "$"); i >= 0 {
		return ep[:i], ep[i+1:]
	}
	return "", ep
}

// splitNonEmpty 按 sep 拆分并去掉空段。
func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/provider/tvbox/ -run 'TestSplitSources|TestParseEpisodes' -count=1`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/provider/provider.go internal/provider/tvbox/episodes.go internal/provider/tvbox/episodes_test.go
git commit -m "feat(tvbox): provider.Episode 类型 + 剧集拆分纯函数（TDD）"
```

---

## Task 2: CMS JSON 客户端

**Files:**
- Create: `internal/provider/tvbox/cms.go`
- Test: `internal/provider/tvbox/cms_test.go`

**Interfaces:**
- Consumes: `config.Site`（API 字段）。
- Produces: `client.videolist`、`client.detail`（Task 3 的 Provider 调用）。

- [ ] **Step 1: 写失败测试（httptest 打桩）**

`internal/provider/tvbox/cms_test.go`：

```go
package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientVideolist(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Query().Get("ac") != "videolist" {
			t.Errorf("ac = %q, 期望 videolist", r.URL.Query().Get("ac"))
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":98823,"vod_name":"狂怒追缉","vod_pic":"http://x/p.jpg","type_id":16,"type_name":"欧美剧"}]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	items, err := c.videolist(context.Background(), "16", "", 0)
	if err != nil {
		t.Fatalf("videolist 失败: %v", err)
	}
	if len(items) != 1 || items[0].VodName != "狂怒追缉" || items[0].TypeName != "欧美剧" {
		t.Fatalf("解析错误: %+v", items)
	}
	if gotPath != "/" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestClientBaseStripsQuery(t *testing.T) {
	// 无水印采集站点的 api 内嵌了 ?ac=list，须剥离后再拼参数。
	c := newClient("https://api.example.com/api.php/provide/vod/?ac=list")
	if c.base != "https://api.example.com/api.php/provide/vod/" {
		t.Fatalf("base 未剥离 query: %q", c.base)
	}
}

func TestClientDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") != "detail" || r.URL.Query().Get("ids") != "98823" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":98823,"vod_name":"狂怒追缉","vod_play_from":"feifan$$$ffm3u8","vod_play_url":"第01集$a#第02集$b"}]}`))
	}))
	defer srv.Close()

	v, err := newClient(srv.URL).detail(context.Background(), "98823")
	if err != nil {
		t.Fatalf("detail 失败: %v", err)
	}
	if v.VodPlayFrom != "feifan$$$ffm3u8" {
		t.Fatalf("detail 解析错误: %+v", v)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/provider/tvbox/ -run TestClient -count=1`
Expected: FAIL（`undefined: newClient` 等）。

- [ ] **Step 3: 实现 cms.go**

```go
package tvbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tvboxUA 是 TVBox 客户端惯例 UA，部分 CMS 按它鉴权。
const tvboxUA = "okhttp/3.12.11"

// cmsVideo 是 CMS 接口返回的影片条目（列表与详情共用）。
type cmsVideo struct {
	VodID       int64  `json:"vod_id"`
	VodName     string `json:"vod_name"`
	VodPic      string `json:"vod_pic"`
	TypeID      int64  `json:"type_id"`
	TypeName    string `json:"type_name"`
	VodRemarks  string `json:"vod_remarks"`
	VodYear     string `json:"vod_year"`
	VodArea     string `json:"vod_area"`
	VodContent  string `json:"vod_content"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
}

type cmsResp struct {
	Code int        `json:"code"`
	List []cmsVideo `json:"list"`
}

// client 是单个 CMS 站点的 JSON API 客户端。
type client struct {
	base string
	hc   *http.Client
}

// newClient 用站点 api 基地址构造客户端；剥离 api 内可能内嵌的 query。
func newClient(api string) *client {
	base := strings.SplitN(api, "?", 2)[0]
	return &client{base: base, hc: &http.Client{Timeout: 15 * time.Second}}
}

// videolist 请求列表/分类/搜索（t、wd 为可空过滤条件）。
func (c *client) videolist(ctx context.Context, t, wd string, page int) ([]cmsVideo, error) {
	q := url.Values{"ac": {"videolist"}, "pg": {fmt.Sprintf("%d", page)}}
	if t != "" {
		q.Set("t", t)
	}
	if wd != "" {
		q.Set("wd", wd)
	}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析列表响应失败: %w", err)
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("站点返回错误码 %d", resp.Code)
	}
	return resp.List, nil
}

// detail 请求单部影片详情。
func (c *client) detail(ctx context.Context, ids string) (*cmsVideo, error) {
	q := url.Values{"ac": {"detail"}, "ids": {ids}}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析详情响应失败: %w", err)
	}
	if resp.Code != 1 || len(resp.List) == 0 {
		return nil, fmt.Errorf("站点返回错误码 %d 或无数据", resp.Code)
	}
	return &resp.List[0], nil
}

// get 发起带 UA 的 GET，返回响应体。
func (c *client) get(ctx context.Context, q url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", tvboxUA)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("站点返回 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/provider/tvbox/ -run TestClient -count=1`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/provider/tvbox/cms.go internal/provider/tvbox/cms_test.go
git commit -m "feat(tvbox): CMS JSON 客户端（httptest TDD）"
```

---

## Task 3: tvbox Provider

**Files:**
- Create: `internal/provider/tvbox/tvbox.go`
- Test: `internal/provider/tvbox/tvbox_test.go`

**Interfaces:**
- Consumes: Task 1 `splitSources`/`parseEpisodes`；Task 2 `client`。
- Produces: `tvbox.New(site) *Provider` 实现 `provider.Provider`（供 Task 4 壳层使用）。

- [ ] **Step 1: 写失败测试（httptest 整站打桩）**

`internal/provider/tvbox/tvbox_test.go`：

```go
package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

// newTestProvider 起一个打桩 CMS 站点并返回 Provider。
func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac := r.URL.Query().Get("ac")
		switch ac {
		case "videolist":
			w.Write([]byte(`{"code":1,"list":[
				{"vod_id":1,"vod_name":"电影A","type_id":10,"type_name":"电影"},
				{"vod_id":2,"vod_name":"剧集B","type_id":20,"type_name":"电视剧"}
			]}`))
		case "detail":
			w.Write([]byte(`{"code":1,"list":[{"vod_id":1,"vod_name":"电影A","vod_content":"简介","vod_year":"2026","vod_area":"大陆","type_name":"电影","vod_play_from":"x$$$y","vod_play_url":"第01集$a#第02集$b$$$第01集$c"}]}`))
		default:
			http.Error(w, "bad ac", 400)
		}
	}))
	t.Cleanup(srv.Close)
	return New(config.Site{Key: "test", Name: "测试站", Type: config.SiteTypeCMS, API: srv.URL})
}

func TestProviderHome(t *testing.T) {
	secs, err := newTestProvider(t).Home(context.Background())
	if err != nil {
		t.Fatalf("Home 失败: %v", err)
	}
	if len(secs) != 2 || secs[0].ID != "10" || secs[0].Title != "电影" || secs[1].Title != "电视剧" {
		t.Fatalf("分类派生错误: %+v", secs)
	}
}

func TestProviderDetailAndResolve(t *testing.T) {
	p := newTestProvider(t)
	m, err := p.Detail(context.Background(), "1")
	if err != nil {
		t.Fatalf("Detail 失败: %v", err)
	}
	if m.Title != "电影A" || m.Description != "简介" || len(m.Sources) != 2 || len(m.Episodes) != 3 {
		t.Fatalf("Detail 解析错误: %+v", m)
	}
	st, err := p.Resolve(context.Background(), "1/0/1")
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	if st.URL != "b" || st.Kind == 0 {
		t.Fatalf("Resolve 错误: %+v", st)
	}
	if st.Headers["Referer"] == "" {
		t.Fatalf("Resolve 应带 Referer: %+v", st.Headers)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/provider/tvbox/ -run TestProvider -count=1`
Expected: FAIL（`undefined: New`）。

- [ ] **Step 3: 实现 tvbox.go**

```go
// Package tvbox 实现 TVBox 点播站点的 Provider（首期仅 CMS JSON，type=1）。
package tvbox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Provider 是单个 CMS 站点的 Provider 实现。
type Provider struct {
	site config.Site
	c    *client

	mu    sync.Mutex
	cache map[string]provider.Episode // epID → 剧集（Detail 时填充，Resolve 反查）
}

// New 用站点定义构造 Provider。
func New(site config.Site) *Provider {
	return &Provider{
		site:  site,
		c:     newClient(site.API),
		cache: map[string]provider.Episode{},
	}
}

func (p *Provider) ID() string { return p.site.Key }

func (p *Provider) Home(ctx context.Context) ([]provider.Section, error) {
	items, err := p.c.videolist(ctx, "", "", 0)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var secs []provider.Section
	for _, v := range items {
		if v.TypeID == 0 || seen[v.TypeID] {
			continue
		}
		seen[v.TypeID] = true
		secs = append(secs, provider.Section{ID: strconv.FormatInt(v.TypeID, 10), Title: v.TypeName})
	}
	return secs, nil
}

func (p *Provider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	items, err := p.c.videolist(ctx, cat, "", page)
	if err != nil {
		return provider.Page{}, err
	}
	out := make([]provider.Item, 0, len(items))
	for _, v := range items {
		out = append(out, provider.Item{
			ID:    strconv.FormatInt(v.VodID, 10),
			Title: v.VodName,
			Logo:  v.VodPic,
			Group: v.TypeName,
		})
	}
	return provider.Page{Items: out}, nil
}

func (p *Provider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	items, err := p.c.videolist(ctx, "", q, 0)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Item, 0, len(items))
	for _, v := range items {
		out = append(out, provider.Item{
			ID:    strconv.FormatInt(v.VodID, 10),
			Title: v.VodName,
			Logo:  v.VodPic,
			Group: v.TypeName,
		})
	}
	return out, nil
}

func (p *Provider) Detail(ctx context.Context, id string) (provider.Media, error) {
	v, err := p.c.detail(ctx, id)
	if err != nil {
		return provider.Media{}, err
	}
	sources := splitSources(v.VodPlayFrom)
	eps := parseEpisodes(id, v.VodPlayURL, sources)
	p.mu.Lock()
	for _, e := range eps {
		p.cache[e.ID] = e
	}
	p.mu.Unlock()
	return provider.Media{
		ID:          id,
		Title:       v.VodName,
		Logo:        v.VodPic,
		Group:       v.TypeName,
		Description: v.VodContent,
		Year:        v.VodYear,
		Area:        v.VodArea,
		Type:        v.TypeName,
		Remarks:     v.VodRemarks,
		Sources:     sources,
		Episodes:    eps,
	}, nil
}

// Resolve 按剧集 id（"<vodID>/<线路下标>/<集下标>"）反查播放地址。
func (p *Provider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	p.mu.Lock()
	ep, ok := p.cache[epID]
	p.mu.Unlock()
	if !ok {
		vodID, _, _, err := parseEpID(epID)
		if err != nil {
			return player.Stream{}, err
		}
		if _, derr := p.Detail(ctx, vodID); derr != nil {
			return player.Stream{}, derr
		}
		p.mu.Lock()
		ep, ok = p.cache[epID]
		p.mu.Unlock()
		if !ok {
			return player.Stream{}, fmt.Errorf("剧集不存在: %s", epID)
		}
	}
	return player.Stream{
		URL:     ep.URL,
		Kind:    kindForURL(ep.URL),
		Headers: map[string]string{"Referer": originOf(p.site.API)},
	}, nil
}

// parseEpID 拆 "<vodID>/<si>/<ei>"。
func parseEpID(id string) (vodID string, si, ei int, err error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("剧集 id 非法: %s", id)
	}
	si, err1 := strconv.Atoi(parts[1])
	ei, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return "", 0, 0, fmt.Errorf("剧集 id 非法: %s", id)
	}
	return parts[0], si, ei, nil
}

// originOf 取站点 api 的 scheme://host 作 Referer（防盗链）。
func originOf(api string) string {
	u, err := url.Parse(api)
	if err != nil {
		return api
	}
	return u.Scheme + "://" + u.Host
}

// kindForURL 依据 URL 扩展名猜流形态，默认 HLS。
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

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/provider/tvbox/ -count=1`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/provider/tvbox/tvbox.go internal/provider/tvbox/tvbox_test.go
git commit -m "feat(tvbox): CMS 站点 Provider（Home/Browse/Search/Detail/Resolve）"
```

---

## Task 4: 壳层多源改造

**Files:**
- Modify: `internal/shell/app.go`（ShellService 结构体）
- Modify: `internal/shell/service.go`（ImportSubscription + 新 Vod* 方法）
- Test: `internal/shell/service_test.go`（追加）

**Interfaces:**
- Consumes: Task 3 `tvbox.New`。
- Produces: `ShellService.Sources` / `VodCategories` / `VodList` / `VodSearch` / `VodDetail` / `PlayVod`（供 Task 5 前端调用）。

- [ ] **Step 1: 改结构体（app.go）**

`ShellService` 的 `provider provider.Provider` 字段改名为 `live`，新增 `vods map[string]provider.Provider` 与 `vodNames map[string]string`：

```go
type ShellService struct {
	player   player.Player
	store    *store.Store
	live     provider.Provider              // 直播（单个）
	vods     map[string]provider.Provider   // 点播站点 key → provider
	vodNames map[string]string              // 点播站点 key → 显示名
	mu       sync.RWMutex
}
```

`NewShellService` 相应初始化：

```go
func NewShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	return &ShellService{
		live:     pv,
		player:   p,
		store:    st,
		vods:     map[string]provider.Provider{},
		vodNames: map[string]string{},
	}
}
```

- [ ] **Step 2: 把 service.go 里 `s.provider` 全部改为 `s.live`**

涉及 `ImportSubscription`、`Groups`、`Channels`、`Search`、`PlayChannel`、`AddFavorite` 中的 `s.provider` 读写。机械替换，无逻辑变化。运行 `go build ./...` 确认无遗漏。

- [ ] **Step 3: 写失败测试（stub provider）**

`internal/shell/service_test.go` 追加（复用包内已有的测试桩；若无则定义）：

```go
type stubProvider struct{ key, name string }

func (s *stubProvider) ID() string { return s.key }
func (s *stubProvider) Home(context.Context) ([]provider.Section, error) {
	return []provider.Section{{ID: "10", Title: "电影"}}, nil
}
func (s *stubProvider) Browse(context.Context, string, int) (provider.Page, error) {
	return provider.Page{Items: []provider.Item{{ID: "1", Title: "电影A"}}}, nil
}
func (s *stubProvider) Search(context.Context, string) ([]provider.Item, error) { return nil, nil }
func (s *stubProvider) Detail(context.Context, string) (provider.Media, error) {
	return provider.Media{ID: "1", Title: "电影A", Sources: []string{"x"}, Episodes: []provider.Episode{{ID: "1/0/0", Source: "x", Name: "第01集"}}}, nil
}
func (s *stubProvider) Resolve(context.Context, string) (player.Stream, error) {
	return player.Stream{URL: "https://x/a.m3u8", Kind: player.StreamHLS}, nil
}

func TestVodSourcesAndRoutes(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	svc.vods["s1"] = &stubProvider{key: "s1"}
	svc.vodNames["s1"] = "站点一"

	srcs := svc.Sources()
	if len(srcs) != 2 || srcs[0].ID != "live" || srcs[1].ID != "s1" || srcs[1].Name != "站点一" {
		t.Fatalf("Sources 错误: %+v", srcs)
	}
	secs, err := svc.VodCategories("s1")
	if err != nil || len(secs) != 1 || secs[0].Title != "电影" {
		t.Fatalf("VodCategories 错误: %+v %v", secs, err)
	}
	m, err := svc.VodDetail("s1", "1")
	if err != nil || len(m.Episodes) != 1 || m.Sources[0] != "x" {
		t.Fatalf("VodDetail 错误: %+v %v", m, err)
	}
	// 未知站点报错
	if _, err := svc.VodCategories("nope"); err == nil {
		t.Fatalf("未知站点应报错")
	}
}
```

- [ ] **Step 4: 运行确认失败**

Run: `go test ./internal/shell/ -run TestVodSources -count=1`
Expected: FAIL（`undefined: Sources` 等）。

- [ ] **Step 5: 实现 ImportSubscription 收集站点 + Vod* 方法**

在 `service.go` 顶部新增 DTO 类型：

```go
// SourceInfo 是顶层来源（直播或某个点播站）。
type SourceInfo struct {
	ID   string
	Name string
	Kind string // "live" | "vod"
}

// Section 是点播分类。
type Section struct {
	ID    string
	Title string
}

// VodItem 是点播影片列表项。
type VodItem struct {
	ID    string
	Title string
	Logo  string
	Group string
}

// EpisodeInfo 是点播剧集（不向前端暴露 URL）。
type EpisodeInfo struct {
	ID     string
	Source string
	Name   string
}

// VodMedia 是点播详情。
type VodMedia struct {
	ID          string
	Title       string
	Logo        string
	Group       string
	Description string
	Year        string
	Area        string
	Type        string
	Remarks     string
	Sources     []string
	Episodes    []EpisodeInfo
}
```

`ImportSubscription` 末尾（收集 channels 之后）追加站点收集：

```go
	vods := make(map[string]provider.Provider)
	vodNames := make(map[string]string)
	for _, cfg := range cfgs {
		for _, site := range cfg.Sites {
			if site.Type == config.SiteTypeCMS && site.API != "" {
				vods[site.Key] = tvbox.New(site)
				vodNames[site.Key] = site.Name
			}
		}
	}
	s.mu.Lock()
	s.live = lp
	s.vods = vods
	s.vodNames = vodNames
	s.mu.Unlock()
```

新增方法：

```go
// Sources 返回顶层来源（直播 + 各点播站，按 key 排序稳定）。
func (s *ShellService) Sources() []SourceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []SourceInfo{{ID: "live", Name: "直播", Kind: "live"}}
	keys := make([]string, 0, len(s.vods))
	for k := range s.vods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, SourceInfo{ID: k, Name: s.vodNames[k], Kind: "vod"})
	}
	return out
}

func (s *ShellService) vodOf(site string) (provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pv, ok := s.vods[site]
	if !ok {
		return nil, fmt.Errorf("点播站点不存在: %s", site)
	}
	return pv, nil
}

func (s *ShellService) VodCategories(site string) ([]Section, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	secs, err := pv.Home(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Section, len(secs))
	for i, sc := range secs {
		out[i] = Section{ID: sc.ID, Title: sc.Title}
	}
	return out, nil
}

func (s *ShellService) VodList(site, cat string, page int) ([]VodItem, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	pg, err := pv.Browse(context.Background(), cat, page)
	if err != nil {
		return nil, err
	}
	return toVodItems(pg.Items), nil
}

func (s *ShellService) VodSearch(site, q string) ([]VodItem, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return nil, err
	}
	items, err := pv.Search(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return toVodItems(items), nil
}

func (s *ShellService) VodDetail(site, id string) (VodMedia, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return VodMedia{}, err
	}
	m, err := pv.Detail(context.Background(), id)
	if err != nil {
		return VodMedia{}, err
	}
	vm := VodMedia{
		ID: m.ID, Title: m.Title, Logo: m.Logo, Group: m.Group,
		Description: m.Description, Year: m.Year, Area: m.Area,
		Type: m.Type, Remarks: m.Remarks, Sources: m.Sources,
	}
	vm.Episodes = make([]EpisodeInfo, len(m.Episodes))
	for i, e := range m.Episodes {
		vm.Episodes[i] = EpisodeInfo{ID: e.ID, Source: e.Source, Name: e.Name}
	}
	return vm, nil
}

func (s *ShellService) PlayVod(site, epID string) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	pv, err := s.vodOf(site)
	if err != nil {
		return err
	}
	st, err := pv.Resolve(context.Background(), epID)
	if err != nil {
		return err
	}
	if err := s.player.Load(context.Background(), st); err != nil {
		return err
	}
	return s.player.Play()
}

func toVodItems(items []provider.Item) []VodItem {
	out := make([]VodItem, len(items))
	for i, it := range items {
		out[i] = VodItem{ID: it.ID, Title: it.Title, Logo: it.Logo, Group: it.Group}
	}
	return out
}
```

需在 service.go 顶部补 `sort` 与 `tvbox` 两个 import。

- [ ] **Step 6: 运行确认通过**

Run: `go test ./internal/shell/ -count=1`
Expected: PASS（含既有直播测试）。

- [ ] **Step 7: 全量验证**

Run: `go test ./... -count=1` 与 `go vet ./...`、`gofmt -l .`
Expected: 全绿。

- [ ] **Step 8: Commit**

```bash
git add internal/shell/app.go internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(shell): 多源改造，收集 CMS 站点并暴露 Vod* 服务方法"
```

---

## Task 5: 前端「直播 / 点播」切换

**Files:**
- Modify: `frontend/src/App.vue`

**Interfaces:**
- Consumes: Task 4 暴露的 `Sources` / `VodCategories` / `VodList` / `VodSearch` / `VodDetail` / `PlayVod`（需重新生成 `frontend/bindings`）。

- [ ] **Step 1: 重新生成 bindings**

Run: `mise run build:bindings`（或项目现有生成方式，见 `mise.toml` / `Taskfile.yml`；若用 `wails3 generate bindings` 则执行之）。
Expected: `frontend/bindings/.../shell` 里出现 `Sources`/`VodCategories`/`VodList`/`VodSearch`/`VodDetail`/`PlayVod` 与 `SourceInfo`/`Section`/`VodItem`/`VodMedia`/`EpisodeInfo` 类型。

- [ ] **Step 2: 加状态与点播逻辑**

在 `<script setup>` 追加（保留现有直播逻辑）：

```ts
const mode = ref<'live' | 'vod'>('live')
const sources = ref<SourceInfo[]>([])
const activeSite = ref('')
const vodCategories = ref<Section[]>([])
const vodActiveCat = ref('')
const vodItems = ref<VodItem[]>([])
const vodDetail = ref<VodMedia | null>(null)
const vodQuery = ref('')
const vodPage = ref(0)

async function loadSources() {
  sources.value = (await ShellService.Sources()) ?? []
  if (sources.value.length > 1 && !activeSite.value) {
    activeSite.value = sources.value.find(s => s.Kind === 'vod')?.ID ?? ''
  }
}

async function switchMode(m: 'live' | 'vod') {
  mode.value = m
  if (m === 'vod') await loadSources()
}

async function selectSite(id: string) {
  activeSite.value = id
  vodDetail.value = null
  vodPage.value = 0
  await reloadVodCategories()
}

async function reloadVodCategories() {
  vodCategories.value = (await ShellService.VodCategories(activeSite.value)) ?? []
  vodActiveCat.value = vodCategories.value[0]?.ID ?? ''
  await reloadVodList()
}

async function reloadVodList() {
  vodItems.value = (await ShellService.VodList(activeSite.value, vodActiveCat.value, vodPage.value)) ?? []
}

async function vodSearch() {
  if (!vodQuery.value) { await reloadVodList(); return }
  vodItems.value = (await ShellService.VodSearch(activeSite.value, vodQuery.value)) ?? []
}

async function openVodDetail(item: VodItem) {
  vodDetail.value = await ShellService.VodDetail(activeSite.value, item.ID)
}

async function playEpisode(ep: EpisodeInfo) {
  errMsg.value = ''
  try {
    await ShellService.PlayVod(activeSite.value, ep.ID)
    nowPlaying.value = ep.Name
  } catch (e) { errMsg.value = String(e) }
}
```

- [ ] **Step 3: 加模板（点播面板）**

在 `<header>` 下方加切换按钮，在现有 `<section class="layout">` 之外、按 `mode` 条件渲染点播面板（放在直播 layout 之后）：

```vue
<nav class="tabs">
  <button :class="{active: mode==='live'}" @click="switchMode('live')">直播</button>
  <button :class="{active: mode==='vod'}" @click="switchMode('vod')">点播</button>
</nav>

<section v-if="mode==='vod'" class="vod">
  <select v-model="activeSite" @change="selectSite(activeSite)">
    <option v-for="s in sources.filter(x => x.Kind==='vod')" :key="s.ID" :value="s.ID">{{ s.Name }}</option>
  </select>

  <div class="vod-layout">
    <aside class="vod-cats">
      <button v-for="c in vodCategories" :key="c.ID" :class="{active: c.ID===vodActiveCat}"
              @click="vodActiveCat = c.ID; vodPage = 0; reloadVodList()">{{ c.Title }}</button>
    </aside>

    <section class="vod-main">
      <div class="search"><input v-model="vodQuery" placeholder="搜索影片" @input="vodSearch" /></div>
      <ul v-if="!vodDetail">
        <li v-for="it in vodItems" :key="it.ID" class="channel" @click="openVodDetail(it)">
          <span class="name">{{ it.Title }}</span><span class="group">{{ it.Group }}</span>
        </li>
      </ul>
      <div v-else class="vod-detail">
        <button @click="vodDetail = null">← 返回</button>
        <h2>{{ vodDetail.Title }}</h2>
        <p class="meta">{{ vodDetail.Type }} · {{ vodDetail.Year }} · {{ vodDetail.Area }}</p>
        <p>{{ vodDetail.Description }}</p>
        <div v-for="(src, i) in vodDetail.Sources" :key="src" class="ep-src">
          <p class="ep-src-name">{{ src }}</p>
          <button v-for="ep in vodDetail.Episodes.filter(e => e.Source===src)" :key="ep.ID"
                  @click="playEpisode(ep)">{{ ep.Name }}</button>
        </div>
      </div>
    </section>
  </div>
</section>
```

TypeScript 需在文件顶部引入新绑定类型：`SourceInfo`、`Section`、`VodItem`、`VodMedia`、`EpisodeInfo`（从 `../bindings/.../shell` 按需 import 或直接用 `ShellService` 返回类型推断）。

- [ ] **Step 4: 类型检查 / 构建前端**

Run: `cd frontend && npm run build`（或 `mise run build` 里对应的前端步骤）。
Expected: 无 TS 错误。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.vue frontend/bindings
git commit -m "feat(frontend): 顶层直播/点播切换 + 点播浏览详情面板"
```

---

## Task 6: 集成验收

- [ ] **Step 1: 全量测试 + 静态检查**

Run: `go test ./... -count=1 && go vet ./... && gofmt -l .`
Expected: 全绿、无输出。

- [ ] **Step 2: Linux 构建**

Run: `CGO_ENABLED=1 go build ./...` 与 `mise run build:linux`
Expected: 构建成功产出 `bin/unbox`。

- [ ] **Step 3: 手动验收**

Run: `./bin/unbox`，导入双龙订阅，切换「点播」tab。
Expected：出现 4 个 CMS 站点；进入「非凡资源」→ 出分类 → 出影片列表 → 点开详情见线路与剧集 → 点剧集出画面；「直播」tab 不受影响。

- [ ] **Step 4: 记录验收结果并更新 HANDOFF**

按验收结果更新 `docs/HANDOFF.md` 的进度（M2 完成情况、遗留项）。

---

## Self-Review

- **Spec coverage**：Spec §3.1 类型扩展→Task 1；§3.4 tvbox Provider→Task 3；§3.5 壳层多源→Task 4；§3.6 前端→Task 5；§5 测试（真实 fixture）→Task 1/2/3 均用 `testdata/cms/` 或 httptest。覆盖完整。
- **Placeholder scan**：无 TBD/TODO；所有测试含具体断言与期望值（fixture 实测 14 集、8 分类）。
- **Type consistency**：`Episode`（provider 包）与 `EpisodeInfo`（shell DTO）命名区分明确；`parseEpisodes` 返回 `[]provider.Episode`，`VodDetail` 映射为 `[]EpisodeInfo`。epID 编码 `vodID/si/ei` 在 Task 1（生成）与 Task 3（解析）一致。
