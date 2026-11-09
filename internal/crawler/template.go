package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

// Rule returns the loaded declaration's rule object.
//
// 导出时 goja 的 ExportTo 在遇到 rule 值里的动态对象（如某脚本把 headers/
// filter 设成运行时对象）会直接 panic（Could not convert *dynamicObject to
// primitive），而不是返回 error——会把整个进程打崩。这里用 recover 兜底，
// 并把导出先经 JSON 中转，类型不兼容时让 json/unmarshal 返回可读错误。
func (e *Engine) Rule() (rule *Rule, err error) {
	defer func() {
		if r := recover(); r != nil {
			rule, err = nil, fmt.Errorf("读取爬虫 rule 失败（rule 值含动态对象）: %v", r)
		}
	}()
	value := e.vm.Get("rule")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, fmt.Errorf("爬虫未定义 rule 对象")
	}
	exported := value.Export()
	b, err := json.Marshal(exported)
	if err != nil {
		return nil, fmt.Errorf("读取爬虫 rule 失败: %w", err)
	}
	var r Rule
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("读取爬虫 rule 失败: %w", err)
	}
	return &r, nil
}

func (e *Engine) VodHome() ([]Vod, error) {
	// homeVideoContent 是 FongMi 官方首页推荐；homeVod/home 是 dr_py 旧名。
	for _, name := range []string{"homeVideoContent", "homeVod", "home"} {
		if hasFunction(e, name) {
			return e.callVods(name)
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, noRuleError("首页", "homeContent/homeVod/home")
	}
	path := rule.HomeURL
	if path == "" {
		path = "/"
	}
	if rule.URL != "" || e.inlineRule("推荐") != "" || e.inlineRule("一级") != "" {
		if rule.URL != "" {
			path = fillURL(rule.URL, "", 1)
		}
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		kind := "推荐"
		if e.inlineRule(kind) == "" {
			kind = "一级"
		}
		return e.extractVods(kind, html, rule)
	}
	return e.fetchVods(joinURL(rule.Host, path), rule)
}

// VodClasses returns categories exposed by a FongMi homeContent action or declaration.
func (e *Engine) VodClasses() ([]Class, error) {
	// homeContent(filter) 是 FongMi 官方首页分类；home 是 dr_py 旧名。
	for _, name := range []string{"homeContent", "home"} {
		if !hasFunction(e, name) {
			continue
		}
		value, err := e.Call(name, e.vm.ToValue(false))
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Class []Class `json:"class"`
		}
		if err := decodeActionValue(value, &envelope); err != nil {
			return nil, fmt.Errorf("爬虫分类返回格式错误: %w", err)
		}
		return envelope.Class, nil
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, err
	}
	if rule.ClassParse != "" {
		path := rule.HomeURL
		if path == "" {
			path = "/"
		}
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return parseClasses(html, rule.ClassParse)
	}
	names, ids := strings.Split(rule.ClassName, "&"), strings.Split(rule.ClassURL, "&")
	classes := make([]Class, 0, len(names))
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id := name
		if i < len(ids) && strings.TrimSpace(ids[i]) != "" {
			id = strings.TrimSpace(ids[i])
		}
		classes = append(classes, Class{TypeID: id, TypeName: name})
	}
	return classes, nil
}

func (e *Engine) VodCategory(tid string, pg int) ([]Vod, error) {
	return e.VodCategoryExtend(tid, pg, nil)
}

// VodCategoryExtend 带用户选择的筛选值浏览分类；extend 为「筛选维度 key → 选中值」。
// 函数路径（categoryContent 等）把选择并入境 extend 对象；声明式路径用选择值
// 覆盖 filter_def 默认值后渲染 {{fl.xxx}}（applyFilterTemplateWithSel）。
func (e *Engine) VodCategoryExtend(tid string, pg int, extend map[string]string) ([]Vod, error) {
	for _, name := range []string{"categoryContent", "category", "categoryVod"} {
		if hasFunction(e, name) {
			// extend 必须是对象：真实脚本会直接读 extend.area / extend.year
			// （drpy2 语义里它是筛选值集合）；传 null 会让 category 抛
			// `Cannot read property 'area' of undefined`。
			ext := e.vm.NewObject()
			for k, v := range extend {
				_ = ext.Set(k, v)
			}
			return e.callVods(name, e.vm.ToValue(tid), e.vm.ToValue(strconv.Itoa(pg)), e.vm.ToValue(false), ext)
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, noRuleError("分类", "categoryContent/category/categoryVod")
	}
	if rule.URL != "" {
		path := fillURL(applyFilterTemplateWithSel(rule.URL, tid, rule, extend), tid, pg)
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return e.extractVods("一级", html, rule)
	}
	paths := strings.Split(rule.ClassURL, "&")
	path := tid
	if n, convErr := parsePositiveIndex(tid); convErr == nil && n >= 0 && n < len(paths) {
		path = paths[n]
	}
	if path == "" {
		path = "/"
	}
	if pg > 1 {
		path = addPage(path, pg)
	}
	return e.fetchVods(joinURL(rule.Host, path), rule)
}

func (e *Engine) VodSearch(wd string) ([]Vod, error) {
	for _, name := range []string{"searchContent", "search", "searchVod"} {
		if hasFunction(e, name) {
			if name == "searchContent" {
				// searchContent(key, quick)：quick=false 表示完整搜索。
				return e.callVods(name, e.vm.ToValue(wd), e.vm.ToValue(false))
			}
			return e.callVods(name, e.vm.ToValue(wd))
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, noRuleError("搜索", "searchContent/search/searchVod")
	}
	if rule.SearchURL != "" && (strings.Contains(rule.SearchURL, "**") || e.inlineRule("搜索") != "") {
		path := fillURL(rule.SearchURL, wd, 1)
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return e.extractVods("搜索", html, rule)
	}
	path := strings.ReplaceAll(rule.SearchURL, "{wd}", url.QueryEscape(wd))
	if !strings.Contains(path, url.QueryEscape(wd)) {
		path += url.QueryEscape(wd)
	}
	return e.fetchVods(joinURL(rule.Host, path), rule)
}

// VodPlay resolves a script-level playback action into a media URL.
func (e *Engine) VodPlay(flag, id string) (string, error) {
	// playerContent(flag, id, vipFlags) 是 FongMi 官方签名（3 参），
	// playContent/play/playVod 是 dr_py 旧名（2 参），都兼容。
	for _, name := range []string{"playerContent", "playContent", "play", "playVod"} {
		if !hasFunction(e, name) {
			continue
		}
		args := []goja.Value{e.vm.ToValue(flag), e.vm.ToValue(id)}
		if name == "playerContent" {
			args = append(args, e.vm.ToValue([]string{}))
		}
		value, err := e.Call(name, args...)
		if err != nil {
			return "", err
		}
		url, err := playURLFromEnvelope(value)
		if err != nil {
			return "", fmt.Errorf("爬虫播放返回格式错误: %w", err)
		}
		return url, nil
	}
	rule, err := e.Rule()
	if err != nil {
		return "", noRuleError("播放", "playerContent/playContent/play/playVod")
	}
	if strings.TrimSpace(rule.Lazy) != "" {
		return e.resolveLazy(flag, id)
	}
	if rule.PlayURL != "" {
		path := strings.ReplaceAll(fillURL(rule.PlayURL, id, 1), "fyid", id)
		path = strings.ReplaceAll(path, "{id}", url.QueryEscape(id))
		return joinURL(rule.Host, path), nil
	}
	return id, nil
}

func (e *Engine) VodDetail(id string) (*Detail, error) {
	for _, name := range []string{"detailContent", "detail", "detailVod"} {
		if hasFunction(e, name) {
			// detailContent(ids) 接收数组；detail/detailVod 旧名接收单个 id。
			var value goja.Value
			var err error
			if name == "detailContent" {
				value, err = e.Call(name, e.vm.ToValue([]string{id}))
			} else {
				value, err = e.Call(name, e.vm.ToValue(id))
			}
			if err != nil {
				return nil, err
			}
			return exportDetail(value)
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, noRuleError("详情", "detailContent/detail/detailVod")
	}
	if rule.DetailURL != "" || e.inlineRule("二级") != "" {
		fyclass, detailID := splitDetailID(id, rule)
		path := rule.DetailURL
		if path == "" {
			path = detailID
		}
		path = fillURL(path, fyclass, 1)
		path = strings.ReplaceAll(path, "fyid", detailID)
		path = strings.ReplaceAll(path, "{id}", url.QueryEscape(detailID))
		target := joinURL(rule.Host, path)
		inline := strings.TrimSpace(e.inlineRule("二级"))
		if strings.HasPrefix(inline, "js:") {
			return e.extractJSDetail(target, strings.TrimSpace(strings.TrimPrefix(inline, "js:")), rule, id)
		}
		html, fetchErr := e.fetchHTMLWithRule(target, rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return e.extractDetail(html, rule, id)
	}
	path := strings.ReplaceAll(rule.DetailURL, "{id}", url.QueryEscape(id))
	if !strings.Contains(path, url.QueryEscape(id)) {
		path += url.QueryEscape(id)
	}
	html, err := e.fetchHTML(joinURL(rule.Host, path))
	if err != nil {
		return nil, err
	}
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail := &Detail{
		Vod: Vod{
			VodID:      flexID(id),
			VodName:    firstRule(doc, firstNonEmpty(rule.DetailNameSelector, rule.NameSelector)),
			VodPic:     firstRule(doc, rule.PicSelector),
			VodRemarks: firstRule(doc, rule.RemarksSelector),
			TypeName:   firstRule(doc, rule.TypeSelector),
			VodContent: firstRule(doc, rule.DetailContentSelector),
		},
		VodYear: firstRule(doc, rule.DetailYearSelector),
		VodArea: firstRule(doc, rule.DetailAreaSelector),
	}
	if detail.VodName == "" {
		detail.VodName = id
	}
	detail.VodPlayFrom, detail.VodPlayURL = parsePlay(doc, rule)
	return detail, nil
}

// splitDetailID 拆 drpy 的 `分类$真实id` 形式 vod_id。
//
// drpy2 的 detail() 就是 `vod_url.split("$")`：第一段当 fyclass、第二段当真实 id。
// 列表规则里 `cat_id+en_id` 正是用 "$" 拼出来的（见 evalJSONIDExpr），
// 拆不开就会把「分类+id 粘在一起」的串填进 ?id=，站点直接拒绝。
// 没有 "$" 时退回规则里的首个分类，保持既有行为。
func splitDetailID(id string, rule *Rule) (fyclass, detailID string) {
	if i := strings.Index(id, "$"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return detailCategoryID(rule), id
}

func detailCategoryID(rule *Rule) string {
	if rule == nil {
		return ""
	}
	for _, id := range strings.Split(rule.ClassURL, "&") {
		if id = strings.TrimSpace(id); id != "" {
			return id
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func hasFunction(e *Engine, name string) bool {
	_, ok := goja.AssertFunction(e.vm.Get(name))
	return ok
}

func (e *Engine) callVods(name string, args ...goja.Value) ([]Vod, error) {
	value, err := e.Call(name, args...)
	if err != nil {
		return nil, err
	}
	b, err := actionJSON(value)
	if err != nil {
		return nil, fmt.Errorf("爬虫列表返回格式错误: %w", err)
	}
	if len(strings.TrimSpace(string(b))) > 0 && strings.TrimSpace(string(b))[0] == '[' {
		var vods []Vod
		if err := json.Unmarshal(b, &vods); err != nil {
			return nil, fmt.Errorf("爬虫列表返回格式错误: %w", err)
		}
		return vods, nil
	}
	var envelope struct {
		List []Vod `json:"list"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, fmt.Errorf("爬虫列表返回格式错误: %w", err)
	}
	return envelope.List, nil
}

func actionJSON(value goja.Value) ([]byte, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, fmt.Errorf("爬虫动作返回空值")
	}
	if raw, ok := value.Export().(string); ok {
		return []byte(raw), nil
	}
	return json.Marshal(value.Export())
}

func decodeActionValue(value goja.Value, target any) error {
	b, err := actionJSON(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

// playURLFromEnvelope 提取 play()/playerContent() 返回信封里的播放地址。
//
// 绝大多数规则返回 {url:"https://..."} 单字符串；音乐类规则（听海/库里等）
// 返回 {url:["FLAC无损","https://...","HQ高品质","https://...",...]} 的
// 多清晰度数组。数组形态按「标签/地址交替」约定，取第一个 http(s) 地址，
// 不能因数组无法解码为 string 而整体报错。
func playURLFromEnvelope(value goja.Value) (string, error) {
	b, err := actionJSON(value)
	if err != nil {
		return "", err
	}
	var envelope struct {
		URL json.RawMessage `json:"url"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(envelope.URL, &s); err == nil {
		return s, nil
	}
	var arr []string
	if err := json.Unmarshal(envelope.URL, &arr); err != nil {
		return "", fmt.Errorf("url 字段既不是字符串也不是数组")
	}
	for _, item := range arr {
		if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") {
			return item, nil
		}
	}
	// 数组里没有 http 地址时退化为第一个元素（规则可能只放了单地址元素）。
	if len(arr) > 0 {
		return arr[0], nil
	}
	return "", fmt.Errorf("play 响应 url 为空")
}

func (e *Engine) fetchVods(target string, rule *Rule) ([]Vod, error) {
	html, err := e.fetchHTML(target)
	if err != nil {
		return nil, err
	}
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	selector := rule.VodSelector
	if selector == "" {
		selector = ".item, .module-item, .stui-vodlist__box"
	}
	var vods []Vod
	doc.Find(selector).Each(func(_ int, item *goquery.Selection) {
		id := firstRule(itemToDocument(item), rule.IDSelector)
		name := firstRule(itemToDocument(item), rule.NameSelector)
		pic := firstRule(itemToDocument(item), rule.PicSelector)
		remarks := firstRule(itemToDocument(item), rule.RemarksSelector)
		typeName := firstRule(itemToDocument(item), rule.TypeSelector)
		if id == "" {
			id, _ = item.Find("a").First().Attr("href")
		}
		if name == "" {
			name = strings.TrimSpace(item.Text())
		}
		if id != "" || name != "" {
			vods = append(vods, Vod{VodID: flexID(id), VodName: name, VodPic: pic, TypeName: typeName, VodRemarks: remarks})
		}
	})
	return vods, nil
}

func (e *Engine) fetchHTML(target string) (string, error) {
	value, err := e.Call("req", e.vm.ToValue(target), e.vm.NewObject())
	if err != nil {
		return "", err
	}
	return value.ToObject(e.vm).Get("content").String(), nil
}

func (e *Engine) fetchHTMLWithRule(target string, rule *Rule) (string, error) {
	opts := e.vm.NewObject()
	if rule != nil {
		if len(rule.Headers) > 0 {
			headers := e.vm.NewObject()
			for key, value := range rule.Headers {
				_ = headers.Set(key, value)
			}
			_ = opts.Set("headers", headers)
		}
		if rule.Timeout > 0 {
			_ = opts.Set("timeout", rule.Timeout)
		}
	}
	value, err := e.Call("req", e.vm.ToValue(target), opts)
	if err != nil {
		return "", err
	}
	return value.ToObject(e.vm).Get("content").String(), nil
}

func firstRule(doc *goquery.Document, rules ...string) string {
	for _, rule := range rules {
		if rule == "" {
			continue
		}
		values := evalRule(doc, rule)
		if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func itemToDocument(item *goquery.Selection) *goquery.Document {
	return goquery.NewDocumentFromNode(item.Get(0))
}

func parsePlay(doc *goquery.Document, rule *Rule) (string, string) {
	if rule.PlaySelector == "" {
		return "", ""
	}
	items := doc.Find(rule.PlaySelector)
	names, urls := make([]string, 0, items.Length()), make([]string, 0, items.Length())
	items.Each(func(_ int, item *goquery.Selection) {
		name := strings.TrimSpace(item.Text())
		if rule.PlayNameSelector != "" {
			name = firstRule(itemToDocument(item), rule.PlayNameSelector)
		}
		playURL := ""
		if rule.PlayURLSelector != "" {
			playURL = firstRule(itemToDocument(item), rule.PlayURLSelector)
		}
		if playURL == "" {
			playURL, _ = item.Attr("href")
		}
		names = append(names, name)
		urls = append(urls, playURL)
	})
	if len(urls) == 0 {
		return "", ""
	}
	parts := make([]string, len(urls))
	for i := range urls {
		parts[i] = names[i] + "$" + urls[i]
	}
	return "线路", strings.Join(parts, "#")
}

func exportDetail(value goja.Value) (*Detail, error) {
	b, err := actionJSON(value)
	if err != nil {
		return nil, fmt.Errorf("爬虫详情返回格式错误: %w", err)
	}
	detail, flow := lenientDetailFromJSON(b)
	if flow != "" {
		return nil, fmt.Errorf("爬虫详情返回格式错误: %s", flow)
	}
	return detail, nil
}

// lenientDetailFromJSON 把详情 JSON 解成 Detail，兼容三种信封形态：
// 数组、{list:[…]}、字段直铺。JS 脚本（二级 js:）常把数字直接赋给字符串字段
// （如博看听书 `VOD.vod_name = data.list[0].id`，id=5690048），直接 json.Unmarshal
// 到 Detail 的 string 字段会报 "cannot unmarshal number"。JS 语义里 number 赋给
// string 字段本应转成十进制字符串，这里先递归把数字值规范化再解码。
// 返回 flow 非空表示解析失败（错误说明）。
func lenientDetailFromJSON(b []byte) (*Detail, string) {
	if len(bytes.TrimSpace(b)) > 0 && bytes.TrimSpace(b)[0] == '[' {
		var details []Detail
		if err := json.Unmarshal(coerceDetailNumbers(b), &details); err != nil || len(details) == 0 {
			if err != nil {
				return nil, err.Error()
			}
			return nil, "爬虫详情返回列表为空"
		}
		return &details[0], ""
	}
	var envelope struct {
		List []Detail `json:"list"`
	}
	if err := json.Unmarshal(coerceDetailNumbers(b), &envelope); err == nil && len(envelope.List) > 0 {
		return &envelope.List[0], ""
	}
	var detail Detail
	if err := json.Unmarshal(coerceDetailNumbers(b), &detail); err != nil {
		return nil, err.Error()
	}
	return &detail, ""
}

// coerceDetailNumbers 把 JSON 里所有 JSON 数字转成字符串值（保持十进制，无科学计数法）。
//
// 详情字段在 JS 语义里基本是字符串；脚本给字符串字段赋数字（博看听书 id）时，
// 规范化后 json.Unmarshal 到 string 字段才能成功。整数字符串不经过 float64 中转，
// 直接用 []byte 文本，避免 9000000000000 这类大整数丢精度。
func coerceDetailNumbers(b []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return b // 不是合法 JSON，让原调用方报错
	}
	coerced := coerceJSONValueNumbers(decoded)
	out, err := json.Marshal(coerced)
	if err != nil {
		return b
	}
	return out
}

func coerceJSONValueNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = coerceJSONValueNumbers(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = coerceJSONValueNumbers(child)
		}
		return t
	case json.Number:
		return t.String()
	default:
		return v
	}
}

func joinURL(host, path string) string {
	if host == "" {
		return path
	}
	base, err := url.Parse(host)
	if err != nil {
		// 规则 host 可能是 URL 编码混淆形式（芒果 `https://pianku.api.%6d%67%74%76.com`，
		// `%6d%67%74%76`=mgtv）：Go 的 url.Parse 对 host 段的 %XX 报 invalid URL
		// escape。此时把 host 段解码后再解析，否则解析失败返回相对路径，请求会被
		// resolveRequestURL 打到脚本基址 → 站点反爬/空响应。
		if decoded, derr := decodeHostEscapes(host); derr == nil {
			base, err = url.Parse(decoded)
		}
		if err != nil {
			return path
		}
	}
	ref, err := url.Parse(path)
	if err != nil {
		return path
	}
	return base.ResolveReference(ref).String()
}

// decodeHostEscapes 把 URL 中主机段的 %XX 转义（%6d%67%74%76 → mgtv）解码。
// 只改「//host 之后、路径之前」的字符，避免动 query/path 里的合法转义。
func decodeHostEscapes(raw string) (string, error) {
	// 拆 scheme://authority
	rest := raw
	scheme := ""
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme = raw[:i+3]
		rest = raw[i+3:]
	}
	authorityEnd := strings.IndexAny(rest, "/?#")
	authority := rest
	tail := ""
	if authorityEnd >= 0 {
		authority = rest[:authorityEnd]
		tail = rest[authorityEnd:]
	}
	decoded, err := url.PathUnescape(authority)
	if err != nil {
		return "", err
	}
	return scheme + decoded + tail, nil
}

func addPage(path string, pg int) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%spage=%d", path, sep, pg)
}

func parsePositiveIndex(value string) (int, error) {
	var n int
	_, err := fmt.Sscanf(value, "%d", &n)
	return n, err
}

// noRuleError 在脚本既没有可调用的动作函数、也没有声明式 rule 时给出可定位的错误。
// 单独报「爬虫未定义 rule 对象」分不清是脚本缺动作还是配置写错了站点类型。
func noRuleError(action, actions string) error {
	return fmt.Errorf("该站点脚本没有提供%s动作（%s），也没有声明式 rule 对象：可能是脚本改版或站点类型配置错误", action, actions)
}
