package crawler

import "testing"

// rule 里有动态对象值（如 headers.Cookie 是函数返回、filter 嵌套对象）时，
// ExportTo 到 Rule struct 不应 panic 整个进程，而应容错（error 或成功）。
func TestRuleExportRobust(t *testing.T) {
	e := New()
	// filter 是嵌套对象（真实 FongMi filter 形态），值里含 value 数组对象——之前 ExportTo 会崩
	script := `var rule={host:"https://x",class_name:"电影",class_url:"1",
	  headers:{'User-Agent':'UA'}, filter:{movie:[{key:'type',name:'类型',value:[{n:'全部',v:'0'}]}]}}`
	if err := e.Load(script); err != nil {
		t.Fatalf("load: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Rule() panic（应容错）: %v", r)
		}
	}()
	r, err := e.Rule()
	if err != nil {
		t.Fatalf("Rule() err: %v", err)
	}
	if r.Host != "https://x" || len(r.Filter) != 1 {
		t.Fatalf("Rule 解析: %+v", r)
	}
}
