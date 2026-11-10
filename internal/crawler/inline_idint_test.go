package crawler

import "testing"

// JSON 列表里的整数 id（3331822）经 jsonScalarString 必须保持整数形式，
// 否则 float64 的 %v 会输出科学计数法 3.331822e+06，详情 URL（album_id=…）
// 直接失效（某听书源实际线上根因）。
func TestJSONScalarStringIntID(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"整数id", float64(3331822), "3331822"},
		{"小整数", float64(1024), "1024"},
		{"大整数在精确范围", float64(9000000000000), "9000000000000"},
		{"真小数", float64(3.14), "3.14"},
		{"负整数", float64(-42), "-42"},
		{"字符串", "abc", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonScalarString(tc.value)
			if got != tc.want {
				t.Fatalf("jsonScalarString(%#v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
