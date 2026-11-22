package crawler

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/unbox/unbox/internal/httpx"
)

// maskNonCode 返回与 src 等长的副本：字符串、模板字面量（含 ${} 插值）、正则字面量、
// 行注释、块注释内容替换为空格。保留换行符，便于错误定位。仅用于语法定位，不参与执行。
// 已知局限：正则判定用前一个有效 token 粗略判断（isRegexStart），不追求完整 JS
// 词法；个别罕见形态（如除号后紧跟正则可疑）可能误判，但对压缩脚本足够。
func maskNonCode(src string) string {
	out := []byte(src)
	n := len(src)
	blank := func(i int) {
		if out[i] != '\n' && out[i] != '\r' {
			out[i] = ' '
		}
	}
	for i := 0; i < n; {
		switch src[i] {
		case '\'', '"':
			i = maskString(src, out, i, src[i], blank)
		case '`':
			i = maskTemplate(src, out, i, blank)
		case '/':
			if i+1 < n && src[i+1] == '/' {
				for i < n && src[i] != '\n' {
					blank(i)
					i++
				}
			} else if i+1 < n && src[i+1] == '*' {
				blank(i)
				blank(i + 1)
				i += 2
				for i < n {
					if src[i] == '*' && i+1 < n && src[i+1] == '/' {
						blank(i)
						blank(i + 1)
						i += 2
						break
					}
					blank(i)
					i++
				}
			} else if isRegexStart(src, i) {
				if end, ok := scanRegex(src, i); ok {
					for k := i; k < end; k++ {
						blank(k)
					}
					i = end
				} else {
					i++
				}
			} else {
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

// maskString 从 i（指向开引号）开始清空字符串内容到闭引号（引号本身保留，
// import 正则需要匹配 "..."），返回闭引号之后的下标。
func maskString(src string, out []byte, i int, quote byte, blank func(int)) int {
	n := len(src)
	i++
	for i < n {
		c := src[i]
		if c == '\\' {
			blank(i)
			if i+1 < n {
				blank(i + 1)
			}
			i += 2
			continue
		}
		if c == quote {
			i++
			return i
		}
		blank(i)
		i++
	}
	return i
}

// maskTemplate 从 i（指向开 backtick）开始清空模板字面量内容到闭 backtick
// （backtick 本身保留），正确处理 ${} 插值内的嵌套字符串/模板/正则/注释，
// 返回闭 backtick 之后的下标。
func maskTemplate(src string, out []byte, i int, blank func(int)) int {
	n := len(src)
	i++
	for i < n {
		c := src[i]
		if c == '\\' {
			blank(i)
			if i+1 < n {
				blank(i + 1)
			}
			i += 2
			continue
		}
		if c == '`' {
			i++
			return i
		}
		if c == '$' && i+1 < n && src[i+1] == '{' {
			blank(i)
			blank(i + 1)
			i = maskInterpolation(src, out, i+2, blank)
			continue
		}
		blank(i)
		i++
	}
	return i
}

// maskInterpolation 从 i（指向 ${ 之后）开始清空插值表达式到匹配的 }，
// 返回 } 之后的下标。插值内可再含嵌套模板/字符串/正则/注释与花括号。
func maskInterpolation(src string, out []byte, i int, blank func(int)) int {
	n := len(src)
	depth := 1
	for i < n && depth > 0 {
		switch src[i] {
		case '\\':
			blank(i)
			if i+1 < n {
				blank(i + 1)
			}
			i += 2
		case '\'', '"':
			i = maskString(src, out, i, src[i], blank)
		case '`':
			i = maskTemplate(src, out, i, blank)
		case '/':
			if i+1 < n && src[i+1] == '/' {
				for i < n && src[i] != '\n' {
					blank(i)
					i++
				}
			} else if i+1 < n && src[i+1] == '*' {
				blank(i)
				blank(i + 1)
				i += 2
				for i < n {
					if src[i] == '*' && i+1 < n && src[i+1] == '/' {
						blank(i)
						blank(i + 1)
						i += 2
						break
					}
					blank(i)
					i++
				}
			} else if isRegexStart(src, i) {
				if end, ok := scanRegex(src, i); ok {
					for k := i; k < end; k++ {
						blank(k)
					}
					i = end
				} else {
					i++
				}
			} else {
				i++
			}
		case '{':
			depth++
			blank(i)
			i++
		case '}':
			depth--
			blank(i)
			i++
		default:
			blank(i)
			i++
		}
	}
	return i
}

// isRegexStart 用前一个有效 token 粗略判断 '/' 是正则还是除法。
// 覆盖压缩脚本常见形态；不追求完整 JS 词法。
func isRegexStart(src string, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r' || src[j] == '\n') {
		j--
	}
	if j < 0 {
		return true
	}
	switch src[j] {
	case '(', '[', '{', ',', ';', ':', '=', '!', '&', '|', '?', '+', '-', '*', '%', '^', '~', '<', '>':
		return true
	}
	// 关键字：return / typeof / instanceof / in / of / new / delete /
	// void / throw / case / do / else / yield / await
	end := j + 1
	for j >= 0 && isIdentByte(src[j]) {
		j--
	}
	word := src[j+1 : end]
	switch word {
	case "return", "typeof", "instanceof", "in", "of", "new", "delete",
		"void", "throw", "case", "do", "else", "yield", "await":
		return true
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '$' || (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// scanRegex 从 i（指向 '/'）开始，返回结束位置（含 flags）和是否成功。
// 扫描到未转义、不在字符类 [...] 内的结束 '/'，并跳过其后的 flags；
// 遇到换行视为未闭合（回退），避免吞掉后续代码。
func scanRegex(src string, i int) (int, bool) {
	j := i + 1
	inClass := false
	for j < len(src) {
		c := src[j]
		if c == '\\' {
			j += 2
			continue
		}
		if c == '[' {
			inClass = true
		} else if c == ']' {
			inClass = false
		} else if c == '/' && !inClass {
			j++
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			return j, true
		} else if c == '\n' || c == '\r' {
			return i, false
		}
		j++
	}
	return i, false
}

var (
	// 注意：Go regexp 不支持 lookahead，用两组分支覆盖：
	//   import x / import{a} / import*as / import "x" / import.meta
	// 压缩器会输出 import{a}from... / import*as n from...（无空格），必须匹配。
	reImportKeyword = regexp.MustCompile(`\bimport\s+[\p{L}_$][\p{L}\p{N}_$]*|\bimport\s*[{\*"'\.]`)
	// 动态 import：前面不能是 . 或标识符字符，避免把 obj.import(...) 误判。
	reDynamicImport = regexp.MustCompile(`(?:^|[^.\p{L}\p{N}_$])import\s*\(`)
	//   export default / export function / export {x} / export{x}
	reExportKeyword = regexp.MustCompile(`\bexport\s+([\p{L}_$][\p{L}\p{N}_$]*|\{|\*)|\bexport\s*[\{\*]`)
)

// needsModuleLoader 判断是否需要 ESM 加载器：
// 有 import（含无空格 named/namespace/dynamic），或有 export default 之外的 export。
func needsModuleLoader(src string) bool {
	masked := maskNonCode(src)
	if reImportKeyword.MatchString(masked) || reDynamicImport.MatchString(masked) {
		return true
	}
	for _, m := range reExportKeyword.FindAllStringSubmatch(masked, -1) {
		if !strings.EqualFold(m[1], "default") {
			return true
		}
	}
	return false
}

type moduleEdit struct {
	start, end int
	text       string
}

func addModuleEdit(edits *[]moduleEdit, start, end int, text string) bool {
	for _, e := range *edits {
		if start < e.end && e.start < end {
			return false
		}
	}
	*edits = append(*edits, moduleEdit{start: start, end: end, text: text})
	return true
}

var (
	reImpDefault = regexp.MustCompile(`import\s+([\p{L}_$][\p{L}\p{N}_$]*)\s*(?:,\s*\{([^}]*)\})?\s+from\s*(["'][^"']*["'])`)
	reImpNamed   = regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*(["'][^"']*["'])`)
	reImpNS      = regexp.MustCompile(`import\s*\*\s*as\s+([\p{L}_$][\p{L}\p{N}_$]*)\s+from\s*(["'][^"']*["'])`)
	reImpSide    = regexp.MustCompile(`import\s*(["'][^"']*["'])`)
	// 压缩器会把 `export default {...}` 写成 `export default{...}`（default 后无空格），
	// 用 \b 匹配完整关键字 default，而不是要求后随空白。
	reExpDefault = regexp.MustCompile(`export\s+default\b`)
	// export async function 是真实脚本的常见形态；async 由 rewriteModule 末尾的
	// stripAsyncAwait 统一去掉（这里替换时也直接丢弃）。
	reExpFunc = regexp.MustCompile(`export\s+(?:async\s+)?function\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpVar  = regexp.MustCompile(`export\s+(const|let|var)\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpList = regexp.MustCompile(`export\s*\{([^}]*)\}`)
	reExpStar = regexp.MustCompile(`export\s*\*\s*from`)
	// 当前加载器不支持的再导出/声明导出形态。必须显式拒绝：
	// 静默跳过会留下非法 JS，goja 只报难以定位的语法错误。
	reExpUnsupported = regexp.MustCompile(
		`\bexport\s*\*\s*as\s+[\p{L}_$]|\bexport\s*\{[^}]*\}\s*from\b|\bexport\s+class\b`)
)

// resolveImportSpec 把 import 的字符串字面量解析成绝对地址。
// assets:// / http(s):// 原样返回，相对路径按 baseURL 解析。
func resolveImportSpec(vm *goja.Runtime, spec, baseURL string) (string, error) {
	v, err := vm.RunString(spec)
	if err != nil {
		return "", fmt.Errorf("导入地址不是字符串字面量: %s", spec)
	}
	ref := v.String()
	if strings.HasPrefix(ref, "assets://") ||
		strings.HasPrefix(ref, "http://") ||
		strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	if baseURL == "" {
		return "", fmt.Errorf("相对导入 %q 缺少 base URL", ref)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(r).String(), nil
}

// rewriteModule 把 ES Module 语法改写成 goja 可直接执行的 CommonJS 形态：
// import → __module() 调用，export → __exports 赋值。暂不支持 export * from
// 与 import.meta / 动态 import()。
func rewriteModule(vm *goja.Runtime, src, baseURL string) (string, error) {
	masked := maskNonCode(src)
	if reExpUnsupported.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 export * as / export {…} from / export class", ErrESModuleUnsupported)
	}
	if reExpStar.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 export * from", ErrESModuleUnsupported)
	}
	if strings.Contains(masked, "import.meta") || reDynamicImport.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 import.meta / 动态 import()", ErrESModuleUnsupported)
	}

	var edits []moduleEdit
	var exports []string

	// imports
	for _, idx := range reImpDefault.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpDefault.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[3], baseURL)
		if err != nil {
			return "", err
		}
		repl := fmt.Sprintf("const %s = (function(m){return m&&m.default!==undefined?m.default:m;})(__module(%s));", m[1], strconv.Quote(abs))
		if m[2] != "" {
			repl += fmt.Sprintf(" const {%s} = __module(%s);", strings.ReplaceAll(m[2], " as ", ": "), strconv.Quote(abs))
		}
		if !addModuleEdit(&edits, idx[0], idx[1], repl) {
			return "", fmt.Errorf("import 语句重叠: %s", region)
		}
	}
	for _, idx := range reImpNamed.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNamed.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil {
			return "", err
		}
		repl := fmt.Sprintf("const {%s} = __module(%s);", strings.ReplaceAll(m[1], " as ", ": "), strconv.Quote(abs))
		if !addModuleEdit(&edits, idx[0], idx[1], repl) {
			continue
		}
	}
	for _, idx := range reImpNS.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNS.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil {
			return "", err
		}
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("const %s = __module(%s);", m[1], strconv.Quote(abs))) {
			continue
		}
	}
	for _, idx := range reImpSide.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpSide.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[1], baseURL)
		if err != nil {
			return "", err
		}
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("__module(%s);", strconv.Quote(abs))) {
			continue
		}
	}

	// exports
	for _, idx := range reExpDefault.FindAllStringIndex(masked, -1) {
		if !addModuleEdit(&edits, idx[0], idx[1], "__exports.default = ") {
			return "", fmt.Errorf("重复的 export default")
		}
	}
	for _, idx := range reExpFunc.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpFunc.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], "function "+m[1]) {
			continue
		}
		exports = append(exports, m[1])
	}
	for _, idx := range reExpVar.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpVar.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], m[1]+" "+m[2]) {
			continue
		}
		exports = append(exports, m[2])
	}
	for _, idx := range reExpList.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpList.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], "") {
			continue
		}
		for item := range strings.SplitSeq(m[1], ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			local, exported := item, item
			if parts := strings.Split(item, " as "); len(parts) == 2 {
				local, exported = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
			exports = append(exports, exported+" = "+local)
		}
	}

	slices.SortFunc(edits, func(a, b moduleEdit) int { return cmp.Compare(b.start, a.start) })
	for _, e := range edits {
		src = src[:e.start] + e.text + src[e.end:]
	}
	for _, item := range exports {
		if parts := strings.SplitN(item, " = ", 2); len(parts) == 2 {
			src += "\n__exports." + parts[0] + " = " + parts[1] + ";"
		} else {
			src += "\n__exports." + item + " = " + item + ";"
		}
	}
	return stripAsyncAwait(src), nil
}

type moduleLoader struct {
	e     *Engine
	ctx   context.Context
	mu    sync.Mutex
	cache map[string]goja.Value
}

func (l *moduleLoader) load(rawURL string) (goja.Value, error) {
	l.mu.Lock()
	if v, ok := l.cache[rawURL]; ok {
		l.mu.Unlock()
		return v, nil
	}
	l.mu.Unlock()

	var src string
	embedded := false
	if s, ok := assetSource(rawURL); ok {
		src = s
		embedded = true
	} else if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		b, err := l.e.fetchModule(l.ctx, rawURL)
		if err != nil {
			return nil, err
		}
		src = b
	} else if strings.HasPrefix(rawURL, "assets://") {
		// assets:// 语法本身是支持的，走到这里说明内置资源里没有这个文件。
		// 最常见的是 cat.js：上游为 GPL-3.0，不随应用分发（见 M7.3）。
		return nil, assetMissingError(rawURL)
	} else {
		return nil, fmt.Errorf("模块地址协议不支持: %s", rawURL)
	}
	return l.loadSource(src, rawURL, embedded)
}

func (l *moduleLoader) forget(rawURL string) {
	if rawURL == "" {
		return
	}
	l.mu.Lock()
	delete(l.cache, rawURL)
	l.mu.Unlock()
}

func (l *moduleLoader) loadSource(src, rawURL string, embedded bool) (goja.Value, error) {
	// 内置的 UMD 库（crypto-js 等）靠 `globalThis.X = factory()` 把自己
	// 挂到全局，站点脚本随后直接引用裸标识符 CryptoJS。若按 CommonJS 包装执行，
	// UMD 会走 `module.exports` 分支而不挂全局，于是站点里 `CryptoJS is not
	// defined`（表现为「filterOptions 解码失败」这类静默降级）。这类没有
	// import/export 语法的内置库直接在全局作用域执行。
	if embedded && !hasModuleSyntax(src) {
		return l.runGlobalAsset(src, rawURL)
	}

	exports := l.e.vm.NewObject()
	module := l.e.vm.NewObject()
	_ = module.Set("exports", exports)

	if rawURL != "" {
		l.mu.Lock()
		l.cache[rawURL] = exports
		l.mu.Unlock()
	}

	// 主模块没有 URL；用 <main> 占位，避免「模块  改写失败」这种双空格报错。
	label := rawURL
	if label == "" {
		label = "<main>"
	}

	rewritten, err := rewriteModule(l.e.vm, src, rawURL)
	if err != nil {
		l.forget(rawURL)
		return nil, fmt.Errorf("模块 %s 改写失败: %w", label, err)
	}
	code := "(function(__exports, module, exports){" + rewritten +
		"\n;if(Object.keys(__exports).length>0){return __exports;}" +
		"return module.exports;})"
	fnVal, err := l.e.vm.RunString(code)
	if err != nil {
		l.forget(rawURL)
		return nil, fmt.Errorf("模块 %s 编译失败: %w", label, err)
	}
	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		l.forget(rawURL)
		return nil, fmt.Errorf("模块 %s 不是可执行函数", label)
	}
	result, err := fn(goja.Undefined(), exports, module, exports)
	if err != nil {
		l.forget(rawURL)
		return nil, fmt.Errorf("模块 %s 执行失败: %w", label, err)
	}
	// UMD/CJS 模块会重新赋值 module.exports，缓存需更新为执行后的最终导出；
	// 执行前写入只是为了支持循环依赖时拿到同一个（部分填充的）exports 对象。
	if rawURL != "" {
		l.mu.Lock()
		l.cache[rawURL] = result
		l.mu.Unlock()
	}
	return result, nil
}

// hasModuleSyntax 判断脚本里是否有真实的 import/export 语句（注释与字符串已掩码）。
func hasModuleSyntax(src string) bool {
	masked := maskNonCode(src)
	return reImportKeyword.MatchString(masked) || reExportKeyword.MatchString(masked)
}

// globalAssignRe 匹配 `globalThis.CryptoJS =` 这类全局挂载。
var globalAssignRe = regexp.MustCompile(`globalThis\.([\p{L}_$][\p{L}\p{N}_$]*)\s*=[^=]`)

// runGlobalAsset 在全局作用域执行一个无模块语法的内置库，并把它挂载的全局
// 同时暴露成模块导出，这样 `import CryptoJS from "assets://…"` 与
// `import "assets://…"` 两种写法都能用。
func (l *moduleLoader) runGlobalAsset(src, rawURL string) (goja.Value, error) {
	if rawURL != "" {
		l.mu.Lock()
		l.cache[rawURL] = l.e.vm.NewObject()
		l.mu.Unlock()
	}
	label := rawURL
	if label == "" {
		label = "<main>"
	}
	if _, err := l.e.vm.RunString(src); err != nil {
		l.forget(rawURL)
		return nil, fmt.Errorf("模块 %s 执行失败: %w", label, err)
	}

	exports := l.e.vm.NewObject()
	for _, m := range globalAssignRe.FindAllStringSubmatch(src, -1) {
		name := m[1]
		value := l.e.vm.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			continue
		}
		_ = exports.Set(name, value)
		if def := exports.Get("default"); def == nil || goja.IsUndefined(def) {
			_ = exports.Set("default", value)
		}
	}
	if rawURL != "" {
		l.mu.Lock()
		l.cache[rawURL] = exports
		l.mu.Unlock()
	}
	return exports, nil
}

func (e *Engine) fetchModule(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", crawlerUA)
	resp, err := httpx.Do(e.hc, req)
	if err != nil {
		return "", fmt.Errorf("下载模块失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载模块 %s 返回 HTTP %d", rawURL, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("读取模块失败: %w", err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "<") {
		return "", fmt.Errorf("模块 %s 返回了 HTML 而不是 JS", rawURL)
	}
	return string(b), nil
}

func (e *Engine) loadESM(ctx context.Context, src, baseURL string) error {
	l := &moduleLoader{e: e, ctx: ctx, cache: map[string]goja.Value{}}
	_ = e.vm.Set("__module", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(e.vm.NewTypeError("__module 缺少 URL"))
		}
		v, err := l.load(call.Argument(0).String())
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		return v
	})
	exports, err := l.loadSource(src, baseURL, false)
	if err != nil {
		return err
	}
	return e.promoteModuleExports(exports)
}

func (e *Engine) promoteModuleExports(exports goja.Value) error {
	if exports == nil || goja.IsUndefined(exports) || goja.IsNull(exports) {
		return nil
	}
	obj := exports.ToObject(e.vm)
	// 先提升直接导出。
	for _, key := range obj.Keys() {
		if key == "default" || key == "__jsEvalReturn" {
			continue
		}
		_ = e.vm.Set(key, obj.Get(key))
	}
	// drpyS/ESM 入口：调用 __jsEvalReturn()，提升返回对象。
	if fn, ok := goja.AssertFunction(obj.Get("__jsEvalReturn")); ok {
		v, err := fn(goja.Undefined())
		if err != nil {
			return fmt.Errorf("__jsEvalReturn 执行失败: %w", err)
		}
		if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			ret := v.ToObject(e.vm)
			for _, key := range ret.Keys() {
				_ = e.vm.Set(key, ret.Get(key))
			}
		}
		return nil
	}
	// FongMi js0 / drpyS：export default 对象。先复制其键；若对象自身带有
	// __jsEvalReturn（drpyS 风格），再调用它并提升返回对象，避免动作静默丢失。
	if def := obj.Get("default"); def != nil && !goja.IsUndefined(def) && !goja.IsNull(def) {
		if o, ok := def.(*goja.Object); ok {
			for _, key := range o.Keys() {
				_ = e.vm.Set(key, o.Get(key))
			}
			if fn, ok := goja.AssertFunction(o.Get("__jsEvalReturn")); ok {
				v, err := fn(goja.Undefined())
				if err != nil {
					return fmt.Errorf("__jsEvalReturn 执行失败: %w", err)
				}
				if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					ret := v.ToObject(e.vm)
					for _, key := range ret.Keys() {
						_ = e.vm.Set(key, ret.Get(key))
					}
				}
			}
		}
	}
	return nil
}
