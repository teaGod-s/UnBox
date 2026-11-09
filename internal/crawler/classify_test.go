package crawler

import (
	"errors"
	"testing"
)

func TestClassifyJsError(t *testing.T) {
	cases := []struct {
		err  error
		want string
		desc string
	}{
		{errors.New("ReferenceError: md5X is not defined at go"), "engine", "JS 运行时异常"},
		{errors.New("Timeout exceeded"), "site", "网络超时"},
		{errors.New("Get https://x: connection refused"), "site", "连接失败"},
		{errors.New("爬虫动作返回空值"), "empty", "脚本返回空"},
		{errors.New("爬虫详情返回列表为空"), "empty", "详情空列表"},
		{errors.New("play 响应 url 为空"), "empty", "playURL空"},
		{errors.New("class_parse 需要四段规则"), "engine", "class_parse 约束"},
		{errors.New("HTTP 522"), "site", "HTTP状态码"},
		{errors.New("格式错误"), "engine", "引擎格式错误"},
	}
	for _, c := range cases {
		got, _ := classifyJsError(c.err)
		if got != c.want {
			t.Errorf("classifyJsError(%q) = %q, want %q (%s)", c.err.Error(), got, c.want, c.desc)
		}
	}
}
