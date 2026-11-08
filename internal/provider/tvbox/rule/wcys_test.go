package rule

import (
	"os"
	"testing"
)

// 真实文才影视规则（播放数组含 "题链#\\#>>" 合法转义，旧 sanitize 把它误成
// \\\# 报 invalid escape sequence，整站解析失败）。缺文件时跳过。
func TestParseXBPQWcysRealRule(t *testing.T) {
	raw, err := os.ReadFile("/tmp/wcys.json")
	if err != nil {
		t.Skipf("缺真实规则文件: %v", err)
	}
	r, err := ParseXBPQ(raw)
	if err != nil {
		t.Fatalf("真实 wcys 规则应能解析（\\# 合法转义不再破坏）: %v", err)
	}
	if r.HomeURL == "" && r.CategoryURL == "" {
		t.Fatal("规则解析后主页/分类为空")
	}
	t.Logf("wcys 解析成功: 分类url=%q 数组=%q", r.CategoryURL, clip(r.Array, 40))
}

// clip 截断字符串用于日志。
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
