package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// FlexString 兼容 string 与 JSON 值两种形态：JSON 的值直接取字符串，
// 对象/数组则序列化成紧凑 JSON 字符串（如 POST请求数据 可写成
// {"sk":"{wd}","offset":0}，反序列化后仍是 JSON 字符串，供 Search 拼请求体）。
type FlexString string

// UnmarshalJSON 实现 string / object / array 三态兼容。
func (f *FlexString) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// 对象/数组：重序列化成紧凑 JSON（map 遍历无序，但对请求体语义无影响）。
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	*f = FlexString(b)
	return nil
}

// XYQ 是 XYQHiker 规则文件的字段子集。字段名保持规则文件的中文键。
// 所有字段可选；缺失时对应能力返回空结果而非报错。
type XYQ struct {
	HomeURL         string     `json:"首页推荐链接"`
	CategoryNames   string     `json:"分类名称"`
	CategoryIDs     string     `json:"分类名称替换词"`
	CategoryURL     string     `json:"分类链接"`
	CategoryArray   string     `json:"分类列表数组规则"`
	CatTitle        string     `json:"分类片单标题"`
	CatLink         string     `json:"分类片单链接"`
	CatPic          string     `json:"分类片单图片"`
	CatSubtitle     string     `json:"分类片单副标题"`
	LinkPrefix      string     `json:"分类片单链接加前缀"`
	LinkSuffix      string     `json:"分类片单链接加后缀"`
	SearchURL       string     `json:"搜索链接"`
	PostBody        FlexString `json:"POST请求数据"`
	SearchModeJSON  string     `json:"搜索截取模式"` // "0" = JSON 响应
	SearchArray     string     `json:"搜索列表数组规则"`
	SearchTitle     string     `json:"搜索片单标题"`
	SearchLink      string     `json:"搜索片单链接"`
	SearchPic       string     `json:"搜索片单图片"`
	SearchSubtitle  string     `json:"搜索片单副标题"`
	SearchPrefix    string     `json:"搜索片单链接加前缀"`
	SearchSuffix    string     `json:"搜索片单链接加后缀"`
	SearchHeaders   HeaderMap  `json:"搜索请求头参数"`
	TypeDetail      string     `json:"类型详情"`
	YearDetail      string     `json:"年代详情"`
	AreaDetail      string     `json:"地区详情"`
	DirectorDetail  string     `json:"导演详情"`
	Actor           string     `json:"演员详情"`
	Content         string     `json:"简介详情"`
	DetailMode      string     `json:"详情是否Jsoup写法"` // "是"/"1" = JSoup；否则标记截取/XPath
	PlayerTabArray  string     `json:"线路列表数组规则"`
	PlayerTabTitle  string     `json:"线路标题"`
	PlayerListArray string     `json:"播放列表数组规则"`
	PlayArray       string     `json:"选集列表数组规则"`
	PlayTitle       string     `json:"选集标题"`
	PlayLink        string     `json:"选集链接"`
	PlayPrefix      string     `json:"选集链接加前缀"`
	PlaySuffix      string     `json:"选集链接加后缀"`
	Reverse         string     `json:"是否反转选集序列"`
	DirectPlay      string     `json:"链接是否直接播放"`
	SniffEnabled    string     `json:"是否开启手动嗅探"`
	SniffKeys       string     `json:"手动嗅探视频链接关键词"`
	SniffFilters    string     `json:"手动嗅探视频链接过滤词"`
	Headers         HeaderMap  `json:"请求头参数"`
}

// ParseXYQ 解析规则 JSON；规则是 JSONC（可能含 // 注释），先剥离注释。
func ParseXYQ(data []byte) (*XYQ, error) {
	clean := stripJSONCComments(data)
	var r XYQ
	if err := json.Unmarshal(sanitizeJSONEscapes(clean), &r); err != nil {
		return nil, fmt.Errorf("解析 XYQHiker 规则失败: %w", err)
	}
	if strings.TrimSpace(r.HomeURL) == "" && strings.TrimSpace(r.CategoryURL) == "" &&
		strings.TrimSpace(r.SearchURL) == "" {
		return nil, fmt.Errorf("XYQHiker 规则缺少 首页推荐链接/分类链接/搜索链接")
	}
	return &r, nil
}

// stripJSONCComments 逐行去掉行首 // 注释（规则文件里层内注释不能删，
// 例如 URL 里的 "https://"）。
func stripJSONCComments(data []byte) []byte {
	var out []byte
	for len(data) > 0 {
		line := data
		if i := indexByte(data, '\n'); i >= 0 {
			line, data = data[:i+1], data[i+1:]
		} else {
			data = nil
		}
		trimmed := strings.TrimLeftFunc(string(line), func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\r'
		})
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		out = append(out, line...)
	}
	return out
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// XYQHikerEngine 是 csp_XYQHiker 的规则解释器。
type XYQHikerEngine struct {
	rule *XYQ
	c    *Client
}

// NewXYQHikerEngine 构造引擎。
func NewXYQHikerEngine(r *XYQ, c *Client) *XYQHikerEngine {
	return &XYQHikerEngine{rule: r, c: c}
}

func (e *XYQHikerEngine) Home(context.Context) ([]provider.Section, error) {
	if strings.TrimSpace(e.rule.CategoryNames) == "" {
		return nil, nil
	}
	names := strings.Split(e.rule.CategoryNames, "&")
	ids := strings.Split(e.rule.CategoryIDs, "&")
	var out []provider.Section
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id := name
		if i < len(ids) && strings.TrimSpace(ids[i]) != "" {
			id = strings.TrimSpace(ids[i])
		}
		out = append(out, provider.Section{ID: id, Title: name})
	}
	return out, nil
}

func (e *XYQHikerEngine) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if strings.TrimSpace(e.rule.CategoryURL) == "" {
		return provider.Page{}, fmt.Errorf("该站点没有配置 分类链接")
	}
	if page <= 0 {
		page = 1
	}
	raw, err := e.fetch(ctx, e.absURL(e.resolveCategoryURL(cat, page)), nil)
	if err != nil {
		return provider.Page{}, err
	}
	items := e.parseHTMLItems(e.newDoc(raw), "分类", e.rule.CategoryArray,
		e.rule.CatTitle, e.rule.CatLink, e.rule.CatPic, e.rule.CatSubtitle,
		e.rule.LinkPrefix, e.rule.LinkSuffix)
	return provider.Page{Items: items, Page: page, HasMore: len(items) > 0}, nil
}

func (e *XYQHikerEngine) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if strings.TrimSpace(e.rule.SearchURL) == "" {
		return nil, nil
	}
	// 搜索链接可能以 ";post"/";get" 结尾声明方法（如 yingso 的 .../search;post）。
	target, forcePost := splitMethodSuffix(xyqReplaceSearch(e.rule.SearchURL, q))
	body := strings.NewReplacer("{wd}", q).Replace(string(e.rule.PostBody))
	usePost := forcePost || strings.TrimSpace(body) != ""

	var raw string
	var err error
	if usePost {
		contentType := "application/x-www-form-urlencoded"
		if strings.HasPrefix(strings.TrimSpace(body), "{") {
			contentType = "application/json"
		}
		raw, err = e.c.Post(ctx, e.absURL(target), body, contentType)
	} else {
		raw, err = e.fetch(ctx, e.absURL(target), e.rule.SearchHeaders)
	}
	if err != nil {
		return nil, err
	}
	// 截取模式 "0"：JSON 响应，列表数组是 JSON 路径，字段是 JSON 键。
	if e.rule.SearchModeJSON == "0" {
		return e.parseJSONItems(raw)
	}
	items := e.parseHTMLItems(e.newDoc(raw), "搜索", e.rule.SearchArray,
		e.rule.SearchTitle, e.rule.SearchLink, e.rule.SearchPic, e.rule.SearchSubtitle,
		e.rule.SearchPrefix, e.rule.SearchSuffix)
	return items, nil
}

// splitMethodSuffix 拆出搜索链接尾部的 ";post"/";get" 方法声明。
func splitMethodSuffix(raw string) (url string, post bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasSuffix(raw, ";post"):
		return strings.TrimSuffix(raw, ";post"), true
	case strings.HasSuffix(raw, ";get"):
		return strings.TrimSuffix(raw, ";get"), false
	default:
		return raw, false
	}
}

func (e *XYQHikerEngine) Detail(ctx context.Context, id string) (Vod, error) {
	raw, err := e.fetch(ctx, id, nil)
	if err != nil {
		return Vod{}, err
	}
	doc := e.newDoc(raw)
	name := firstNonEmpty(h1Text(raw), titleTag(raw))
	pic := ""
	// 详情页海报优先按「分类片单图片」规则在整页提取（真实 XYQHiker 站点的
	// 详情页复用列表图片字段，如去看吧动漫的 ".fed-list-pics&&data-original"），
	// 而不是硬编码某个站点特有结构。
	if strings.TrimSpace(e.rule.CatPic) != "" {
		if vals := jsoupExtractFirst(doc, e.rule.CatPic); len(vals) > 0 && vals[0] != "" {
			pic = vals[0]
		}
	}
	if pic == "" {
		if img := doc.Find(".module-item-pic img").First(); img.Length() > 0 {
			if src, ok := img.Attr("data-src"); ok {
				pic = src
			} else if src, ok = img.Attr("src"); ok {
				pic = src
			}
		}
	}
	if pic == "" {
		pic = ogImage(raw)
	}
	if pic != "" {
		pic = e.absURL(pic)
	}
	v := Vod{
		ID:       id,
		Name:     name,
		Pic:      pic,
		TypeName: e.extractDetail(raw, e.rule.TypeDetail),
		Year:     e.extractDetail(raw, e.rule.YearDetail),
		Area:     e.extractDetail(raw, e.rule.AreaDetail),
		Content:  e.extractDetail(raw, e.rule.Content),
	}
	actor := e.extractDetail(raw, e.rule.Actor)
	_ = actor // 演员信息保留在 Content 之外（Vod 无此字段，留待接入）

	// 线路
	var froms []string
	for _, node := range e.selectNodes(doc, e.rule.PlayerTabArray) {
		if t := e.extractOn(node, e.rule.PlayerTabTitle); t != "" {
			froms = append(froms, t)
		}
	}
	// 播放列表段：每个 .module-player-list 是一段，段内是选集
	var plays []string
	for _, listNode := range e.selectNodes(doc, e.rule.PlayerListArray) {
		var eps []string
		for _, row := range e.selectOn(listNode, e.rule.PlayArray) {
			name := e.extractOn(row, e.rule.PlayTitle)
			href := e.extractOn(row, e.rule.PlayLink)
			if name == "" || href == "" || isNonPlayableHref(href) {
				continue
			}
			href = e.absURL(withPrefixSuffix(href, e.rule.PlayPrefix, e.rule.PlaySuffix))
			eps = append(eps, name+"$"+href)
		}
		if xyqReverseEnabled(e.rule.Reverse) {
			reverseStrings(eps)
		}
		if len(eps) > 0 {
			plays = append(plays, strings.Join(eps, "#"))
		}
	}
	if len(plays) == 0 {
		// 选集规则可能全部不匹配（如 :matches 过滤条件与页面不符）。
		// 绝不能因此把线路名也砍掉：保留原样，让上层看到「有线路无剧集」。
		v.PlayFrom = strings.Join(froms, "$$$")
		return v, nil
	}
	if len(froms) > len(plays) {
		froms = froms[:len(plays)]
	}
	v.PlayFrom = strings.Join(froms, "$$$")
	v.PlayURL = strings.Join(plays, "$$$")
	return v, nil
}

func (e *XYQHikerEngine) Resolve(ctx context.Context, epURL string) (player.Stream, error) {
	realURL := epURL
	if !looksLikeMediaURL(epURL) {
		if page, err := e.c.Get(ctx, epURL); err == nil {
			if u := decodeDataPlay(page); u != "" {
				realURL = u
			} else if u := findMediaURL(page); u != "" {
				realURL = u
			}
		}
	}
	return player.Stream{
		URL:     realURL,
		Kind:    player.KindForURL(realURL),
		Headers: map[string]string{"User-Agent": tvboxUA, "Referer": originOf(e.rule.HomeURL)},
	}, nil
}

// parseHTMLItems 解析一个 HTML 列表（分类首页与搜索共用）。
func (e *XYQHikerEngine) parseHTMLItems(doc *goquery.Document, kind, array, title, link, pic, subtitle, prefix, suffix string) []provider.Item {
	if strings.TrimSpace(array) == "" {
		return nil
	}
	var out []provider.Item
	for _, node := range e.selectNodes(doc, array) {
		name := e.extractOn(node, title)
		href := e.extractOn(node, link)
		if name == "" && kind == "搜索" {
			// 搜索规则可能只有标题没有链接（无详情页）；跳过
		}
		if href == "" {
			if kind == "分类" {
				continue
			}
		}
		logo := e.extractOn(node, pic)
		sub := e.extractOn(node, subtitle)
		if logo != "" {
			logo = e.absURL(withPrefixSuffix(logo, prefix, suffix))
		}
		out = append(out, provider.Item{
			ID:    e.absURL(withPrefixSuffix(href, prefix, suffix)),
			Title: name,
			Logo:  logo,
			Group: sub,
		})
	}
	return out
}

// parseJSONItems 解析 JSON 响应（截取模式 0）。
func (e *XYQHikerEngine) parseJSONItems(raw string) ([]provider.Item, error) {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("搜索 JSON 解析失败: %w", err)
	}
	arr := descendJSONPath(root, e.rule.SearchArray)
	if arr == nil {
		return nil, nil
	}
	// 兼容数组和单个对象
	list, ok := arr.([]any)
	if !ok {
		if _, ok2 := arr.(map[string]any); ok2 {
			list = []any{arr}
		} else {
			return nil, nil
		}
	}
	var out []provider.Item
	for _, entry := range list {
		obj, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		title := jsonStr(obj[e.rule.SearchTitle])
		href := jsonStr(obj[e.rule.SearchLink])
		if strings.TrimSpace(title) == "" && strings.TrimSpace(href) == "" {
			continue
		}
		item := provider.Item{
			ID:    e.absURL(withPrefixSuffix(href, e.rule.SearchPrefix, e.rule.SearchSuffix)),
			Title: title,
			Logo:  e.absURL(withPrefixSuffix(jsonStr(obj[e.rule.SearchPic]), e.rule.SearchPrefix, e.rule.SearchSuffix)),
			Group: jsonStr(obj[e.rule.SearchSubtitle]),
		}
		if strings.TrimSpace(item.ID) == "" {
			item.ID = item.Title // 无链接时以标题兜底 id，仍可进详情尝试
		}
		out = append(out, item)
	}
	return out, nil
}

// ascendJSONPath 按点号路径下钻 JSON（"data"、"data.list"）。
func descendJSONPath(v any, path string) any {
	if strings.TrimSpace(path) == "" {
		return v
	}
	cur := v
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[seg]
		if !ok {
			return nil
		}
	}
	return cur
}

func jsonStr(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// extractDetail 详情字段提取：按「详情是否Jsoup写法」分流。
//   - "是"/"1"（或空）：JSoup 链式规则；
//   - 规则以 "/" 开头：XPath 子集；
//   - 其余：XBPQ 式 "A&&B" 标记截取（XYQHiker 非 JSoup 详情的常见写法）。
func (e *XYQHikerEngine) extractDetail(raw string, rule string) string {
	if strings.TrimSpace(rule) == "" {
		return ""
	}
	rule = strings.TrimSpace(rule)
	if isJsoupNonNull(e.rule.DetailMode) {
		return jsoupExtractOn(e.newDoc(raw).Selection, rule)
	}
	if strings.HasPrefix(rule, "/") {
		vals := xpathExtractFirst(e.newDoc(raw), rule)
		if len(vals) == 0 {
			return ""
		}
		return vals[0]
	}
	// 标记截取
	return Clean(Cut(raw, rule))
}

// isJsoupNonNull 判断「是否Jsoup写法」类字段是否取 JSoup 语义。
// 真值："是"/"1"/"Y/y"；否则走标记截取/XPath。空视为 JSoup（大多数规则主路径）。
func isJsoupNonNull(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", "是", "1", "y":
		return true
	default:
		return false
	}
}

func (e *XYQHikerEngine) selectNodes(doc *goquery.Document, rule string) []*goquery.Selection {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	sel := jsoupSelectNodes(doc, rule)
	if sel == nil {
		return nil
	}
	out := make([]*goquery.Selection, 0, sel.Length())
	sel.Each(func(_ int, s *goquery.Selection) { out = append(out, s) })
	return out
}

func (e *XYQHikerEngine) selectOn(base *goquery.Selection, rule string) []*goquery.Selection {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	sel := jsoupSelectOn(base, rule)
	if sel == nil {
		return nil
	}
	out := make([]*goquery.Selection, 0, sel.Length())
	sel.Each(func(_ int, s *goquery.Selection) { out = append(out, s) })
	return out
}

func (e *XYQHikerEngine) extractOn(node *goquery.Selection, rule string) string {
	if strings.TrimSpace(rule) == "" {
		return ""
	}
	return jsoupExtractOn(node, rule)
}

func (e *XYQHikerEngine) newDoc(raw string) *goquery.Document {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(raw))
	return doc
}

func (e *XYQHikerEngine) fetch(ctx context.Context, rawURL string, headers HeaderMap) (string, error) {
	c := e.c
	if len(headers) > 0 {
		cp := NewClient(mergeMaps(c.headers, map[string]string(headers)))
		cp.hc = c.hc
		c = cp
	}
	return c.Get(ctx, rawURL)
}

func mergeMaps(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func (e *XYQHikerEngine) absURL(raw string) string {
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

func xyqExpandURL(tpl, cat string, page int) string {
	return strings.NewReplacer(
		"{cateId}", cat, "{catePg}", strconv.Itoa(page),
		"{area}", "", "{by}", "", "{class}", "", "{lang}", "",
		"{year}", "", "{letter}", "",
	).Replace(tpl)
}

// resolveCategoryURL 处理分类链接的 XYQHiker 边界语法：
//   - "[firstPage=xxx]" / "[firstPg=xxx]" 后缀：第 1 页跳转到指定地址
//     （常见于 React 站点，此时归档不带页码），第 2 页起用主链接拼 {catePg}。
//
// 返回展开后的 URL（相对地址仍由调用方转绝对）。
func (e *XYQHikerEngine) resolveCategoryURL(cat string, page int) string {
	tpl := e.rule.CategoryURL
	firstURL := ""
	if i := strings.Index(tpl, "[firstPage="); i >= 0 {
		rest := tpl[i+len("[firstPage="):]
		if j := strings.Index(rest, "]"); j >= 0 {
			firstURL = strings.TrimSpace(rest[:j])
			tpl = strings.TrimSpace(tpl[:i] + rest[j+1:])
		}
	} else if i := strings.Index(tpl, "[firstPg="); i >= 0 {
		rest := tpl[i+len("[firstPg="):]
		if j := strings.Index(rest, "]"); j >= 0 {
			firstURL = strings.TrimSpace(rest[:j])
			tpl = strings.TrimSpace(tpl[:i] + rest[j+1:])
		}
	}
	if firstURL != "" && page <= 1 {
		return xyqExpandURL(firstURL, cat, page)
	}
	return xyqExpandURL(tpl, cat, page)
}

func xyqReplaceSearch(tpl, wd string) string {
	return strings.NewReplacer(
		"{wd}", url.QueryEscape(wd),
		"{SearchPg}", "1",
	).Replace(tpl)
}

func reverseStrings(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// xyqReverseEnabled 判断「是否反转选集序列」是否为真值。
// 真实规则取值是 0/1/否/是 的混合（动漫巴士/七新影视/农民影视写 "1"），
// 不能只认「是」——它代表页面按最新在前降序，需要翻转为升序展示。
func xyqReverseEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "是", "y", "true":
		return true
	default:
		return false
	}
}

// withPrefixSuffix 给相对地址补上前缀/后缀，绝对地址原样返回。
// 站点页里图片/链接常是相对路径（如 /d/1.html）需要前缀补全；但若是绝对
// URL（CDN 直链），一律加前缀会拼出 "https://站https://cdn/..." 畸形地址。
func withPrefixSuffix(raw, prefix, suffix string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}
	return prefix + trimmed + suffix
}

var _ Engine = (*XYQHikerEngine)(nil)
