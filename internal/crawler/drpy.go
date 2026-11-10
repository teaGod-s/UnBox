package crawler

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func fillURL(tmpl, tid string, pg int) string {
	tmpl = strings.ReplaceAll(tmpl, "fyclass", tid)
	tmpl = strings.ReplaceAll(tmpl, "fypage", strconv.Itoa(pg))
	return strings.ReplaceAll(tmpl, "**", url.QueryEscape(tid))
}

// flPlaceholderRe 匹配 drpy2 筛选模板里的 {{fl.key}} / {{ fl.key }}（纯键形态）。
var flPlaceholderRe = regexp.MustCompile(`\{\{\s*fl\.([\p{L}\p{N}_]+)\s*\}\}`)

// flTemplateRe 匹配一个完整的 {{fl.xxx ...}} 占位块（含 jinja「或默认值」形态
// {{fl.year or "all"}} 与管道过滤器形态），供 renderFilterTemplate 逐块替换。
var flTemplateRe = regexp.MustCompile(`\{\{\s*fl\.[\p{L}\p{N}_]+\s*(?:\|\s*[^}]*)?(?:\s+or\s+[^}]*)?\s*\}\}`)

// flOrDefaultRe 抓 or 默认值块的键（组1）与值文本（组2，可能带引号）。
var flOrDefaultRe = regexp.MustCompile(`\{\{\s*fl\.([\p{L}\p{N}_]+)\s*.*?\s+or\s+([^}]*)\s*\}\}`)

// unquote strips surrounding single/double quotes from a jinja default value.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// applyFilterTemplate 复刻 drpy2 categoryParse 的 filter_url 展开：
//
//	if (rule.filter_url) {
//	  url 含 fyfilter ? 用 filter_url 替换 : 追加 filter_url
//	  url = jinja2(url, {fl: filter_def[tid] 合并用户选择})
//	}
//
// 这里只用 filter_def 里该分类的默认值渲染；带用户选择的走
// applyFilterTemplateWithSel。缺少的键按 jinja2 语义渲染成空串。不做这一步，
// 规则会把字面量 "fyfilter" 直接发到站点（例如 params=fyfilter），请求必然失败。
func applyFilterTemplate(path, tid string, rule *Rule) string {
	if rule == nil || strings.TrimSpace(rule.FilterURL) == "" {
		return path
	}
	return renderFilterTemplate(expandFilterURL(path, tid, rule), filterDefaults(rule, tid))
}

// applyFilterTemplateWithSel 在 filterDefaults 之上合并用户选择（选择覆盖默认值），
// 再渲染 {{fl.xxx}}。sel 为 nil 等价 applyFilterTemplate（只有默认值）。
func applyFilterTemplateWithSel(path, tid string, rule *Rule, sel map[string]string) string {
	if rule == nil || strings.TrimSpace(rule.FilterURL) == "" {
		return path
	}
	values := filterDefaults(rule, tid)
	for k, v := range sel {
		values[k] = v
	}
	return renderFilterTemplate(expandFilterURL(path, tid, rule), values)
}

// expandFilterURL 完成 fyfilter → filter_url 的替换（或按 & 拼接），不含模板渲染。
func expandFilterURL(path, tid string, rule *Rule) string {
	filterURL := strings.TrimSpace(rule.FilterURL)
	if filterURL == "" {
		return path
	}
	if strings.Contains(path, "fyfilter") {
		path = strings.Replace(path, "fyfilter", filterURL, 1)
	} else {
		if !strings.HasSuffix(path, "&") && !strings.HasPrefix(filterURL, "&") {
			path += "&"
		}
		path += filterURL
	}
	return strings.ReplaceAll(path, "fyclass", tid)
}

// filterDefaults 取出某个分类的筛选默认值。
func filterDefaults(rule *Rule, tid string) map[string]string {
	out := map[string]string{}
	if rule == nil || len(rule.FilterDef) == 0 {
		return out
	}
	raw, ok := rule.FilterDef[tid]
	if !ok {
		return out
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range values {
		out[key] = filterValueString(value)
	}
	return out
}

// filterValueString 把筛选默认值转成查询串里可用的文本。
func filterValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// renderFilterTemplate 把筛选模板里的 {{fl.key}} 替换成对应的筛选值。
//
// 支持两种 jinja 形态：
//   - 纯键 `{{fl.year}}`：键缺失渲染空串（与原行为一致）。
//   - 「或默认值」`{{fl.year or "all"}}`：键有值用值，键缺失用 or 后的默认值
//     （去掉引号）。某影视源等规则的 filter_url 写成 `year={{fl.year or "all"}}`，
//     不展开会把整段字面量带进请求 → 某影视源等站点返回空 JSON。
func renderFilterTemplate(tmpl string, values map[string]string) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}
	return flTemplateRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		// 纯键形态优先（避免把 or 默认值当普通模板误判）。
		if sub := flPlaceholderRe.FindStringSubmatch(match); len(sub) >= 2 {
			if v, ok := values[sub[1]]; ok && v != "" {
				return v
			}
			// 键缺失但有 or 默认值 → 用默认值。
			if or := flOrDefaultRe.FindStringSubmatch(match); len(or) >= 3 {
				return unquote(or[2])
			}
			return ""
		}
		// 带 or/管道的整块没被纯键正则吃下时，直接抓 or 默认值。
		if or := flOrDefaultRe.FindStringSubmatch(match); len(or) >= 3 {
			if v, ok := values[or[1]]; ok && v != "" {
				return v
			}
			return unquote(or[2])
		}
		return ""
	})
}
