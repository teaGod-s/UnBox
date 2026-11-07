package rule

import (
	"html"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// jsoup 选择器引擎：实现 XYQHiker 规则用到的 JSoup 链式子集。
//
// 规则形如 ".module-items&&.module-item:not(:matches(饭太硬|公众号))"，
// 以 && 分段，每段是一个选择器步或取值步：
//
//   - 选择器步：如 .module-item-pic、img、a。支持 goquery 的 CSS 子集，
//     以及 XYQHiker 常用的 :matches(regex)（按元素文本过滤）与
//     :not(:matches(regex))。支持 ",N" 索引后缀，N 取第 N 个命中
//     （负数从末尾数，-1 = 最后）。
//   - 取值步：Text（纯文本）、Html（外部 HTML）、title/href/data-src
//     等属性名（取该属性）。缺省时返回元素文本。
//
// 每步作用于上一步的每个命中节点（第一部的起点是 document）。
// jsoupExtractFirst 返回全部最终值（每个起点节点一个值）。

var jsoupIndexSuffixRe = regexp.MustCompile(`^(.*),(-?\d+)$`)

// noJSoupIndex 表示选择器没有 ",N" 索引，取全部命中。
// 用独立哨兵值避免与真正的索引 -1（最后一项）混淆。
const noJSoupIndex = int(-1 << 30)

// jsoupKnownAttrs 是 XYQHiker 规则常用的取值属性名。
// 其余段一律按选择器处理。
var jsoupKnownAttrs = map[string]bool{
	"title": true, "href": true, "src": true, "data-src": true,
	"data-original": true, "data-screenshot": true, "data-lazy": true,
	"data-echo": true, "alt": true, "value": true, "content": true,
	"id": true, "name": true,
}

// stepIsExtract 判断整段（含 || 备选）是否为取值步。
func stepIsExtract(step string) bool {
	for _, alt := range strings.Split(step, "||") {
		if isExtractAlt(strings.TrimSpace(alt)) {
			return true
		}
	}
	return false
}

// jsoupExtractFirst 在 doc 上执行一条链式规则，返回全部命中值。
func jsoupExtractFirst(doc *goquery.Document, rule string) []string {
	return jsoupExtractChain(doc.Selection, rule)
}

// jsoupSelectNodes 执行链式规则并停在最后的选择器步，返回命中节点集合
// （供数组规则使用：取回列表项节点后逐节点再取字段）。
// 链中含取值步时返回 nil（此时用 jsoupExtractChain）。
func jsoupSelectNodes(doc *goquery.Document, rule string) *goquery.Selection {
	return jsoupSelectOn(doc.Selection, rule)
}

// jsoupSelectOn 与 jsoupSelectNodes 相同，但起点是任意节点集合。
func jsoupSelectOn(base *goquery.Selection, rule string) *goquery.Selection {
	if base == nil || strings.TrimSpace(rule) == "" {
		return nil
	}
	nodes := base
	for _, raw := range strings.Split(rule, "&&") {
		step := strings.TrimSpace(raw)
		if step == "" {
			continue
		}
		if stepIsExtract(step) {
			return nil
		}
		sel, index := stripIndex(step)
		nodes = findNodes(nodes, sel, index)
		if nodes == nil || nodes.Length() == 0 {
			return nil
		}
	}
	return nodes
}

// jsoupExtractOn 在单个列表项节点上按字段规则提取值（返回首个命中）。
// 规则与 jsoupExtractChain 相同，但起点是 node 而非整个文档。
func jsoupExtractOn(node *goquery.Selection, rule string) string {
	vals := jsoupExtractChain(node, rule)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// jsoupExtractChain 在 base 节点上执行链式规则。values 为最终取值时返回；
// 返回 (nil, values) 后上层可直接取用。
func jsoupExtractChain(base *goquery.Selection, rule string) []string {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	steps := strings.Split(rule, "&&")
	nodes := base
	var values []string
	for _, raw := range steps {
		step := strings.TrimSpace(raw)
		if step == "" {
			continue
		}
		nodes, values = jsoupStep(nodes, step)
		if values != nil {
			return values
		}
		if nodes == nil || nodes.Length() == 0 {
			return nil
		}
	}
	if values != nil {
		return values
	}
	// 无取值步：返回每个命中元素的文本。
	out := make([]string, 0, nodes.Length())
	nodes.Each(func(_ int, s *goquery.Selection) {
		out = append(out, strings.TrimSpace(s.Text()))
	})
	return out
}

// jsoupStep 执行一个选择器步或取值步，返回后续步的起点节点（或取值结果）。
// 步内可用 "||" 连接多个备选（如 img&&data-src||src、.badge||.ribbon）：
// 取值步按「逐节点」回退（每个节点取第一个非空值），选择器步按「整组」回退
// （第一个有命中的选择器胜出）。取值步可用 "!前缀" 去掉结果里的固定前缀。
func jsoupStep(base *goquery.Selection, step string) (*goquery.Selection, []string) {
	alts := strings.Split(step, "||")
	hasExtract := false
	for _, a := range alts {
		if isExtractAlt(strings.TrimSpace(a)) {
			hasExtract = true
			break
		}
	}
	if hasExtract {
		// 逐节点取值：每个节点按备选顺序取第一个非空值。
		var vals []string
		base.Each(func(_ int, node *goquery.Selection) {
			for _, a := range alts {
				a = strings.TrimSpace(a)
				if !isExtractAlt(a) {
					continue
				}
				v := applyExtract(node, a)
				if len(v) > 0 && v[0] != "" {
					vals = append(vals, v[0])
					return
				}
			}
		})
		return nil, vals
	}
	// 选择器整组回退：第一个有命中的备选胜出。
	for _, a := range alts {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		sel, index := stripIndex(a)
		nodes := findNodes(base, sel, index)
		if nodes != nil && nodes.Length() > 0 {
			return nodes, nil
		}
	}
	return nil, nil
}

// isExtractAlt 判断单个备选（已按 || 拆开）是否为取值步。
func isExtractAlt(alt string) bool {
	head := alt
	if i := strings.Index(alt, "!"); i >= 0 {
		head = strings.TrimSpace(alt[:i])
	}
	lower := strings.ToLower(head)
	return lower == "text" || lower == "html" || lower == "owntext" ||
		lower == "alltext" || jsoupKnownAttrs[lower]
}

// applyExtract 执行单个取值步（含 !前缀 去前缀）。
func applyExtract(base *goquery.Selection, alt string) []string {
	prefix := ""
	head := alt
	if i := strings.Index(alt, "!"); i >= 0 {
		head = strings.TrimSpace(alt[:i])
		prefix = strings.TrimSpace(alt[i+1:])
	}
	var vals []string
	lower := strings.ToLower(head)
	switch lower {
	case "text", "owntext", "alltext":
		vals = mapText(base)
	case "html":
		vals = mapHTML(base)
	default:
		vals = mapAttr(base, head)
	}
	if prefix != "" {
		for i, v := range vals {
			vals[i] = strings.TrimSpace(strings.TrimPrefix(v, prefix))
		}
	}
	return vals
}

// stripIndex 从选择器尾部剥离 ",N" 索引。
// 返回值：sel 为去掉索引后的选择器；无索引时返回 noJSoupIndex（要全部命中）。
func stripIndex(sel string) (string, int) {
	sel = strings.TrimSpace(sel)
	m := jsoupIndexSuffixRe.FindStringSubmatch(sel)
	if m == nil {
		return sel, noJSoupIndex
	}
	idx := 0
	neg := m[2][0] == '-'
	for _, c := range m[2] {
		if c < '0' || c > '9' {
			continue
		}
		idx = idx*10 + int(c-'0')
	}
	if neg {
		idx = -idx
	}
	return strings.TrimSpace(m[1]), idx
}

// findNodes 对 base 中每个节点依次执行选择器；index>=0 时取第 index 个
// 命中（负数从末尾数），noJSoupIndex 表示取全部。:matches(regex) /
// :not(:matches(...)) 在剥离后按文本过滤。
func findNodes(base *goquery.Selection, sel string, index int) *goquery.Selection {
	baseSel, matches := splitFilter(sel)

	var found []*xhtml.Node
	base.Each(func(_ int, node *goquery.Selection) {
		nodes := node.Find(baseSel).Nodes
		if index == noJSoupIndex {
			found = append(found, nodes...)
		} else if index >= 0 {
			if index < len(nodes) {
				found = append(found, nodes[index])
			}
		} else {
			if n := len(nodes) + index; n >= 0 {
				found = append(found, nodes[n])
			}
		}
	})
	if len(found) == 0 {
		return nil
	}
	selBuilder := &goquery.Selection{}
	selBuilder = selBuilder.AddNodes(found...)
	if matches == nil {
		return selBuilder
	}
	var filtered []*xhtml.Node
	for _, n := range selBuilder.Nodes {
		wrap := goquery.NewDocumentFromNode(n).Selection
		if matches(wrap) {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	out := &goquery.Selection{}
	return out.AddNodes(filtered...)
}

// closingParen 返回 open 处 '(' 对应的 ')' 下标（感知嵌套括号与正则内括号）。
func closingParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitFilter 剥离 goquery 不支持的伪类，返回剩余选择器与组合后的文本/结构
// 匹配函数（nil 表示不过滤）。支持：
//   - :matches(re) / :not(:matches(re))：按元素自身 HTML（含属性）做正则匹配；
//   - :has(sel)：仅保留内部能命中 sel 后代的节点。
//
// 正则本身可能含括号（如 :matches(https://www.ali(pan|yundrive).com/s/)），
// 必须按配对括号取参数，不能用第一个 ")" 截断。
// :contains(text) 由 goquery 原生支持，原样保留。
func splitFilter(sel string) (string, func(*goquery.Selection) bool) {
	sel = strings.TrimSpace(sel)
	var filters []func(*goquery.Selection) bool

	// :not(:matches(re))
	for {
		marker := ":not(:matches("
		i := strings.Index(sel, marker)
		if i < 0 {
			break
		}
		open := i + len(marker) - 1 // 内层 :matches( 的 '(' 位置
		end := closingParen(sel, open)
		if end < 0 {
			break
		}
		pat := sel[open+1 : end]
		// 外层 :not( 的 ')' 紧跟在内层 ')' 之后。
		outerEnd := end + 1
		if outerEnd >= len(sel) || sel[outerEnd] != ')' {
			break
		}
		sel = strings.TrimSpace(sel[:i] + sel[outerEnd+1:])
		if re, err := regexp.Compile(pat); err == nil {
			filters = append(filters, func(s *goquery.Selection) bool {
				return !re.MatchString(outerHTML(s))
			})
		}
	}

	// :matches(re)
	for {
		marker := ":matches("
		i := strings.Index(sel, marker)
		if i < 0 {
			break
		}
		open := i + len(marker) - 1
		end := closingParen(sel, open)
		if end < 0 {
			break
		}
		pat := sel[open+1 : end]
		sel = strings.TrimSpace(sel[:i] + sel[end+1:])
		if re, err := regexp.Compile(pat); err == nil {
			filters = append(filters, func(s *goquery.Selection) bool {
				return re.MatchString(outerHTML(s))
			})
		}
	}

	// :has(inner)
	for {
		marker := ":has("
		i := strings.Index(sel, marker)
		if i < 0 {
			break
		}
		open := i + len(marker) - 1
		end := closingParen(sel, open)
		if end < 0 {
			break
		}
		inner := strings.TrimSpace(sel[open+1 : end])
		sel = strings.TrimSpace(sel[:i] + sel[end+1:])
		if inner != "" {
			filters = append(filters, func(s *goquery.Selection) bool {
				return s.Find(inner).Length() > 0
			})
		}
	}

	if len(filters) == 0 {
		return sel, nil
	}
	return sel, func(s *goquery.Selection) bool {
		for _, f := range filters {
			if !f(s) {
				return false
			}
		}
		return true
	}
}

// outerHTML 返回节点自身 HTML（失败时退化为纯文本），供正则匹配属性。
func outerHTML(s *goquery.Selection) string {
	h, err := goquery.OuterHtml(s)
	if err != nil {
		return s.Text()
	}
	return h
}

func mapText(base *goquery.Selection) []string {
	out := make([]string, 0, base.Length())
	base.Each(func(_ int, s *goquery.Selection) { out = append(out, strings.TrimSpace(s.Text())) })
	return out
}

func mapHTML(base *goquery.Selection) []string {
	out := make([]string, 0, base.Length())
	base.Each(func(_ int, s *goquery.Selection) {
		if h, err := goquery.OuterHtml(s); err == nil {
			out = append(out, html.UnescapeString(h))
		}
	})
	return out
}

func mapAttr(base *goquery.Selection, name string) []string {
	out := make([]string, 0, base.Length())
	base.Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr(name); ok {
			out = append(out, v)
		}
	})
	return out
}
