package crawler

import "testing"

// rule.Host 可能出现 URL 编码形式（某影视源规则把 host 段用 %XX 编码，
// 如 `pianku.api.%6d%67%74%76.com` 实为 `pianku.api.mgtv.com`）。Go 的
// url.Parse 对 host 里的 %XX 报 invalid URL escape，joinURL 若解析失败就返回
// 相对路径，请求被 resolveRequestURL 打到脚本基址 → 站点反爬/空响应。
// joinURL 必须能解码编码 host 并拼出正确绝对 URL。
func TestJoinURLEncodedHost(t *testing.T) {
	cases := []struct {
		host, path, want string
	}{
		{
			host: "https://pianku.api.%6d%67%74%76.com",
			path: "/rider/list/v3?a=1",
			want: "https://pianku.api.mgtv.com/rider/list/v3?a=1",
		},
		{
			host: "https://pianku.api.mgtv.com",
			path: "/rider/list/v3?a=1",
			want: "https://pianku.api.mgtv.com/rider/list/v3?a=1",
		},
	}
	for i, tc := range cases {
		got := joinURL(tc.host, tc.path)
		if got != tc.want {
			t.Errorf("#%d joinURL(%q, %q) = %q, want %q", i, tc.host, tc.path, got, tc.want)
		}
	}
}
