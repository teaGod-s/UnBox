package rule

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// XBPQEngine 是 csp_XBPQ 的规则解释器。
type XBPQEngine struct {
	rule *XBPQ
	c    *Client
}

// NewXBPQEngine 构造引擎。
func NewXBPQEngine(r *XBPQ, c *Client) *XBPQEngine {
	return &XBPQEngine{rule: r, c: c}
}

func (e *XBPQEngine) Home(context.Context) ([]provider.Section, error) {
	var out []provider.Section
	for _, part := range strings.Split(e.rule.Classes, "#") {
		name, id, ok := strings.Cut(strings.TrimSpace(part), "$")
		if !ok {
			id = name
		}
		name, id = strings.TrimSpace(name), strings.TrimSpace(id)
		if name == "" || id == "" {
			continue
		}
		out = append(out, provider.Section{ID: id, Title: name})
	}
	return out, nil
}

func (e *XBPQEngine) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if page <= 0 {
		page = 1
	}
	raw, err := e.c.Get(ctx, expandURL(e.rule.CategoryURL, cat, page))
	if err != nil {
		return provider.Page{}, err
	}
	items := e.parseItems(raw, e.rule.Array, e.rule.Title, e.rule.Link, e.rule.Pic, e.rule.Subtitle)
	return provider.Page{Items: items, Page: page, HasMore: len(items) > 0}, nil
}

func (e *XBPQEngine) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if strings.TrimSpace(e.rule.SearchURL) == "" {
		return nil, nil
	}
	u := strings.NewReplacer("{wd}", url.QueryEscape(q), "{pg}", "1").Replace(e.rule.SearchURL)
	raw, err := e.c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return e.parseItems(raw,
		firstNonEmpty(e.rule.SearchArray, e.rule.Array),
		firstNonEmpty(e.rule.SearchTitle, e.rule.Title),
		firstNonEmpty(e.rule.SearchLink, e.rule.Link),
		firstNonEmpty(e.rule.SearchPic, e.rule.Pic),
		firstNonEmpty(e.rule.SearchSubtitle, e.rule.Subtitle)), nil
}

func (e *XBPQEngine) Detail(ctx context.Context, id string) (Vod, error) {
	raw, err := e.c.Get(ctx, id)
	if err != nil {
		return Vod{}, err
	}
	name := firstNonEmpty(h1Text(raw), Clean(Cut(raw, e.rule.Title)), titleTag(raw))
	v := Vod{
		ID:       id,
		Name:     name,
		TypeName: Clean(Cut(raw, firstNonEmpty(e.rule.Type, e.rule.LegacyType))),
		Year:     Clean(Cut(raw, firstNonEmpty(e.rule.Year, e.rule.LegacyYear))),
		Area:     Clean(Cut(raw, firstNonEmpty(e.rule.Area, e.rule.LegacyArea))),
		Remarks:  Clean(Cut(raw, firstNonEmpty(e.rule.Remarks, e.rule.LegacyStatus))),
		Content:  Clean(Cut(raw, e.rule.Content)),
	}
	var froms, plays []string
	for _, seg := range CutList(raw, e.rule.PlayFromArray) {
		if name := Clean(Cut(seg, e.rule.PlayFromTitle)); name != "" {
			froms = append(froms, name)
		}
	}
	playList := firstNonEmpty(e.rule.PlayList, `<a&&</a>`)
	playTitle := firstNonEmpty(e.rule.PlayTitle, `>&&`)
	playLink := firstNonEmpty(e.rule.PlayLink, `href="&&"`)
	for _, seg := range CutList(raw, e.rule.PlayArray) {
		var eps []string
		for _, entry := range CutList(seg, playList) {
			name := Clean(Cut(entry, playTitle))
			href := Clean(Cut(entry, playLink))
			if href == "" || isNonPlayableHref(href) {
				continue
			}
			eps = append(eps, name+"$"+e.absURL(href))
		}
		if len(eps) > 0 {
			plays = append(plays, strings.Join(eps, "#"))
		}
	}
	// 线路数组规则可能匹配到页面其他区块；线路名不能多于实际播放段。
	if len(froms) > len(plays) {
		froms = froms[:len(plays)]
	}
	v.PlayFrom = strings.Join(froms, "$$$")
	v.PlayURL = strings.Join(plays, "$$$")
	return v, nil
}

// isNonPlayableHref 过滤排序按钮等 javascript:; 伪链接。
func isNonPlayableHref(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "javascript:") || strings.HasPrefix(h, "#")
}

func (e *XBPQEngine) Resolve(_ context.Context, epURL string) (player.Stream, error) {
	return player.Stream{
		URL:     epURL,
		Kind:    player.KindForURL(epURL),
		Headers: map[string]string{"User-Agent": tvboxUA, "Referer": originOf(e.rule.HomeURL)},
	}, nil
}

func (e *XBPQEngine) parseItems(raw, array, title, link, pic, subtitle string) []provider.Item {
	var out []provider.Item
	for _, seg := range CutList(raw, array) {
		name := Clean(Cut(seg, title))
		href := Clean(Cut(seg, link))
		if name == "" || href == "" {
			continue
		}
		href = e.absURL(e.rule.LinkPrefix + href + e.rule.LinkSuffix)
		out = append(out, provider.Item{
			ID:    href,
			Title: name,
			Logo:  Clean(Cut(seg, pic)),
			Group: Clean(Cut(seg, subtitle)),
		})
	}
	return out
}

func (e *XBPQEngine) absURL(raw string) string {
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	base, err := url.Parse(e.rule.HomeURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func expandURL(tpl, cat string, page int) string {
	return strings.NewReplacer(
		"{cateId}", cat, "{catePg}", strconv.Itoa(page),
		"{area}", "", "{by}", "", "{class}", "", "{lang}", "",
		"{year}", "", "{letter}", "",
	).Replace(tpl)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var h1Re = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)

// titleTag 是详情页标题的兜底：有的规则没有给 标题 字段。
func titleTag(s string) string {
	m := titleRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return Clean(m[1])
}

// h1Text 取详情页首个 h1 的文本；真实站点的列表 标题 规则常会先匹配到导航
// 里的「首页」，详情页用 h1 更贴近实际片名。
func h1Text(s string) string {
	m := h1Re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return Clean(m[1])
}

var _ Engine = (*XBPQEngine)(nil)
