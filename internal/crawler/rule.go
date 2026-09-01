package crawler

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

var jqueryPositionalRe = regexp.MustCompile(`(?i)^(.*):((?:lt|eq))\((-?\d+)\)$`)

// evalRule 执行一条 pdfh/pdfa 规则，返回全部命中值。
func evalRule(doc *goquery.Document, rule string) []string {
	selection := doc.Selection
	values := []string(nil)
	stringMode := false
	for _, raw := range strings.Split(rule, "&&") {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			continue
		}
		if !stringMode {
			switch {
			case seg == "Text()":
				values = selection.Map(func(_ int, s *goquery.Selection) string { return strings.TrimSpace(s.Text()) })
				stringMode = true
			case seg == "Html()":
				values = selection.Map(func(_ int, s *goquery.Selection) string { h, _ := goquery.OuterHtml(s); return h })
				stringMode = true
			case seg == "href" || seg == "src" || strings.HasPrefix(seg, "attr("):
				name := seg
				if seg == "href" || seg == "src" {
					values = selection.Map(func(_ int, s *goquery.Selection) string { v, _ := s.Attr(name); return v })
				} else {
					name = strings.Trim(strings.TrimSuffix(strings.TrimPrefix(seg, "attr("), ")"), `"' `)
					values = selection.Map(func(_ int, s *goquery.Selection) string { v, _ := s.Attr(name); return v })
				}
				stringMode = true
			case seg == "Array()":
				values = selection.Map(func(_ int, s *goquery.Selection) string { return strings.TrimSpace(s.Text()) })
				stringMode = true
			case strings.HasPrefix(seg, "match("):
				pattern := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(seg, "match("), ")"), `"'`)
				if re, err := regexp.Compile(pattern); err == nil {
					values = selection.Map(func(_ int, s *goquery.Selection) string { return re.FindString(s.Text()) })
				} else {
					values = selection.Map(func(_ int, _ *goquery.Selection) string { return "" })
				}
				stringMode = true
			case strings.HasPrefix(seg, "split("):
				sep := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(seg, "split("), ")"), `"'`)
				values = selection.Map(func(_ int, s *goquery.Selection) string { return strings.Split(s.Text(), sep)[0] })
				stringMode = true
			default:
				selection = findRuleSelection(selection, seg)
			}
			continue
		}
		switch {
		case seg == "trim":
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
		case seg == "ltrim":
			for i := range values {
				values[i] = strings.TrimLeftFunc(values[i], func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' })
			}
		case seg == "rtrim":
			for i := range values {
				values[i] = strings.TrimRightFunc(values[i], func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' })
			}
		case strings.HasPrefix(seg, "match("):
			pattern := strings.TrimSuffix(strings.TrimPrefix(seg, "match("), ")")
			pattern = strings.Trim(pattern, `"'`)
			if re, err := regexp.Compile(pattern); err == nil {
				for i := range values {
					values[i] = re.FindString(values[i])
				}
			}
		case strings.HasPrefix(seg, "split("):
			sep := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(seg, "split("), ")"), `"'`)
			for i := range values {
				values[i] = strings.Split(values[i], sep)[0]
			}
		case strings.HasPrefix(seg, "replace("):
			args := parseCallArgs(seg)
			if len(args) >= 2 {
				for i := range values {
					values[i] = strings.ReplaceAll(values[i], args[0], args[1])
				}
			}
		case strings.HasPrefix(seg, "substring("):
			args := parseCallArgs(seg)
			start, _ := strconv.Atoi(argsAt(args, 0))
			end := -1
			if len(args) > 1 && args[1] != "" {
				end, _ = strconv.Atoi(args[1])
			}
			for i := range values {
				values[i] = substring(values[i], start, end)
			}
		}
	}
	if stringMode {
		return values
	}
	out := make([]string, 0, selection.Length())
	selection.Each(func(_ int, s *goquery.Selection) { out = append(out, strings.TrimSpace(s.Text())) })
	return out
}

// findRuleSelection adds the small jQuery selector subset commonly used by
// dr_py. goquery's CSS parser does not understand :lt/:eq, so strip the
// pseudo, select normally, then apply the positional operation to the result.
func findRuleSelection(selection *goquery.Selection, selector string) *goquery.Selection {
	if match := jqueryPositionalRe.FindStringSubmatch(selector); len(match) == 4 {
		base := selection.Find(strings.TrimSpace(match[1]))
		index, _ := strconv.Atoi(match[3])
		if strings.EqualFold(match[2], "lt") {
			if index < 0 {
				index = 0
			}
			if index > base.Length() {
				index = base.Length()
			}
			return base.Slice(0, index)
		}
		if index < 0 {
			index = base.Length() + index
		}
		if index < 0 || index >= base.Length() {
			return base.Slice(0, 0)
		}
		return base.Eq(index)
	}
	return selection.Find(selector)
}

func substring(s string, start, end int) string {
	r := []rune(s)
	if end < 0 {
		end = len(r)
	}
	if start < 0 {
		start = 0
	}
	if start > len(r) {
		start = len(r)
	}
	if end < start {
		end = start
	}
	if end > len(r) {
		end = len(r)
	}
	return string(r[start:end])
}
func argsAt(args []string, i int) string {
	if i >= len(args) {
		return "0"
	}
	return args[i]
}
func parseCallArgs(s string) []string {
	s = strings.TrimSuffix(strings.TrimPrefix(s, strings.SplitN(s, "(", 2)[0]+"("), ")")
	var out []string
	for _, a := range strings.Split(s, ",") {
		out = append(out, strings.Trim(strings.TrimSpace(a), `"'`))
	}
	return out
}

func (e *Engine) installRule() {
	doc := func(html string) *goquery.Document {
		d, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		return d
	}
	_ = e.vm.Set("pdfh", func(call goja.FunctionCall) goja.Value {
		values := evalRule(doc(call.Argument(0).String()), call.Argument(1).String())
		if len(values) == 0 {
			return goja.Undefined()
		}
		return e.vm.ToValue(values[0])
	})
	_ = e.vm.Set("pdfa", func(call goja.FunctionCall) goja.Value {
		return e.vm.ToValue(evalRule(doc(call.Argument(0).String()), call.Argument(1).String()))
	})
	_ = e.vm.Set("pd", func(call goja.FunctionCall) goja.Value {
		values := evalRule(doc(call.Argument(0).String()), call.Argument(1).String())
		join := valueString(call.Argument(2))
		return e.vm.ToValue(strings.Join(values, join))
	})
}
