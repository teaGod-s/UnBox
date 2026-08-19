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
