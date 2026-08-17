package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestResolveFollowsStoreHouseAndURLs 验证三层 storeHouse → urls[] → 配置
// 能被递归展开到终端配置。
func TestResolveFollowsStoreHouseAndURLs(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"storeHouse":[{"sourceName":"仓A","sourceUrl":"%s/house"}]}`, srv.URL)
	})
	mux.HandleFunc("/house", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/leaf","name":"线路1"}]}`, srv.URL)
	})
	mux.HandleFunc("/leaf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("应当展开到 1 个站点，实际 %d（配置数 %d）", total, len(cfgs))
	}
}

// TestResolveDetectsCycle 验证仓库互相引用时不会无限递归：环引用命中时
// 正常终止，不算作错误。
func TestResolveDetectsCycle(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// A 指向 B，B 指回 A
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/b","name":"b"}]}`, srv.URL)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/a","name":"a"}]}`, srv.URL)
	})

	r := NewResolver()
	done := make(chan struct{})
	var cfgs []*Config
	var err error
	go func() {
		cfgs, err = r.Resolve(context.Background(), srv.URL+"/a")
		close(done)
	}()
	select {
	case <-done: // 正常结束即通过
	case <-timeAfter():
		t.Fatal("环引用导致无限递归")
	}
	// 环引用是正常终止条件，不应产生错误，也不会有任何终端配置。
	if err != nil {
		t.Errorf("环引用不应产生错误，实际: %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("环引用链路中没有终端配置，实际得到 %d 个", len(cfgs))
	}
}

// TestResolveEmptyNodeIsError 验证四集合（sites/lives/storeHouse/urls）
// 全为空的节点被视为失败节点，而不是静默地既不收集也不报错。
func TestResolveEmptyNodeIsError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		// 带有 spider/wallpaper 等字段，但四个判定集合皆为空。
		fmt.Fprint(w, `{"spider":"http://x/spider.jar","wallpaper":"http://x/wall.png"}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/empty")
	if err == nil {
		t.Fatal("四集合全空的节点应当返回错误，实际 err 为 nil")
	}
	if !strings.Contains(err.Error(), srv.URL+"/empty") {
		t.Errorf("错误信息应包含失败节点的 ref，实际: %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("空配置节点不应产生任何终端配置，实际 %d 个", len(cfgs))
	}
}

// TestResolveIndexOnlyNodeIsNotEmpty 验证只有 storeHouse（没有 urls，也没有
// sites/lives）的节点是合法的索引节点，会被继续展开，而不是被判为空配置。
func TestResolveIndexOnlyNodeIsNotEmpty(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"storeHouse":[{"sourceName":"仓A","sourceUrl":"%s/leaf"}]}`, srv.URL)
	})
	mux.HandleFunc("/leaf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err != nil {
		t.Fatalf("只含 storeHouse 的索引节点不应报错: %v", err)
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("应当展开到 1 个站点，实际 %d", total)
	}
}

// TestResolvePartialFailureAggregatesErrors 验证根节点是索引、其子节点部分
// 成功部分失败时，Resolve 采用“部分成功 + 汇总错误”语义：已成功展开的
// 配置照常返回，同时汇总错误也非 nil（不会因为有部分成功就把错误吞掉）。
func TestResolvePartialFailureAggregatesErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/good","name":"好"},{"url":"%s/bad","name":"坏"}]}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/good", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err == nil {
		t.Fatal("部分子节点失败时应返回汇总错误，实际 err 为 nil")
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("成功的子节点应当照常展开，实际站点数 %d", total)
	}
}

// TestResolveAllChildrenFailReturnsError 验证根节点是索引、其全部子节点都
// 失败时，Resolve 不会返回“成功但零配置”（即 out 为空但 err 为 nil），
// 而是返回非 nil 的汇总错误，避免下游把全盘失败误报为“成功 0 个配置”。
func TestResolveAllChildrenFailReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/bad1","name":"坏1"},{"url":"%s/bad2","name":"坏2"}]}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/bad1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/bad2", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err == nil {
		t.Fatal("全部子节点失败时应返回错误，实际 err 为 nil")
	}
	if len(cfgs) != 0 {
		t.Errorf("全部子节点失败时不应有任何终端配置，实际 %d 个", len(cfgs))
	}
	// 两个子节点的错误都应体现在汇总错误中。
	if !strings.Contains(err.Error(), srv.URL+"/bad1") {
		t.Errorf("汇总错误应包含 /bad1 的失败信息，实际: %v", err)
	}
	if !strings.Contains(err.Error(), srv.URL+"/bad2") {
		t.Errorf("汇总错误应包含 /bad2 的失败信息，实际: %v", err)
	}
	// errors.Join 产出的错误应当能用 errors.Is/As 逐个拆解（不是被拍扁成
	// 单个字符串），这里用 errors.Unwrap 系列接口间接验证聚合结构存在。
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Errorf("汇总错误应支持 Unwrap() []error（如 errors.Join 产出），实际类型: %T", err)
	} else if len(joined.Unwrap()) != 2 {
		t.Errorf("应聚合 2 个子错误，实际 %d 个", len(joined.Unwrap()))
	}
}

// TestResolveMaxDepthExceeded 验证深度上限确实生效。
//
// NewResolver().MaxDepth == 3，walk 的判定是 depth > r.MaxDepth 时失败，
// 且 Resolve 以 depth=0 调用根节点。因此：
//
//	depth 0 = n0（根，index）  -- 允许（0 > 3 为假）
//	depth 1 = n1（index）      -- 允许
//	depth 2 = n2（index）      -- 允许
//	depth 3 = n3（index）      -- 允许（3 > 3 为假）
//	depth 4 = n4（终端，含 sites） -- 拒绝（4 > 3 为真），n4 从未被 fetch
//
// 构造一条 n0 → n1 → n2 → n3 → n4 的非环形链（每个节点 URL 各不相同，
// 只有链尾 n4 含 sites），断言：
//   - 最终站点总数为 0（n4 从未被展开到）
//   - 汇总错误非 nil，且提及深度上限与被拒绝的 n4 ref
func TestResolveMaxDepthExceeded(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/n0", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/n1","name":"n1"}]}`, srv.URL)
	})
	mux.HandleFunc("/n1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/n2","name":"n2"}]}`, srv.URL)
	})
	mux.HandleFunc("/n2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/n3","name":"n3"}]}`, srv.URL)
	})
	mux.HandleFunc("/n3", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/n4","name":"n4"}]}`, srv.URL)
	})
	mux.HandleFunc("/n4", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	if r.MaxDepth != 3 {
		t.Fatalf("本测试假定 NewResolver().MaxDepth == 3，实际 %d，请相应调整测试", r.MaxDepth)
	}

	cfgs, err := r.Resolve(context.Background(), srv.URL+"/n0")
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 0 {
		t.Errorf("超出深度上限的 n4 不应被展开到，实际站点数 %d", total)
	}
	if err == nil {
		t.Fatal("超出深度上限应产生错误，实际 err 为 nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", r.MaxDepth)) {
		t.Errorf("错误信息应体现深度上限 %d，实际: %v", r.MaxDepth, err)
	}
	if !strings.Contains(err.Error(), srv.URL+"/n4") {
		t.Errorf("错误信息应体现被拒绝的 n4 ref，实际: %v", err)
	}
}

func timeAfter() <-chan time.Time {
	return time.After(10 * time.Second)
}
