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
	return provider.Page{Items: toItems(items)}, nil
}

func (p *Provider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	items, err := p.c.videolist(ctx, "", q, 0)
	if err != nil {
		return nil, err
	}
	return toItems(items), nil
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

// toItems 把 cmsVideo 映射为 provider.Item。
func toItems(videos []cmsVideo) []provider.Item {
	out := make([]provider.Item, 0, len(videos))
	for _, v := range videos {
		out = append(out, provider.Item{
			ID:    strconv.FormatInt(v.VodID, 10),
			Title: v.VodName,
			Logo:  v.VodPic,
			Group: v.TypeName,
		})
	}
	return out
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

// kindForURL 依据 URL 扩展名猜流形态。
func kindForURL(u string) player.StreamKind {
	return player.KindForURL(u)
}
