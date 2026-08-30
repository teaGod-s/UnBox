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
	if len(parts) != 4 {
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
	selection := selectClassEntries(doc, selectorRule)

	var re *regexp.Regexp
	if strings.TrimSpace(idPattern) != "" {
		re, err = regexp.Compile(strings.TrimSpace(idPattern))
		if err != nil {
			return nil, fmt.Errorf("class_parse id 正则无效: %w", err)
		}
	}
	classes := make([]Class, 0, len(entries))
	for i := range entries {
		name, id := "", ""
		if i < selection.Length() {
			item := selection.Eq(i)
			itemDoc := itemToDocument(item)
			if values := evalRule(itemDoc, normalizeClassRule(ruleForEntry(nameRule, item))); len(values) > 0 {
				name = strings.TrimSpace(values[0])
			}
			if values := evalRule(itemDoc, normalizeClassRule(ruleForEntry(idRule, item))); len(values) > 0 {
				id = strings.TrimSpace(values[0])
			}
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

func selectClassEntries(doc *goquery.Document, rule string) *goquery.Selection {
	parts := strings.Split(rule, "&&")
	sel := doc.Selection
	for _, raw := range parts {
		seg := strings.TrimSpace(raw)
		if seg == "" || seg == "Text" || seg == "Text()" || seg == "Html" || seg == "Html()" || seg == "href" || seg == "src" || strings.HasPrefix(seg, "attr(") {
			break
		}
		sel = sel.Find(seg)
	}
	return sel
}

func ruleForEntry(rule string, item *goquery.Selection) string {
	if len(item.Nodes) == 0 {
		return rule
	}
	parts := strings.Split(rule, "&&")
	for i, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), item.Nodes[0].Data) {
			return strings.Join(parts[i+1:], "&&")
		}
	}
	return rule
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
