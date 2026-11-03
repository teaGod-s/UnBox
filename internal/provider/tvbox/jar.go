package tvbox

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/tvbox/rule"
)

// JarProvider 是规则型 JAR 站点的 Provider。
type JarProvider struct {
	site config.Site

	mu      sync.Mutex
	engine  rule.Engine
	loaded  bool
	loadErr error
	cache   map[string]provider.Episode
}

// NewJar 校验类名与 ext；规则在首次动作时懒加载，避免导入源阶段联网。
func NewJar(site config.Site) (*JarProvider, error) {
	if !rule.Supported(site.API) {
		return nil, fmt.Errorf("暂不支持的 JAR 爬虫类: %s", site.API)
	}
	if site.ExtString() == "" {
		return nil, fmt.Errorf("JAR 站点 %s 缺少 ext 规则地址", site.Key)
	}
	return &JarProvider{site: site, cache: map[string]provider.Episode{}}, nil
}

func (p *JarProvider) ID() string { return p.site.Key }

func (p *JarProvider) ensureLoaded(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.loadErr
	}
	p.engine, p.loadErr = rule.NewEngine(ctx, p.site.API, p.site.ExtString())
	p.loaded = true
	return p.loadErr
}

func (p *JarProvider) Home(ctx context.Context) ([]provider.Section, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	return p.engine.Home(ctx)
}

func (p *JarProvider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Page{}, err
	}
	return p.engine.Browse(ctx, cat, page)
}

func (p *JarProvider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (provider.Page, error) {
	return p.Browse(ctx, cat, page)
}

func (p *JarProvider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	return p.engine.Search(ctx, q)
}

func (p *JarProvider) Detail(ctx context.Context, id string) (provider.Media, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Media{}, err
	}
	v, err := p.engine.Detail(ctx, id)
	if err != nil {
		return provider.Media{}, err
	}
	sources := splitSources(v.PlayFrom)
	eps := parseEpisodes(id, v.PlayURL, sources)
	p.mu.Lock()
	for _, ep := range eps {
		p.cache[ep.ID] = ep
	}
	p.mu.Unlock()
	return provider.Media{
		ID: id, Title: v.Name, Logo: v.Pic, Group: v.TypeName,
		Description: v.Content, Year: v.Year, Area: v.Area,
		Type: v.TypeName, Remarks: v.Remarks, Sources: sources, Episodes: eps,
	}, nil
}

func (p *JarProvider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return player.Stream{}, err
	}
	p.mu.Lock()
	ep, ok := p.cache[epID]
	p.mu.Unlock()
	if !ok {
		vodID, _, _, err := parseEpID(epID)
		if err != nil {
			return player.Stream{}, err
		}
		if _, err := p.Detail(ctx, vodID); err != nil {
			return player.Stream{}, err
		}
		p.mu.Lock()
		ep, ok = p.cache[epID]
		p.mu.Unlock()
		if !ok {
			return player.Stream{}, fmt.Errorf("剧集不存在: %s", epID)
		}
	}
	return p.engine.Resolve(ctx, ep.URL)
}

func (p *JarProvider) String() string {
	return "jar:" + strings.TrimSpace(p.site.API)
}
