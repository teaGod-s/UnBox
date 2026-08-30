package crawler

import "testing"

func TestMubanAutoVivifies(t *testing.T) {
	e := New()
	if err := e.Load(`muban.首图2.二级.desc = '.data:eq(0)&&Text'; var rule={title:"x",host:"https://example.com"}`); err != nil {
		t.Fatalf("muban 赋值报错: %v", err)
	}
	m := e.readMuban()
	if m["首图2.二级.desc"] != ".data:eq(0)&&Text" {
		t.Fatalf("muban=%#v", m)
	}
}
