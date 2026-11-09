package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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

// extractDetail interprets a dr_py detail rule. Inline json:/js: rules take
// precedence; declaration/template selectors remain the fallback.
func (e *Engine) extractDetail(html string, rule *Rule, id string) (*Detail, error) {
	inline := strings.TrimSpace(e.inlineRule("二级"))
	switch {
	case strings.HasPrefix(inline, "json:"):
		return extractJSONDetail(html, strings.TrimPrefix(inline, "json:"), id)
	case strings.HasPrefix(inline, "js:"):
		return e.extractJSDetail(html, strings.TrimPrefix(inline, "js:"), rule, id)
	}

	effective := Rule{}
	if rule != nil {
		effective = *rule
	}
	applyMubanDetail(&effective, e.readMuban())
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("详情页面解析失败: %w", err)
	}
	detail := &Detail{Vod: Vod{VodID: flexID(id)}}
	detail.Vod.VodName = firstNonEmpty(firstRule(doc, effective.DetailNameSelector), firstRule(doc, effective.NameSelector))
	detail.Vod.VodPic = firstRule(doc, effective.PicSelector)
	detail.Vod.VodRemarks = firstRule(doc, effective.RemarksSelector)
	detail.Vod.TypeName = firstRule(doc, effective.TypeSelector)
	detail.VodContent = firstRule(doc, effective.DetailContentSelector)
	detail.VodYear = firstRule(doc, effective.DetailYearSelector)
	detail.VodArea = firstRule(doc, effective.DetailAreaSelector)
	detail.VodPlayFrom, detail.VodPlayURL = parsePlay(doc, &effective)
	return detail, nil
}

func extractJSONDetail(raw, rule, id string) (*Detail, error) {
	parts := strings.Split(strings.TrimSpace(rule), ";")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("json 详情规则格式错误")
	}
	if looksLikeHTML([]byte(raw)) {
		return nil, fmt.Errorf("json 详情响应解析失败: 站点返回了网页而不是 JSON（开头: %s）", responseHead(raw))
	}
	var document any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return nil, fmt.Errorf("json 详情响应解析失败: %w", err)
	}
	value, ok := navigateJSON(document, parts[0])
	if !ok {
		return nil, fmt.Errorf("json 详情路径不存在: %s（响应开头: %s）", parts[0], responseHead(raw))
	}
	obj, ok := value.(map[string]any)
	if !ok {
		if items, isArray := value.([]any); isArray && len(items) > 0 {
			obj, ok = items[0].(map[string]any)
		}
	}
	if !ok {
		return nil, fmt.Errorf("json 详情路径不是对象: %s", parts[0])
	}
	detail := &Detail{Vod: Vod{VodID: flexID(id)}}
	for _, field := range parts[1:] {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		setDetailField(detail, field, evalJSONExpr(obj, field))
	}
	return detail, nil
}

func (e *Engine) extractJSDetail(html, script string, rule *Rule, id string) (*Detail, error) {
	if e == nil || e.vm == nil {
		return nil, fmt.Errorf("爬虫引擎未初始化")
	}
	_ = e.vm.Set("input", html)
	vod := e.vm.NewObject()
	_ = vod.Set("vod_id", id)
	_ = e.vm.Set("VOD", vod)
	e.setFetchParams(rule)
	host := ""
	if rule != nil {
		host = rule.Host
	}
	_ = e.vm.Set("urljoin2", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return e.vm.ToValue("")
		}
		base := host
		path := call.Argument(0).String()
		if len(call.Arguments) > 1 {
			base = call.Argument(0).String()
			path = call.Argument(1).String()
		}
		return e.vm.ToValue(joinURL(base, path))
	})
	_, scopedVOD, err := e.runRuleScript(script)
	if err != nil {
		return nil, fmt.Errorf("js 详情规则执行失败: %w", err)
	}
	// 脚本里 `var VOD = {...}` 只活在函数作用域，优先用返回值；否则读全局。
	if scopedVOD != nil && !goja.IsUndefined(scopedVOD) && !goja.IsNull(scopedVOD) {
		_ = e.vm.Set("VOD", scopedVOD)
	}
	detail, err := exportDetail(e.vm.Get("VOD"))
	if err != nil {
		return nil, err
	}
	if detail.VodID == "" {
		detail.VodID = flexID(id)
	}
	return detail, nil
}

func setDetailField(detail *Detail, field, value string) {
	switch canonicalDetailField(field) {
	case "title", "name", "vod_name":
		detail.VodName = value
	case "id", "url", "vod_id":
		detail.VodID = flexID(value)
	case "cover", "pic", "image", "vod_pic":
		detail.VodPic = value
	case "description", "desc", "content", "vod_content":
		detail.VodContent = value
	case "year", "vod_year":
		detail.VodYear = value
	case "area", "vod_area":
		detail.VodArea = value
	case "cat_name", "type_name", "type":
		detail.TypeName = value
	case "remarks", "remark", "vod_remarks":
		detail.VodRemarks = value
	case "play_from", "vod_play_from":
		detail.VodPlayFrom = value
	case "play_url", "vod_play_url":
		detail.VodPlayURL = value
	}
}

func canonicalDetailField(expr string) string {
	for _, part := range strings.Split(expr, "+") {
		for _, candidate := range strings.Split(part, "||") {
			name := strings.ToLower(strings.TrimSpace(candidate))
			switch name {
			case "title", "name", "vod_name", "id", "url", "vod_id", "cover", "pic", "image", "vod_pic", "description", "desc", "content", "vod_content", "year", "vod_year", "area", "vod_area", "cat_name", "type_name", "type", "remarks", "remark", "vod_remarks", "play_from", "vod_play_from", "play_url", "vod_play_url":
				return name
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(expr))
}

func applyMubanDetail(rule *Rule, values map[string]any) {
	if rule == nil {
		return
	}
	for key, raw := range values {
		if !strings.Contains(key, ".二级.") {
			continue
		}
		field := key[strings.LastIndexByte(key, '.')+1:]
		value := jsonScalarString(raw)
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "title", "name":
			rule.DetailNameSelector = value
		case "desc", "description", "content":
			rule.DetailContentSelector = value
		case "pic", "cover", "image":
			rule.PicSelector = value
		case "year":
			rule.DetailYearSelector = value
		case "area":
			rule.DetailAreaSelector = value
		case "type", "type_name":
			rule.TypeSelector = value
		case "remarks", "remark":
			rule.RemarksSelector = value
		case "play", "play_selector":
			rule.PlaySelector = value
		case "play_name", "play_name_selector":
			rule.PlayNameSelector = value
		case "play_url", "play_url_selector":
			rule.PlayURLSelector = value
		}
	}
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
	// 脚本可能把规则字段设为对象（如 `搜索: muban.首图2.搜索2`，值是 Go 侧
	// NewDynamicObject）。对象不是内联规则；且这类对象没有 Object.prototype，
	// 对它调 String() 会 panic（Could not convert *goja.dynamicObject to
	// primitive），全站搜索时直接闪退。这里安全回退为空串，走 HTML/muban 路径。
	if _, isObject := inline.(*goja.Object); isObject {
		return ""
	}
	return inline.String()
}

func extractJSONVods(raw, rule string) ([]Vod, error) {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(rule, "json:")), ";")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("json 列表规则格式错误")
	}
	if looksLikeHTML([]byte(raw)) {
		return nil, fmt.Errorf("json 列表响应解析失败: 站点返回了网页而不是 JSON（开头: %s）", responseHead(raw))
	}
	var document any
	if err := json.Unmarshal(stripJSONP([]byte(raw)), &document); err != nil {
		return nil, fmt.Errorf("json 列表响应解析失败: %w", err)
	}
	value, ok := navigateJSON(document, parts[0])
	if !ok {
		// 站点会间歇性返回「外层结构在、列表键整个缺失」的空响应
		//（360影视就时不时回 {"longData":{}}）：这不是规则写错，
		// 按「没有结果」处理，别把一次空响应报成搜索失败。
		if jsonParentExists(document, parts[0]) {
			return nil, nil
		}
		return nil, fmt.Errorf("json 列表路径不存在: %s（响应开头: %s）", parts[0], responseHead(raw))
	}
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("json 列表路径不是数组: %s（响应开头: %s）", parts[0], responseHead(raw))
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
			// 链接/id 字段：drpy2 用 links.join("$") 连接 `a+b`，
			// 详情再按 "$" 拆成 fyclass + 真实 id（见 splitDetailID）。
			if isJSONIDField(name) {
				value = evalJSONIDExpr(obj, name)
			}
			setVodField(&vod, field, value)
		}
		if vod.VodID != "" || vod.VodName != "" {
			vods = append(vods, vod)
		}
	}
	return vods, nil
}

// stripJSONP 剥离 JSONP 外壳（`callback({...})` → `{...}`），原样返回非 JSONP。
//
// 网易公版影像等站点的 `.jsonp` 接口返回 `_callback({...})` 包裹体：
// 直接 json.Unmarshal 报 "invalid character '_'"，但参考 TVBox 引擎会剥掉
// callback 名与首尾括号再按 json 规则取字段。仅当整串以 `名字(` 开头且以 `)` 结尾、
// 中间是可解析 JSON 时才剥；纯 JSON（`{`/`[` 开头）与网页原样放行。
func stripJSONP(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] == '{' || trimmed[0] == '[' {
		return raw
	}
	open := bytes.IndexByte(trimmed, '(')
	if open <= 0 {
		return raw
	}
	// 尾部剥掉可选分号（网易公版真实响应 `_callback({...});`），保留右括号。
	end := bytes.LastIndexByte(trimmed, ')')
	if end <= open {
		return raw
	}
	inner := bytes.TrimSpace(trimmed[open+1 : end])
	if len(inner) == 0 || (inner[0] != '{' && inner[0] != '[') {
		return raw
	}
	// 只有内层能解析成 JSON 时才剥，避免把含糊串误改坏。
	if json.Valid(inner) {
		return inner
	}
	return raw
}

// responseHead 截取响应开头一小段，附在「路径不存在」类错误里。
//
// 这类错误绝大多数是因为站点返回了错误 JSON（如 {"code":500,"msg":"参数错误"}）
// 或干脆返回了网页；不带片段时只能靠猜。
func responseHead(raw string) string {
	const max = 120
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > max {
		return trimmed[:max] + "…"
	}
	return trimmed
}

// jsonParentExists 判断列表路径的上一级是否解析得出来。
// 用来区分「规则路径写错」（上级也不存在）和「站点返回了空列表」（只有叶子缺失）。
func jsonParentExists(document any, path string) bool {
	index := strings.LastIndex(path, ".")
	if index <= 0 {
		return false
	}
	_, ok := navigateJSON(document, path[:index])
	return ok
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

// isJSONIDField 判断规则字段是不是「链接/id」。drpy2 按位置（第 5 段）判断，
// 这里按字段名判断：本引擎同时支持非标准排序与别名，按名字更稳。
func isJSONIDField(expr string) bool {
	switch canonicalVodField(expr) {
	case "id", "url", "vod_id", "cat_id", "en_id", "play_part_id", "playpartid", "vid":
		return true
	}
	return false
}

// evalJSONIDExpr 复刻 drpy2 的 id 字段：`a+b` 按 "$" 连接（links.join("$")），
// 而不是裸拼接。详情那一步再把 "$" 前一段当分类、后一段当真实 id。
// 360影视这类规则靠它把搜索结果各自的分类带进详情请求。
func evalJSONIDExpr(obj map[string]any, expr string) string {
	parts := strings.Split(expr, "+")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, evalJSONExpr(obj, part))
	}
	return strings.Join(values, "$")
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
		// JSON 整数 id（如 3331822）经 json.Unmarshal 会解析成 float64，
		// 不能直接 %v——大整数会输出科学计数法（3.331822e+06），详情 URL
		// 直接失效（博看听书实际线上根因）。整数按十进制输出，真小数保留。
		if v == math.Trunc(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1<<53 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
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
	case "titletxt", "titlealias":
		vod.VodName = value
	case "cover", "pic", "image", "vod_pic", "cover_pic", "coverpic":
		vod.VodPic = value
	case "id", "url", "vod_id":
		vod.VodID = flexID(value)
	case "cat_id", "en_id", "play_part_id", "playpartid", "vid":
		vod.VodID = flexID(value)
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
			case "title", "name", "vod_name", "titletxt", "titlealias", "cover", "pic", "image", "vod_pic", "cover_pic", "coverpic", "id", "url", "vod_id", "cat_id", "en_id", "play_part_id", "playpartid", "vid", "description", "desc", "content", "vod_content", "cat_name", "type_name", "type", "remarks", "remark", "vod_remarks":
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
			vods = append(vods, Vod{VodID: flexID(id), VodName: name, VodPic: pic, TypeName: typeName, VodRemarks: remarks})
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
