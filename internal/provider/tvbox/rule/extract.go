package rule

import (
	"html"
	"regexp"
	"strings"
)

var tagRe = regexp.MustCompile(`<[^>]*>`)

// Cut 提取 "A&&B" 规则中 A 与 B 之间的文本。
// spec 为空返回空串；没有 "&&" 时取标记之后到结尾。
func Cut(s, spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	prefix, suffix, ok := strings.Cut(spec, "&&")
	if !ok {
		if _, after, found := strings.Cut(s, prefix); found {
			return strings.TrimSpace(after)
		}
		return ""
	}
	_, rest, found := strings.Cut(s, prefix)
	if !found {
		return ""
	}
	if suffix == "" {
		return strings.TrimSpace(rest)
	}
	before, _, found := strings.Cut(rest, suffix)
	if !found {
		return ""
	}
	return strings.TrimSpace(before)
}

// CutList 按 "A&&B" 数组规则切分：以 A 分段，每段截到 B。
func CutList(s, spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	prefix, suffix, ok := strings.Cut(spec, "&&")
	if !ok {
		prefix, suffix = spec, ""
	}
	parts := strings.Split(s, prefix)
	if len(parts) <= 1 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		if suffix != "" {
			if before, _, found := strings.Cut(p, suffix); found {
				p = before
			}
		}
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Clean 去掉 HTML 标签并反转义实体。
func Clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}
