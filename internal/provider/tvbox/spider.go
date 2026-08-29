package tvbox

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/crawler"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Spider 是 type=3 且 api 指向 .js 文件的爬虫 Provider。
// 爬虫脚本在第一次动作时加载，避免导入源阶段阻塞网络请求。
type Spider struct {
	site config.Site

	mu      sync.Mutex
	engine  *crawler.Engine
	loaded  bool
	loadErr error
	cache   map[string]provider.Episode
}

// NewSpider 用站点定义构造 JS 爬虫 Provider。
func NewSpider(site config.Site) (*Spider, error) {
	if strings.TrimSpace(site.API) == "" {
		return nil, fmt.Errorf("JS 爬虫地址为空")
	}
	return &Spider{site: site, engine: crawler.New(), cache: make(map[string]provider.Episode)}, nil
}

func (p *Spider) ID() string { return p.site.Key }

func (p *Spider) ensureLoaded(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.loadErr
	}
	p.loadErr = p.engine.LoadFromURL(ctx, p.site.API)
	p.loaded = true
	return p.loadErr
}

func (p *Spider) Home(ctx context.Context) ([]provider.Section, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rule, err := p.engine.Rule()
	if err == nil && strings.TrimSpace(rule.ClassName) != "" {
		names := strings.Split(rule.ClassName, "&")
		ids := strings.Split(rule.ClassURL, "&")
		sections := make([]provider.Section, 0, len(names))
		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id := name
			if i < len(ids) && strings.TrimSpace(ids[i]) != "" {
				id = strings.TrimSpace(ids[i])
			}
			sections = append(sections, provider.Section{ID: id, Title: name})
		}
		return sections, nil
	}
	vods, err := p.engine.VodHome()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	sections := make([]provider.Section, 0)
	for _, vod := range vods {
		name := strings.TrimSpace(vod.TypeName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		sections = append(sections, provider.Section{ID: name, Title: name})
	}
	return sections, nil
}

func (p *Spider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Page{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	items, err := p.engine.VodCategory(cat, page)
	if err != nil {
		return provider.Page{}, err
	}
	return provider.Page{Items: spiderItems(items)}, nil
}

func (p *Spider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	items, err := p.engine.VodSearch(q)
	if err != nil {
		return nil, err
	}
	return spiderItems(items), nil
}

func (p *Spider) Detail(ctx context.Context, id string) (provider.Media, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Media{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	detail, err := p.engine.VodDetail(id)
	if err != nil {
		return provider.Media{}, err
	}
	sources := splitSources(detail.VodPlayFrom)
	episodes := parseEpisodes(id, detail.VodPlayURL, sources)
	for _, episode := range episodes {
		p.cache[episode.ID] = episode
	}
	return provider.Media{
		ID: id, Title: detail.VodName, Logo: detail.VodPic, Group: detail.TypeName,
		Description: detail.VodContent, Year: detail.VodYear, Area: detail.VodArea,
		Type: detail.TypeName, Remarks: detail.VodRemarks, Sources: sources, Episodes: episodes,
	}, nil
}

func (p *Spider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return player.Stream{}, err
	}
	p.mu.Lock()
	episode, ok := p.cache[epID]
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
		episode, ok = p.cache[epID]
		p.mu.Unlock()
		if !ok {
			return player.Stream{}, fmt.Errorf("剧集不存在: %s", epID)
		}
	}
	return player.Stream{URL: episode.URL, Kind: kindForURL(episode.URL), Headers: map[string]string{"Referer": originOf(p.site.API)}}, nil
}

func spiderItems(vods []crawler.Vod) []provider.Item {
	items := make([]provider.Item, 0, len(vods))
	for _, vod := range vods {
		items = append(items, provider.Item{ID: vod.VodID, Title: vod.VodName, Logo: vod.VodPic, Group: firstNonEmpty(vod.TypeName, vod.VodRemarks)})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
