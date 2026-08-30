package crawler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// parseClasses parses drpy's class_parse declaration:
// selector;name rule;id rule;id extraction regexp.
func parseClasses(html, classParse string) ([]Class, error) {
	parts := strings.Split(classParse, ";")
	if len(parts) < 4 {
		return nil, fmt.Errorf("class_parse 需要四段规则")
	}
	selectorRule, nameRule, idRule, idPattern := parts[0], parts[1], parts[2], parts[3]
	if strings.TrimSpace(selectorRule) == "" {
		return nil, fmt.Errorf("class_parse 选择器不能为空")
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("解析分类页面失败: %w", err)
	}
	entries := evalRule(doc, normalizeClassRule(selectorRule))
	names := evalRule(doc, normalizeClassRule(nameRule))
	ids := evalRule(doc, normalizeClassRule(idRule))

	var re *regexp.Regexp
	if strings.TrimSpace(idPattern) != "" {
		re, err = regexp.Compile(strings.TrimSpace(idPattern))
		if err != nil {
			return nil, fmt.Errorf("class_parse id 正则无效: %w", err)
		}
	}
	classes := make([]Class, 0, len(entries))
	for i := range entries {
		name := ""
		if i < len(names) {
			name = strings.TrimSpace(names[i])
		}
		id := ""
		if i < len(ids) {
			id = strings.TrimSpace(ids[i])
		}
		if re != nil {
			matches := re.FindStringSubmatch(id)
			if len(matches) > 1 {
				id = matches[1]
			} else if len(matches) == 0 {
				id = ""
			}
		}
		classes = append(classes, Class{TypeID: id, TypeName: name})
	}
	return classes, nil
}

// drpy commonly omits parentheses for terminal Text/Html operators.
func normalizeClassRule(rule string) string {
	parts := strings.Split(rule, "&&")
	for i, part := range parts {
		switch strings.TrimSpace(part) {
		case "Text":
			parts[i] = strings.Replace(part, "Text", "Text()", 1)
		case "Html":
			parts[i] = strings.Replace(part, "Html", "Html()", 1)
		}
	}
	return strings.Join(parts, "&&")
}
