package crawler

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// resolveLazy runs a dr_py lazy rule and returns the resolved media URL.
// dr_py scripts use both `input = "..."` and `input = {url: "..."}`;
// the initial object also exposes split() for scripts written against the
// older string-shaped input convention.
func (e *Engine) resolveLazy(flag, id string) (string, error) {
	if e == nil || e.vm == nil {
		return "", fmt.Errorf("爬虫引擎未初始化")
	}
	rule, err := e.Rule()
	if err != nil {
		return "", err
	}
	script := strings.TrimSpace(rule.Lazy)
	if script == "" {
		return id, nil
	}
	if strings.HasPrefix(script, "js:") {
		script = strings.TrimSpace(strings.TrimPrefix(script, "js:"))
	}

	input := e.vm.NewObject()
	_ = input.Set("flag", flag)
	_ = input.Set("url", id)
	_ = input.Set("split", func(call goja.FunctionCall) goja.Value {
		separator := ""
		if len(call.Arguments) > 0 {
			separator = call.Argument(0).String()
		}
		parts := strings.Split(id, separator)
		return e.vm.ToValue(parts)
	})
	_ = e.vm.Set("input", input)

	if _, err := e.vm.RunString(script); err != nil {
		return "", fmt.Errorf("lazy 规则执行失败: %w", err)
	}
	value := e.vm.Get("input")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return "", fmt.Errorf("lazy 规则未返回播放地址")
	}
	if object := value.ToObject(e.vm); object != nil {
		if url := valueString(object.Get("url")); url != "" {
			return url, nil
		}
	}
	if result := value.String(); result != "" {
		return result, nil
	}
	return "", fmt.Errorf("lazy 规则未返回播放地址")
}
