package crawler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

// extractVods interprets a dr_py list rule. Inline json: rules are read from
// the loaded rule object; HTML selectors remain the fallback for muban rules.
func (e *Engine) extractVods(kind string, html string, rule *Rule) ([]Vod, error) {
	inline := e.inlineRule(kind)
	if strings.HasPrefix(strings.TrimSpace(inline), "json:") {
		return extractJSONVods(html, strings.TrimSpace(inline))
	}

	effective := Rule{}
	if rule != nil {
		effective = *rule
	}
	applyMubanList(&effective, kind, e.readMuban())
	return extractHTMLVods(html, &effective), nil
}

func (e *Engine) inlineRule(kind string) string {
	if e == nil || e.vm == nil {
		return ""
	}
	value := e.vm.Get("rule")
	if value == nil {
		return ""
	}
	obj := value.ToObject(e.vm)
	inline := obj.Get(kind)
	if inline == nil || goja.IsUndefined(inline) || goja.IsNull(inline) {
		return ""
	}
	return inline.String()
}

func extractJSONVods(raw, rule string) ([]Vod, error) {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(rule, "json:")), ";")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("json 列表规则格式错误")
	}
	var document any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return nil, fmt.Errorf("json 列表响应解析失败: %w", err)
	}
	value, ok := navigateJSON(document, parts[0])
	if !ok {
		return nil, fmt.Errorf("json 列表路径不存在: %s", parts[0])
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("json 列表路径不是数组: %s", parts[0])
	}

	vods := make([]Vod, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		var vod Vod
		for _, field := range parts[1:] {
			name := strings.TrimSpace(field)
			if name == "" {
				continue
			}
			value := evalJSONExpr(obj, name)
			setVodField(&vod, field, value)
		}
		if vod.VodID != "" || vod.VodName != "" {
			vods = append(vods, vod)
		}
	}
	return vods, nil
}

func navigateJSON(value any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return value, true
	}
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = obj[segment]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func evalJSONExpr(obj map[string]any, expr string) string {
	parts := strings.Split(expr, "+")
	var out strings.Builder
	for _, part := range parts {
		value := ""
		for _, candidate := range strings.Split(part, "||") {
			if v, ok := navigateJSON(obj, strings.TrimSpace(candidate)); ok {
				value = jsonScalarString(v)
				if value != "" {
					break
				}
			}
		}
		out.WriteString(value)
	}
	return out.String()
}

func jsonScalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func setVodField(vod *Vod, field, value string) {
	field = canonicalVodField(field)
	switch field {
	case "title", "name", "vod_name":
		vod.VodName = value
	case "cover", "pic", "image", "vod_pic":
		vod.VodPic = value
	case "id", "url", "vod_id":
		vod.VodID = value
	case "description", "desc", "content", "vod_content":
		vod.VodContent = value
	case "cat_name", "type_name", "type":
		vod.TypeName = value
	case "remarks", "remark", "vod_remarks":
		vod.VodRemarks = value
	}
}

func canonicalVodField(expr string) string {
	for _, part := range strings.Split(expr, "+") {
		for _, candidate := range strings.Split(part, "||") {
			name := strings.ToLower(strings.TrimSpace(candidate))
			switch name {
			case "title", "name", "vod_name", "cover", "pic", "image", "vod_pic", "id", "url", "vod_id", "description", "desc", "content", "vod_content", "cat_name", "type_name", "type", "remarks", "remark", "vod_remarks":
				return name
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(expr))
}

func extractHTMLVods(html string, rule *Rule) []Vod {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	selector := rule.VodSelector
	if selector == "" {
		selector = ".item, .module-item, .stui-vodlist__box"
	}
	vods := make([]Vod, 0)
	doc.Find(selector).Each(func(_ int, item *goquery.Selection) {
		itemDoc := itemToDocument(item)
		id := firstRule(itemDoc, rule.IDSelector)
		name := firstRule(itemDoc, rule.NameSelector)
		pic := firstRule(itemDoc, rule.PicSelector)
		remarks := firstRule(itemDoc, rule.RemarksSelector)
		typeName := firstRule(itemDoc, rule.TypeSelector)
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
	return vods
}

func applyMubanList(rule *Rule, kind string, values map[string]any) {
	if rule == nil {
		return
	}
	for key, raw := range values {
		if strings.Contains(key, "."+kind+".") {
			field := key[strings.LastIndexByte(key, '.')+1:]
			applyMubanField(rule, field, raw)
			continue
		}
		if !strings.HasSuffix(key, "."+kind) {
			continue
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for field, value := range obj {
			applyMubanField(rule, field, value)
		}
	}
}

func applyMubanField(rule *Rule, field string, raw any) {
	value := jsonScalarString(raw)
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "vod", "vod_selector", "selector", "list":
		rule.VodSelector = value
	case "name", "title", "name_selector":
		rule.NameSelector = value
	case "pic", "cover", "pic_selector":
		rule.PicSelector = value
	case "id", "id_selector":
		rule.IDSelector = value
	case "type", "type_selector":
		rule.TypeSelector = value
	case "remarks", "remark", "remarks_selector":
		rule.RemarksSelector = value
	}
}
