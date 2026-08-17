package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

// TestResolveDetectsCycle 验证仓库互相引用时不会无限递归；同时验证这个
// "全程没有任何终端配置的纯环形引用" 会被 Fix1 的兜底判定为失败——
// 而不是像最初实现那样被静默当作"成功但零配置"。
//
// 这里刻意不只断言 err != nil，而是拆成两条互补的断言：
//   - err 必须包含 Fix1 兜底错误的特征串（"未产生任何可用配置"），
//     证明这个环确实是被 seen（环检测）在环内拦下、而不是碰巧因为
//     别的原因失败；
//   - err 必须不包含"超出最大展开深度"。这一条是关键：如果环检测
//     哪天被改坏、不再识别这个环，Resolve 也不会死循环（还有
//     MaxDepth 兜底），而是会一路撞到深度上限、同样返回一个
//     err != nil 的错误。只断言 err != nil 的话，环检测彻底失效
//     这条测试依然会绿——它就不再是有效的回归闸门了。必须证明
//     "err 非 nil"这件事的原因是环检测本身在生效，而不是别的
//     兜底机制凑巧也报了错。
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
	if err == nil {
		t.Fatal("纯环形引用、全程无终端配置时应返回错误，实际 err 为 nil")
	}
	if !strings.Contains(err.Error(), "未产生任何可用配置") {
		t.Errorf("错误应体现 Fix1 的兜底判定（证明这个环是被 seen 拦下的），实际: %v", err)
	}
	if strings.Contains(err.Error(), "超出最大展开深度") {
		t.Errorf("这个环应当被 seen 直接拦下而不是撞到深度上限，实际: %v", err)
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

// TestResolveMixedNodeCollectsAndExpands 锁定“混合节点”（同一份配置同时
// 含 sites 与 storeHouse/urls）的行为：既作为终端配置被收集，也继续展开
// 其索引部分，两者互不排斥。防止后续重构把这两种行为改成互斥的。
func TestResolveMixedNodeCollectsAndExpands(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/mixed", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}],"urls":[{"url":"%s/leaf","name":"线路1"}]}`, srv.URL)
	})
	mux.HandleFunc("/leaf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"b","name":"站点B","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/mixed")
	if err != nil {
		t.Fatalf("混合节点不应报错: %v", err)
	}
	if len(cfgs) != 2 {
		t.Fatalf("应当同时收集混合节点自身与其展开出的 leaf，配置数应为 2，实际 %d", len(cfgs))
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 2 {
		t.Errorf("站点总数应为 2（混合节点自身 1 个 + leaf 1 个），实际 %d", total)
	}
}

// TestResolveAllEmptyURLEntriesIsError 验证条目非空但 url 全是空字符串
// 时（`{"urls":[{"url":""},{"url":""}]}`），不会被静默判定为“成功 0 个
// 配置”，而是返回错误。
func TestResolveAllEmptyURLEntriesIsError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"urls":[{"url":"","name":"空1"},{"url":"","name":"空2"}]}`)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/index")
	if err == nil {
		t.Fatal("urls 条目全部为空字符串时应返回错误，实际 err 为 nil")
	}
	if len(cfgs) != 0 {
		t.Errorf("不应产生任何终端配置，实际 %d 个", len(cfgs))
	}
}

// TestResolvePureCyclicIndexWithoutTerminalIsError 验证 Fix1 的兜底不是
// 只对最简单的两节点环生效。TestResolveDetectsCycle 已经覆盖了两节点环
// （A↔B）这个最基础的情形，这里换成三节点环（root→a→b→root，环长度
// 不同，全程同样没有任何终端配置），确认"纯环形索引、无终端配置 =>
// 返回错误"这个行为不依赖环的具体长度，而不是与既有测试断言重复的
// 同一场景。
func TestResolvePureCyclicIndexWithoutTerminalIsError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/root", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/a","name":"a"}]}`, srv.URL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/b","name":"b"}]}`, srv.URL)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/root","name":"root"}]}`, srv.URL)
	})

	r := NewResolver()
	cfgs, err := r.Resolve(context.Background(), srv.URL+"/root")
	if err == nil {
		t.Fatal("三节点纯环形索引、全程无终端配置时应返回错误，实际 err 为 nil")
	}
	if len(cfgs) != 0 {
		t.Errorf("不应产生任何终端配置，实际 %d 个", len(cfgs))
	}
}

// TestResolveDiamondReachesShallowerLeaf 验证 seen 改为深度感知后，菱形
// 引用结构中，一个先经深路径访问过的节点，若之后又能以更浅的深度到达，
// 会被重新展开，而不是被更深路径先占住的访问记录永久挡住。
//
// 结构：
//
//	root(depth0) --urls--> a(depth1) --urls--> b(depth2) --urls--> x(depth3)
//	root(depth0) --urls--------------------------------------------> x(depth1，更浅)
//	x --urls--> leaf(终端，含 sites)
//
// mux 的 handler 里 root 先声明指向 a 的边、再声明指向 x 的边，因此按
// urls 数组顺序，DFS 会先沿 root→a→b→x 这条深路径在 depth3 访问到 x；
// 若用旧的布尔 seen，随后 root→x 这条 depth1 的浅路径会因为“x 已访问”
// 被直接跳过，x 的子节点 leaf（本应在 depth2 可达）永远不会被展开，
// 断言会看到站点数为 0。用深度感知的 seen，浅路径应当重新展开 x，
// leaf 在 depth2 被访问到（未超过 MaxDepth=3），站点数应为 1。
func TestResolveDiamondReachesShallowerLeaf(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/root", func(w http.ResponseWriter, r *http.Request) {
		// 先深路径（root->a），再浅路径（root->x）。
		fmt.Fprintf(w, `{"urls":[{"url":"%s/a","name":"a"},{"url":"%s/x","name":"x"}]}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/b","name":"b"}]}`, srv.URL)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/x","name":"x"}]}`, srv.URL)
	})
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/leaf","name":"leaf"}]}`, srv.URL)
	})
	mux.HandleFunc("/leaf", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sites":[{"key":"a","name":"站点A","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	if r.MaxDepth != 3 {
		t.Fatalf("本测试假定 NewResolver().MaxDepth == 3，实际 %d，请相应调整测试", r.MaxDepth)
	}

	cfgs, err := r.Resolve(context.Background(), srv.URL+"/root")
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("leaf 应当经由 root->x 这条更浅的路径被展开到，实际站点数 %d（err=%v）", total, err)
	}
}

// TestResolveDiamondTerminalCollectedOnce 是深度感知 seen（Fix2）引入的
// 回归的专项回归测试：菱形引用结构中，如果被浅、深两条路径都能到达的
// 那个节点本身就是终端配置（而不是像 TestResolveDiamondReachesShallowerLeaf
// 里那样只是转发到另一个终端 leaf），"以更浅深度重新展开"绝不能变成
// "把同一份终端配置再收集一次"，也不能变成"把同一个 ref 再拉取一次"。
//
// 结构：
//
//	root --urls--> a --urls--> b --urls--> x（depth3，先经深路径到达，x 本身含 sites）
//	root --urls-----------------------------> x（depth1，更浅，重新展开）
//
// x 没有 urls/storeHouse，是纯终端节点。断言两件事：
//   - 最终站点总数恰好为 1（不是 2）——终端配置不能因为经多路径可达
//     就被重复收集；
//   - /x 这个 HTTP 端点恰好被命中 1 次——同一个 ref 在一次 Resolve
//     里最多只应被实际 Fetch 一次，用原子计数器统计而不是仅凭最终
//     结果推断，避免"先重复拉取、再在收集阶段去重"这种对慢速订阅源
//     仍然浪费网络请求的实现蒙混过关。
func TestResolveDiamondTerminalCollectedOnce(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var xHits int64

	mux.HandleFunc("/root", func(w http.ResponseWriter, r *http.Request) {
		// 先深路径（root->a），再浅路径（root->x），与
		// TestResolveDiamondReachesShallowerLeaf 保持同样的触发顺序。
		fmt.Fprintf(w, `{"urls":[{"url":"%s/a","name":"a"},{"url":"%s/x","name":"x"}]}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/b","name":"b"}]}`, srv.URL)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"urls":[{"url":"%s/x","name":"x"}]}`, srv.URL)
	})
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&xHits, 1)
		fmt.Fprint(w, `{"sites":[{"key":"X","name":"站点X","type":1,"api":"http://x/api"}]}`)
	})

	r := NewResolver()
	if r.MaxDepth != 3 {
		t.Fatalf("本测试假定 NewResolver().MaxDepth == 3，实际 %d，请相应调整测试", r.MaxDepth)
	}

	cfgs, err := r.Resolve(context.Background(), srv.URL+"/root")
	if err != nil {
		t.Fatalf("展开不应报错: %v", err)
	}
	total := 0
	for _, c := range cfgs {
		total += len(c.Sites)
	}
	if total != 1 {
		t.Errorf("同一个终端 ref 经深、浅两条路径都可达时，只应被收集一次，实际站点数 %d", total)
	}
	if got := atomic.LoadInt64(&xHits); got != 1 {
		t.Errorf("同一个终端 ref 在一次 Resolve 中最多只应被拉取一次，实际拉取 %d 次", got)
	}
}

func timeAfter() <-chan time.Time {
	return time.After(10 * time.Second)
}
