package rule

import (
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// xpath 简单 XPath 子集：供规则为「否/0」的 XYQHiker 站点使用。
//
// 支持的语法（其余写法优雅降级为空，不报错）：
//
//	//div[@class='module-items']//a[@class='item']/@href
//	  ^轴     ^条件                              ^取值步
//
//   - 轴：/ 或 //（不限层级，goquery Find 本身就是后代）。
//   - 节点条件：元素名或 *，后接多个 [..] 谓词：
//     `[@class='x']` / `[@id="y"]` / `[@data-src='z']`（属性等值）；
//     `[@attr]`（属性存在）；`[2]`（位置，XPath 1 起始）；
//     `[last()]`（最后一个）。
//   - 取值步（最后一段）：`text()` 取文本；`@attr` 取属性；
//     缺省按节点文本返回。
//
// xpathExtractFirst 在 doc 上求值，返回全部命中值。

func xpathExtractFirst(doc *goquery.Document, rule string) []string {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	steps, lastTake := xpathSplitSteps(rule)
	base := doc.Selection
	for _, step := range steps {
		if lastTake != "" && step == lastTake {
			break // 取值步在末尾统一处理
		}
		base = xpathApplyStep(base, step)
		if base == nil || base.Length() == 0 {
			return nil
		}
	}
	if lastTake != "" {
		if strings.HasPrefix(lastTake, "@") {
			return xpathMapAttr(base, strings.TrimPrefix(lastTake, "@"))
		}
		if strings.EqualFold(lastTake, "text()") {
			return xpathMapText(base)
		}
	}
	return xpathMapText(base)
}

// xpathSelectNodes 执行 xpath 并停在最后的节点步，返回命中节点集合
// （供数组规则使用）。含 text()/@attr 取值步时返回 nil。
func xpathSelectNodes(doc *goquery.Document, rule string) *goquery.Selection {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	steps, lastTake := xpathSplitSteps(rule)
	if lastTake != "" {
		return nil
	}
	base := doc.Selection
	for _, step := range steps {
		base = xpathApplyStep(base, step)
		if base == nil || base.Length() == 0 {
			return nil
		}
	}
	return base
}

func xpathApplyStep(base *goquery.Selection, step string) *goquery.Selection {
	nodes, index := xpathStepSelector(step)
	next := base.Find(nodes)
	if index == -2 {
		return next
	}
	if index >= 0 {
		if index < next.Length() {
			return next.Eq(index)
		}
		return nil
	}
	if n := next.Length(); n > 0 {
		return next.Eq(n - 1)
	}
	return nil
}

// xpathSplitSteps 把 xpath 按 / 拆成步；返回步列表与末尾取值步
// （text()/@attr 形态，无则空串）。
func xpathSplitSteps(xpath string) ([]string, string) {
	var steps []string
	var cur strings.Builder
	var lastTake string
	i := 0
	s := strings.TrimSpace(xpath)
	flush := func() {
		if t := strings.TrimSpace(cur.String()); t != "" {
			if strings.HasPrefix(t, "@") || strings.EqualFold(t, "text()") {
				lastTake = t
			} else {
				lastTake = ""
				steps = append(steps, t)
			}
		}
		cur.Reset()
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c == '/' {
			if cur.Len() == 0 {
				continue // 前导 / 或 //：跳过
			}
			flush()
			continue
		}
		cur.WriteByte(c)
	}
	flush()
	return steps, lastTake
}

// xpathStepSelector 把一个节点步翻译成 goquery 选择器与位置索引。
// index：>=0 取第 index 个；-1 取最后一个（[last()]）；-2 表示全部。
func xpathStepSelector(step string) (string, int) {
	step = strings.TrimSpace(step)
	index := -2
	// 收集 [..] 谓词，同时避免拆开引号里的 "]"。
	var css strings.Builder
	i := 0
	for i < len(step) {
		if step[i] == '[' {
			j := xpathFindBracket(step, i)
			if j < 0 {
				return step, index
			}
			pred := step[i+1 : j]
			i = j + 1
			if strings.EqualFold(pred, "last()") {
				index = -1
				continue
			}
			if n, err := strconv.Atoi(strings.TrimSpace(pred)); err == nil {
				index = n - 1 // XPath 1-based
				continue
			}
			if sel := xpathAttrPredicate(pred); sel != "" {
				css.WriteString(sel)
			}
			continue
		}
		css.WriteByte(step[i])
		i++
	}
	selector := strings.TrimSpace(css.String())
	// 无谓词的 * 退化为全选：不用前缀，空串意味着匹配 tag=*。
	if selector == "*" {
		selector = ""
	}
	return selector, index
}

// xpathFindBracket 从 i 起找匹配的 "]"（支持引号内 "]"）。
func xpathFindBracket(s string, i int) int {
	quote := byte(0)
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ']':
			return j
		}
	}
	return -1
}

// xpathAttrPredicate 把 [@attr='v'] / [@attr="v"] / [@attr] 翻译成 CSS。
func xpathAttrPredicate(pred string) string {
	pred = strings.TrimSpace(pred)
	if !strings.HasPrefix(pred, "@") {
		return ""
	}
	pred = strings.TrimPrefix(pred, "@")
	name, val, hasVal := xpathSplitAttr(pred)
	if name == "" {
		return ""
	}
	if !hasVal {
		return "[" + name + "]"
	}
	if name == "class" {
		// [class="x y"] 精确等值可行；直接用属性选择器。
		return `[class="` + val + `"]`
	}
	return `[` + name + `="` + val + `"]`
}

// xpathSplitAttr 拆 "@name" 与 "@name='v'"。
func xpathSplitAttr(raw string) (name, val string, hasVal bool) {
	raw = strings.TrimSpace(raw)
	if q := strings.IndexByte(raw, '='); q >= 0 {
		name = strings.TrimSpace(raw[:q])
		v := strings.TrimSpace(raw[q+1:])
		v = strings.Trim(v, `'"`)
		return name, v, true
	}
	return raw, "", false
}

func xpathMapText(base *goquery.Selection) []string {
	out := make([]string, 0, base.Length())
	base.Each(func(_ int, s *goquery.Selection) { out = append(out, strings.TrimSpace(s.Text())) })
	return out
}

func xpathMapAttr(base *goquery.Selection, name string) []string {
	out := make([]string, 0, base.Length())
	base.Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr(name); ok {
			out = append(out, v)
		}
	})
	return out
}
