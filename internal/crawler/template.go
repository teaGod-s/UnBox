package crawler

import (
	"encoding/json"
	"fmt"
	"net/url"
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
	for _, name := range []string{"homeVod", "home"} {
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
	return e.fetchVods(joinURL(rule.Host, path), rule)
}

func (e *Engine) VodCategory(tid string, pg int) ([]Vod, error) {
	for _, name := range []string{"categoryContent", "category", "categoryVod"} {
		if hasFunction(e, name) {
			if name == "categoryContent" {
				return e.callVods(name, e.vm.ToValue(tid), e.vm.ToValue(pg), e.vm.ToValue(false), e.vm.ToValue(nil))
			}
			return e.callVods(name, e.vm.ToValue(tid), e.vm.ToValue(pg))
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, err
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
				return e.callVods(name, e.vm.ToValue(wd), e.vm.ToValue(1), e.vm.ToValue(false))
			}
			return e.callVods(name, e.vm.ToValue(wd))
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, err
	}
	path := strings.ReplaceAll(rule.SearchURL, "{wd}", url.QueryEscape(wd))
	if !strings.Contains(path, url.QueryEscape(wd)) {
		path += url.QueryEscape(wd)
	}
	return e.fetchVods(joinURL(rule.Host, path), rule)
}

func (e *Engine) VodDetail(id string) (*Detail, error) {
	for _, name := range []string{"detailContent", "detail", "detailVod"} {
		if hasFunction(e, name) {
			value, err := e.Call(name, e.vm.ToValue(id))
			if err != nil {
				return nil, err
			}
			return exportDetail(e, value)
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, err
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
	var vods []Vod
	b, err := json.Marshal(value.Export())
	if err != nil {
		return nil, fmt.Errorf("爬虫列表返回非数组: %w", err)
	}
	if err := json.Unmarshal(b, &vods); err != nil {
		return nil, fmt.Errorf("爬虫列表返回非数组: %w", err)
	}
	return vods, nil
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

func exportDetail(e *Engine, value goja.Value) (*Detail, error) {
	var detail Detail
	b, err := json.Marshal(value.Export())
	if err != nil {
		return nil, fmt.Errorf("爬虫详情返回格式错误: %w", err)
	}
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
