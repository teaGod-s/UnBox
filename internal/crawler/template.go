package crawler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

// Rule returns the loaded declaration's rule object.
func (e *Engine) Rule() (*Rule, error) {
	value := e.vm.Get("rule")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, fmt.Errorf("爬虫未定义 rule 对象")
	}
	var rule Rule
	if err := e.vm.ExportTo(value, &rule); err != nil {
		return nil, fmt.Errorf("读取爬虫 rule 失败: %w", err)
	}
	return &rule, nil
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
		return nil, err
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
	for _, name := range []string{"categoryContent", "category", "categoryVod"} {
		if hasFunction(e, name) {
			return e.callVods(name, e.vm.ToValue(tid), e.vm.ToValue(strconv.Itoa(pg)), e.vm.ToValue(false), e.vm.ToValue(nil))
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, err
	}
	if rule.URL != "" {
		path := fillURL(rule.URL, tid, pg)
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
		return nil, err
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
		var envelope struct {
			URL string `json:"url"`
		}
		if err := decodeActionValue(value, &envelope); err != nil {
			return "", fmt.Errorf("爬虫播放返回格式错误: %w", err)
		}
		return envelope.URL, nil
	}
	rule, err := e.Rule()
	if err != nil {
		return "", err
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
		return nil, err
	}
	if rule.DetailURL != "" || e.inlineRule("二级") != "" {
		path := rule.DetailURL
		if path == "" {
			path = id
		}
		path = fillURL(path, id, 1)
		path = strings.ReplaceAll(path, "fyid", id)
		path = strings.ReplaceAll(path, "{id}", url.QueryEscape(id))
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
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
		Vod:        Vod{VodID: id, VodName: firstRule(doc, firstNonEmpty(rule.DetailNameSelector, rule.NameSelector)), VodPic: firstRule(doc, rule.PicSelector), VodRemarks: firstRule(doc, rule.RemarksSelector), TypeName: firstRule(doc, rule.TypeSelector)},
		VodContent: firstRule(doc, rule.DetailContentSelector), VodYear: firstRule(doc, rule.DetailYearSelector), VodArea: firstRule(doc, rule.DetailAreaSelector),
	}
	if detail.VodName == "" {
		detail.VodName = id
	}
	detail.VodPlayFrom, detail.VodPlayURL = parsePlay(doc, rule)
	return detail, nil
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
			vods = append(vods, Vod{VodID: id, VodName: name, VodPic: pic, TypeName: typeName, VodRemarks: remarks})
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
	if len(strings.TrimSpace(string(b))) > 0 && strings.TrimSpace(string(b))[0] == '[' {
		var details []Detail
		if err := json.Unmarshal(b, &details); err != nil || len(details) == 0 {
			return nil, fmt.Errorf("爬虫详情返回列表为空")
		}
		return &details[0], nil
	}
	var envelope struct {
		List []Detail `json:"list"`
	}
	if err := json.Unmarshal(b, &envelope); err == nil && len(envelope.List) > 0 {
		return &envelope.List[0], nil
	}
	var detail Detail
	if err := json.Unmarshal(b, &detail); err != nil {
		return nil, fmt.Errorf("爬虫详情返回格式错误: %w", err)
	}
	return &detail, nil
}

func joinURL(host, path string) string {
	if host == "" {
		return path
	}
	base, err := url.Parse(host)
	if err != nil {
		return path
	}
	ref, err := url.Parse(path)
	if err != nil {
		return path
	}
	return base.ResolveReference(ref).String()
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
