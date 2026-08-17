package config

import (
	"context"
	"errors"
	"fmt"
)

// Resolver 递归展开多仓/聚合结构，直到得到终端配置。
//
// 实测的 TVBox 订阅结构是三层 storeHouse → urls[] → 配置：一个节点可能
// 是继续指向其它节点的索引（storeHouse/urls 非空），也可能是含 sites/lives
// 的终端配置。仓库之间可以互相引用，因此展开过程必须同时具备深度上限与
// 环检测，否则会无限递归。
type Resolver struct {
	Fetcher  *Fetcher
	MaxDepth int
}

// NewResolver 返回一个默认深度上限为 3 的 Resolver。
func NewResolver() *Resolver {
	return &Resolver{Fetcher: NewFetcher(), MaxDepth: 3}
}

// Resolve 从 ref 出发展开所有终端配置。
//
// 索引结构（storeHouse / urls 非空）会被继续展开；Sites/Lives/StoreHouse/
// URLs 四者皆为空的节点视为失败节点（不会被静默地既不收集也不报错）。
// 深度上限与已访问集合共同防止仓库互相引用导致的无限递归；命中已访问集合
// （环引用）是正常的终止条件，不计入错误。
//
// 多仓聚合场景中，同一个配置 URL 完全可能同时被浅、深两条不同路径引用
// （菱形结构），因此同一个 ref 在一次 Resolve 里可能被 walk 访问多次
// （一次深、一次更浅，见 seen 的说明）。为了不重复拉取、也不重复收集，
// Resolve 内部维护了两份按 ref 索引的记录：一份是拉取+解析结果的缓存
// （同一个 ref 只实际 Fetch 一次，后续重访直接复用缓存结果，无论成功还是
// 失败）；一份是"是否已作为终端配置收集过"的标记（同一个 ref 只会被
// append 进 out 一次，即便它经由多条路径都可达）。重访时仍然会用新的、
// 更浅的深度预算去继续展开该节点的子节点——这是允许多路径重访的本意，
// 只是不再重复拉取节点自身、也不再重复收集它作为终端配置的那一份。
//
// 采用“部分成功 + 汇总错误”语义：out 是已成功展开的全部终端配置——即便
// 部分子节点失败，已经成功展开的部分也会照常返回；aggErr 是所有失败节点
// 错误的汇总（errors.Join），没有任何失败时为 nil。
//
// 重要：out 非空时也可能同时返回非 nil 的 aggErr（不会因为已有部分成功
// 就把失败吞掉），调用方需自行决定如何处理二者并存的情况。
//
// 兜底：整个展开过程走完后，若既没有任何终端配置（out 为空）、也没有
// 任何单点错误被记录（aggErr 为 nil），说明这次展开在某个此前未预料到
// 的分支上“成功但什么都没有”——例如 urls 条目全部为空字符串、或者一条
// 从头到尾没有任何终端配置的纯环形索引链。这类情形不应该被当作
// “成功 0 个配置”悄悄放过，这里统一兜底成一个具名错误。
//
// 深度超限的错误延迟到这里统一生成（而不是在 walk 里发现即报），原因
// 见 walk 中 depthRejected 的说明：同一个 ref 完全可能同时存在至少一条
// 预算内路径（真正展开成功）和至少一条超深度路径（在到达前就被拒绝），
// 只有等整棵树都走完、seen 的最终状态确定下来之后，才能判断某个曾被
// 深度拒绝过的 ref 是不是"实际上已经从别的路径成功过"——如果是，这条
// 深度错误就是针对一个已成功节点的误报，不应该出现在汇总错误里。
func (r *Resolver) Resolve(ctx context.Context, ref string) ([]*Config, error) {
	seen := make(map[string]int)
	cache := make(map[string]*fetchResult)
	collected := make(map[string]bool)
	depthRejected := make(map[string]bool)
	var out []*Config
	var errs []error
	r.walk(ctx, ref, 0, seen, cache, collected, depthRejected, &out, &errs)

	// 只为那些从未（经由任何路径）成功进入过 walk 主体的 ref 生成深度
	// 超限错误。seen 在 walk 里是"通过了深度检查、真正开始处理这个 ref"
	// 时才写入的（见 walk），因此 seen 中不存在某个 ref 就意味着它在
	// 这次 Resolve 里没有任何一条路径真正到达过它——那才是真正意义上的
	// "超出深度上限、彻底无法展开"。depthRejected 用集合去重，同一个
	// ref 无论被多少条超深路径拒绝过，这里也只产生一条错误。
	for rejectedRef := range depthRejected {
		if _, ok := seen[rejectedRef]; !ok {
			errs = append(errs, fmt.Errorf("超出最大展开深度 %d: %s", r.MaxDepth, rejectedRef))
		}
	}

	aggErr := errors.Join(errs...)
	if len(out) == 0 && aggErr == nil {
		aggErr = fmt.Errorf("展开 %s 未产生任何可用配置", ref)
	}
	return out, aggErr
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

// walk 展开单个节点。失败不会中断兄弟节点的展开：每个失败节点的错误会
// 被追加到 errs，由调用方（Resolve）统一汇总，而不是丢弃或提前返回。
// 例外是深度超限——这类错误不在这里直接追加，见下方 depthRejected 的
// 说明与 Resolve 里的统一生成逻辑。
//
// seen 记录每个 ref 已知的最小到达深度（而不是简单的“是否访问过”布尔
// 值）。多仓聚合场景中，同一个配置 URL 完全可能同时被浅、深两条不同
// 路径引用（菱形结构），若用布尔集合，DFS 遍历顺序会决定哪条路径先
// “占住” seen：一旦更深的路径先到，浅路径本可达的子树就会被误判为
// “已访问”而永久跳过——展开结果因此依赖遍历顺序，是错误的。
// 这里改为记录最小深度：只有以严格更浅的深度重新到达同一个 ref 时才
// 值得重新展开（更浅意味着其子树在深度预算内有更大的空间）；以相同或
// 更深的深度重复到达则跳过。深度是非负整数且每次重新展开都严格变浅，
// 所以不会退化成无限递归。
//
// seen[ref] 的写入时机很关键：只要通过了深度检查、走到这一行，就说明
// 这个 ref 这次是"真正开始处理"（无论后续 fetch/parse 是否成功），因此
// seen 中存在某个 ref 可以准确表示"该 ref 至少有一条路径成功进入过
// walk 主体"，这也是 Resolve 里过滤 depthRejected 时使用的判据。
//
// depthRejected 只记录"曾经因为深度超限而被拒绝、连 fetch 都没有尝试"
// 的 ref 本身，不在这里就地生成错误。原因：同一个 ref 完全可能同时存在
// 至少一条超深路径（会命中这里的深度检查）和至少一条预算内路径（会
// 正常进入主体并成功展开、收集）——如果哪条路径先被走到是不确定的
// （取决于 storeHouse/urls 数组顺序），在这里立即报错会对一个实际上已
// 经成功展开的 ref 产生一条虚假的"超出最大展开深度"错误。是否真的要
// 报这个错误，只有等整棵树都走完之后，看这个 ref 最终有没有任何一条
// 路径进入过 seen 才能确定，因此推迟到 Resolve 里统一处理。
//
// cache 与 collected 用于处理"重新展开"带来的副作用：重新展开不代表
// 要重新拉取（cache 让同一个 ref 只实际 Fetch/Parse 一次），也不代表
// 要把同一个终端配置再收集一次（collected 保证每个 ref 只 append 进
// out 一次）。cache/collected 都不影响是否继续下潜展开子节点——子节点
// 的递归展开仍然无条件地在每次 walk 调用里发生，用的是这次调用实际
// 拿到的深度预算。
func (r *Resolver) walk(ctx context.Context, ref string, depth int, seen map[string]int, cache map[string]*fetchResult, collected map[string]bool, depthRejected map[string]bool, out *[]*Config, errs *[]error) {
	if depth > r.MaxDepth {
		depthRejected[ref] = true
		return
	}
	if prevDepth, ok := seen[ref]; ok && prevDepth <= depth {
		// 已经以相同或更浅的深度展开过，不会有新的可展开空间。
		// 若这是一个环引用（严格意义上的重复路径），这里也是它的
		// 正常终止点，不算失败。
		return
	}
	seen[ref] = depth

	res, cached := cache[ref]
	if !cached {
		res = r.fetchAndParse(ctx, ref)
		cache[ref] = res
		if res.err != nil {
			*errs = append(*errs, res.err)
		}
	}
	if res.err != nil {
		// 拉取/解析失败此前已经（且只）在首次遇到该 ref 时记入 errs，
		// 命中缓存的重访不重复报错。
		return
	}
	cfg := res.cfg

	isTerminal := len(cfg.Sites) > 0 || len(cfg.Lives) > 0
	isIndex := len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0

	// 四个判定集合皆为空：既不是终端配置也不是索引，静默传播下去只会
	// 让上游误以为该节点“成功但什么都没有”。其它字段（spider/wallpaper/
	// logo/hosts 等）不参与判定——即便存在，也不能替代 sites/lives 等
	// 四个集合表示“有可用内容”。只在首次遇到该 ref 时记这条错误，
	// 避免重访时重复计入。
	if !isTerminal && !isIndex {
		if !cached {
			*errs = append(*errs, fmt.Errorf("配置无任何可用内容（sites/lives/storeHouse/urls 均为空）: %s", ref))
		}
		return
	}

	if isTerminal && !collected[ref] {
		*out = append(*out, cfg)
		collected[ref] = true
	}

	// 索引结构，继续展开。单个子节点失败不中断其余兄弟节点的展开，
	// 失败会被记录进 errs 而不是丢弃。条目本身的 sourceUrl/url 为空
	// 字符串同样是一种失败（不是可以悄悄略过的“空位”），一并记录进
	// errs，错误信息带上父节点 ref 便于定位是哪个节点里的坏条目；
	// 这条错误同样只在首次遇到该 ref 时记录一次。
	for _, h := range cfg.StoreHouse {
		if h.SourceURL == "" {
			if !cached {
				*errs = append(*errs, fmt.Errorf("storeHouse 条目 sourceUrl 为空，父节点: %s", ref))
			}
			continue
		}
		r.walk(ctx, h.SourceURL, depth+1, seen, cache, collected, depthRejected, out, errs)
	}
	for _, u := range cfg.URLs {
		if u.URL == "" {
			if !cached {
				*errs = append(*errs, fmt.Errorf("urls 条目 url 为空，父节点: %s", ref))
			}
			continue
		}
		r.walk(ctx, u.URL, depth+1, seen, cache, collected, depthRejected, out, errs)
	}
}
