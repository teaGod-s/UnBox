package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Resolver 递归展开多仓/聚合结构，直到得到终端配置。
//
// 实测的 TVBox 订阅结构是三层 storeHouse → urls[] → 配置：一个节点可能
// 是继续指向其它节点的索引（storeHouse/urls 非空），也可能是含 sites/lives
// 的终端配置。仓库之间可以互相引用，因此展开过程必须同时具备深度上限与
// 环检测，否则会无限递归。
//
// 展开是网络密集型的（每个节点一次 HTTP 拉取），因此用并发 DFS 并行拉取：
// 每个节点开一个 goroutine，用 semaphore 限制并发度，共享状态由 resolveState
// 内的互斥锁守护，拉取走 single-flight（同一个 ref 只实际 Fetch 一次）。
type Resolver struct {
	Fetcher  *Fetcher
	MaxDepth int
}

// NewResolver 返回一个默认深度上限为 3 的 Resolver。
func NewResolver() *Resolver {
	return &Resolver{Fetcher: NewFetcher(), MaxDepth: 3}
}

// resolveWorkerLimit 是展开时并发拉取的上限。
const resolveWorkerLimit = 8

// resolveState 是单次 Resolve 的全部共享状态，字段由 mu 守护。
//
// seen 记录每个 ref 已知的最小到达深度（不是简单的"是否访问过"布尔值）。
// 多仓聚合中，同一个配置 URL 可能同时被浅、深两条路径引用（菱形结构），
// 只有以严格更浅的深度重新到达时才有重新展开的价值（更浅意味着其子树在
// 深度预算内有更大空间）；以相同或更深深度重复到达则跳过。claim 方法在
// 互斥锁内原子地完成"检查 + 写入"，保证并发下每个 ref 的最终 seen 值
// 仍是可达的最小深度。
type resolveState struct {
	mu            sync.Mutex
	seen          map[string]int
	cache         map[string]*fetchResult
	inflight      map[string]chan struct{}
	collected     map[string]bool
	depthRejected map[string]bool
	out           []*Config
	errs          []error
}

func newResolveState() *resolveState {
	return &resolveState{
		seen:          make(map[string]int),
		cache:         make(map[string]*fetchResult),
		inflight:      make(map[string]chan struct{}),
		collected:     make(map[string]bool),
		depthRejected: make(map[string]bool),
	}
}

// claim 原子地完成"最小深度"检查与写入：若 ref 已以相同或更浅的深度到达过
// 则返回 false（调用方跳过），否则写入当前深度并返回 true（调用方继续）。
func (st *resolveState) claim(ref string, depth int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if prev, ok := st.seen[ref]; ok && prev <= depth {
		return false
	}
	st.seen[ref] = depth
	return true
}

// recordDepthRejected 记录一个因深度超限而连 fetch 都没尝试的 ref。
func (st *resolveState) recordDepthRejected(ref string) {
	st.mu.Lock()
	st.depthRejected[ref] = true
	st.mu.Unlock()
}

// recordErr 追加一条错误。
func (st *resolveState) recordErr(err error) {
	st.mu.Lock()
	st.errs = append(st.errs, err)
	st.mu.Unlock()
}

// collect 把一个终端配置收集进 out，同一 ref 只收集一次。
func (st *resolveState) collect(ref string, cfg *Config) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.collected[ref] {
		return
	}
	st.collected[ref] = true
	st.out = append(st.out, cfg)
}

// getOrFetch 按 ref 取拉取+解析结果，single-flight：同一个 ref 只实际
// Fetch 一次，并发到达的 goroutine 等待首发者完成。返回 fresh 表示本次
// 调用是否真的执行了拉取（false 表示命中缓存或等待他人）。
func (st *resolveState) getOrFetch(ctx context.Context, ref string, fetchFn func(context.Context, string) *fetchResult) (*fetchResult, bool) {
	st.mu.Lock()
	if res, ok := st.cache[ref]; ok {
		st.mu.Unlock()
		return res, false
	}
	if ch, ok := st.inflight[ref]; ok {
		st.mu.Unlock()
		<-ch
		st.mu.Lock()
		res := st.cache[ref]
		st.mu.Unlock()
		return res, false
	}
	ch := make(chan struct{})
	st.inflight[ref] = ch
	st.mu.Unlock()

	res := fetchFn(ctx, ref)

	st.mu.Lock()
	st.cache[ref] = res
	close(ch)
	delete(st.inflight, ref)
	st.mu.Unlock()
	return res, true
}

// Resolve 从 ref 出发展开所有终端配置。
//
// 索引结构（storeHouse / urls 非空）会被继续展开；Sites/Lives/StoreHouse/
// URLs 四者皆为空的节点视为失败节点。深度上限与 seen 共同防止仓库互相
// 引用导致的无限递归；命中已访问集合（环引用）是正常的终止条件，不计入
// 错误。
//
// 采用"部分成功 + 汇总错误"语义：out 是已成功展开的全部终端配置，aggErr
// 是所有失败节点错误的汇总（errors.Join）。out 非空时也可能同时返回非 nil
// 的 aggErr，调用方自行决定如何处理二者并存。
//
// 深度超限的错误延迟到展开结束后统一生成：同一个 ref 完全可能同时存在
// 至少一条预算内路径（真正展开成功）和至少一条超深度路径（在到达前就被
// 拒绝），只有等整棵树走完、seen 最终状态确定后，才能判断某个曾被深度
// 拒绝过的 ref 是不是"实际上已从别的路径成功过"——若是，这条深度错误是
// 对已成功节点的误报，不应出现在汇总错误里。
func (r *Resolver) Resolve(ctx context.Context, ref string) ([]*Config, error) {
	st := newResolveState()
	sem := make(chan struct{}, resolveWorkerLimit)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()
		r.walk(ctx, ref, 0, st, sem, &wg)
	}()
	wg.Wait()

	return st.finish(r.MaxDepth, ref)
}

// finish 在全部节点处理完后汇总结果：只为从未进入过 seen 的深度拒绝 ref
// 生成深度错误（避免对实际已成功的 ref 产生虚假深度错误），再与 walk 中
// 记录的错误合并，兜底处理"零配置且零错误"的意外分支。
func (st *resolveState) finish(maxDepth int, ref string) ([]*Config, error) {
	var errs []error
	for rejectedRef := range st.depthRejected {
		if _, ok := st.seen[rejectedRef]; !ok {
			errs = append(errs, fmt.Errorf("超出最大展开深度 %d: %s", maxDepth, rejectedRef))
		}
	}
	errs = append(errs, st.errs...)

	aggErr := errors.Join(errs...)
	if len(st.out) == 0 && aggErr == nil {
		aggErr = fmt.Errorf("展开 %s 未产生任何可用配置", ref)
	}
	return st.out, aggErr
}

// fetchResult 缓存单个 ref 的拉取+解析结果：cfg 与 err 恰好一个非零值。
type fetchResult struct {
	cfg *Config
	err error
}

// fetchAndParse 拉取并解析单个 ref，不涉及递归展开。
func (r *Resolver) fetchAndParse(ctx context.Context, ref string) *fetchResult {
	raw, err := r.Fetcher.Fetch(ctx, ref)
	if err != nil {
		return &fetchResult{err: fmt.Errorf("拉取 %s: %w", ref, err)}
	}
	cfg, err := Parse(raw)
	if err != nil {
		return &fetchResult{err: fmt.Errorf("解析 %s: %w", ref, err)}
	}
	return &fetchResult{cfg: cfg}
}

// walk 处理单个节点。每个节点在一个独立 goroutine 里运行，子节点再各自
// spawn 新 goroutine；sem 限制并发度，wg 追踪在途工作。
//
// 拉取/解析失败、四集合全空的空节点、storeHouse/urls 条目为空等错误只在
// 首次实际拉取该 ref（fresh）时记录一次，命中缓存/等待他人的重访不重复
// 报错——与单线程版本的 cached 去重语义一致。
func (r *Resolver) walk(ctx context.Context, ref string, depth int, st *resolveState, sem chan struct{}, wg *sync.WaitGroup) {
	if depth > r.MaxDepth {
		st.recordDepthRejected(ref)
		return
	}
	if !st.claim(ref, depth) {
		return
	}

	res, fresh := st.getOrFetch(ctx, ref, r.fetchAndParse)
	if res.err != nil {
		if fresh {
			st.recordErr(res.err)
		}
		return
	}
	cfg := res.cfg

	isTerminal := len(cfg.Sites) > 0 || len(cfg.Lives) > 0
	isIndex := len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0

	// 四个判定集合皆为空：既不是终端配置也不是索引，视为失败节点。
	if !isTerminal && !isIndex {
		if fresh {
			st.recordErr(fmt.Errorf("配置无任何可用内容（sites/lives/storeHouse/urls 均为空）: %s", ref))
		}
		return
	}

	if isTerminal {
		st.collect(ref, cfg)
	}

	for _, h := range cfg.StoreHouse {
		if h.SourceURL == "" {
			if fresh {
				st.recordErr(fmt.Errorf("storeHouse 条目 sourceUrl 为空，父节点: %s", ref))
			}
			continue
		}
		r.spawn(ctx, h.SourceURL, depth+1, st, sem, wg)
	}
	for _, u := range cfg.URLs {
		if u.URL == "" {
			if fresh {
				st.recordErr(fmt.Errorf("urls 条目 url 为空，父节点: %s", ref))
			}
			continue
		}
		r.spawn(ctx, u.URL, depth+1, st, sem, wg)
	}
}

// spawn 为一个子节点起一个 goroutine，受 sem 与 wg 约束。
func (r *Resolver) spawn(ctx context.Context, ref string, depth int, st *resolveState, sem chan struct{}, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()
		r.walk(ctx, ref, depth, st, sem, wg)
	}()
}
