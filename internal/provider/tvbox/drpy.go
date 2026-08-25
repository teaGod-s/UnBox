package tvbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// drpyClass 是 drpy 首页响应里的分类条目。
type drpyClass struct {
	TypeID   string `json:"type_id"`
	TypeName string `json:"type_name"`
}

// drpyListResp 是 drpy 列表/详情响应的信封。drpy 无 code 字段，直接带 class/list。
type drpyListResp struct {
	Class []drpyClass `json:"class"`
	List  []cmsVideo  `json:"list"`
}

// drpyClient 是单个 drpy2/drpyS 站点（type=3 http）的 HTTP 客户端。
// 端点与 CMS 不同，但返回的 vod_* 结构与 CMS 同构，条目解析复用 cmsVideo。
//
// 端点为 canonical dr_py2 约定，drpyS 或有差异（待真实实例实测钉死）：
//
//	home    GET {api}/api/homeVod
//	category GET {api}/api/category?tid&pg
//	search  GET {api}/api/searchContent?key
//	detail  GET {api}/api/detailContent?ids
type drpyClient struct {
	base string
	hc   *http.Client
}

func newDrpyClient(api string) *drpyClient {
	base, _, _ := strings.Cut(api, "?")
	base = strings.TrimRight(base, "/")
	return &drpyClient{base: base, hc: &http.Client{Timeout: 15 * time.Second}}
}

// homeVod 拉取首页分类（class 数组）。
func (c *drpyClient) homeVod(ctx context.Context) ([]drpyClass, error) {
	raw, err := c.get(ctx, "/api/homeVod", nil)
	if err != nil {
		return nil, err
	}
	var resp drpyListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析首页响应失败: %w", err)
	}
	return resp.Class, nil
}

// category 拉取分类列表。
func (c *drpyClient) category(ctx context.Context, tid string, page int) ([]cmsVideo, error) {
	return c.list(ctx, "/api/category", url.Values{"tid": {tid}, "pg": {fmt.Sprintf("%d", page)}})
}

// search 搜索影片。
func (c *drpyClient) search(ctx context.Context, wd string) ([]cmsVideo, error) {
	return c.list(ctx, "/api/searchContent", url.Values{"key": {wd}})
}

// list 拉取一个返回 {list:[vod_*]} 的端点。
func (c *drpyClient) list(ctx context.Context, path string, q url.Values) ([]cmsVideo, error) {
	raw, err := c.get(ctx, path, q)
	if err != nil {
		return nil, err
	}
	var resp drpyListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析列表响应失败: %w", err)
	}
	return resp.List, nil
}

// detail 拉取单部影片详情。
func (c *drpyClient) detail(ctx context.Context, ids string) (*cmsVideo, error) {
	raw, err := c.get(ctx, "/api/detailContent", url.Values{"ids": {ids}})
	if err != nil {
		return nil, err
	}
	var resp drpyListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析详情响应失败: %w", err)
	}
	if len(resp.List) == 0 {
		return nil, fmt.Errorf("站点未返回详情数据")
	}
	return &resp.List[0], nil
}

// get 发起带 UA 的 GET，返回响应体。path 为 /api/* 端点，q 为 query。
func (c *drpyClient) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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

// Drpy 是 type=3 爬虫站点（drpy2/drpyS 服务）的 Provider 实现。
// 与 CMS Provider 共用 vod_* 解析与剧集 epID 编码，仅 HTTP 客户端不同。
type Drpy struct {
	site config.Site
	c    *drpyClient

	mu    sync.Mutex
	cache map[string]provider.Episode
}

// NewDrpy 用站点定义构造 drpy 客户端 Provider。
func NewDrpy(site config.Site) *Drpy {
	return &Drpy{site: site, c: newDrpyClient(site.API), cache: map[string]provider.Episode{}}
}

func (p *Drpy) ID() string { return p.site.Key }

func (p *Drpy) Home(ctx context.Context) ([]provider.Section, error) {
	classes, err := p.c.homeVod(ctx)
	if err != nil {
		return nil, err
	}
	secs := make([]provider.Section, 0, len(classes))
	for _, cls := range classes {
		if cls.TypeID == "" {
			continue
		}
		secs = append(secs, provider.Section{ID: cls.TypeID, Title: cls.TypeName})
	}
	return secs, nil
}

func (p *Drpy) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	items, err := p.c.category(ctx, cat, page)
	if err != nil {
		return provider.Page{}, err
	}
	return provider.Page{Items: toItems(items)}, nil
}

func (p *Drpy) Search(ctx context.Context, q string) ([]provider.Item, error) {
	items, err := p.c.search(ctx, q)
	if err != nil {
		return nil, err
	}
	return toItems(items), nil
}

func (p *Drpy) Detail(ctx context.Context, id string) (provider.Media, error) {
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
// 当前直接用详情里的剧集 URL；drpy 的 playerContent 懒加载/解析留待实测后补充。
func (p *Drpy) Resolve(ctx context.Context, epID string) (player.Stream, error) {
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
