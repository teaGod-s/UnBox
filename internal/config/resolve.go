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
// 采用“部分成功 + 汇总错误”语义：out 是已成功展开的全部终端配置——即便
// 部分子节点失败，已经成功展开的部分也会照常返回；aggErr 是所有失败节点
// 错误的汇总（errors.Join），没有任何失败时为 nil。
//
// 重要：out 非空时也可能同时返回非 nil 的 aggErr（不会因为已有部分成功
// 就把失败吞掉），调用方需自行决定如何处理二者并存的情况。
func (r *Resolver) Resolve(ctx context.Context, ref string) ([]*Config, error) {
	seen := make(map[string]bool)
	var out []*Config
	var errs []error
	r.walk(ctx, ref, 0, seen, &out, &errs)
	return out, errors.Join(errs...)
}

// walk 展开单个节点。失败不会中断兄弟节点的展开：每个失败节点的错误会
// 被追加到 errs，由调用方（Resolve）统一汇总，而不是丢弃或提前返回。
func (r *Resolver) walk(ctx context.Context, ref string, depth int, seen map[string]bool, out *[]*Config, errs *[]error) {
	if depth > r.MaxDepth {
		*errs = append(*errs, fmt.Errorf("超出最大展开深度 %d: %s", r.MaxDepth, ref))
		return
	}
	if seen[ref] {
		// 环引用，正常终止条件，不算失败。
		return
	}
	seen[ref] = true

	raw, err := r.Fetcher.Fetch(ctx, ref)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("拉取 %s: %w", ref, err))
		return
	}
	cfg, err := Parse(raw)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("解析 %s: %w", ref, err))
		return
	}

	isTerminal := len(cfg.Sites) > 0 || len(cfg.Lives) > 0
	isIndex := len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0

	// 四个判定集合皆为空：既不是终端配置也不是索引，静默传播下去只会
	// 让上游误以为该节点“成功但什么都没有”。其它字段（spider/wallpaper/
	// logo/hosts 等）不参与判定——即便存在，也不能替代 sites/lives 等
	// 四个集合表示“有可用内容”。
	if !isTerminal && !isIndex {
		*errs = append(*errs, fmt.Errorf("配置无任何可用内容（sites/lives/storeHouse/urls 均为空）: %s", ref))
		return
	}

	if isTerminal {
		*out = append(*out, cfg)
	}

	// 索引结构，继续展开。单个子节点失败不中断其余兄弟节点的展开，
	// 失败会被记录进 errs 而不是丢弃。
	for _, h := range cfg.StoreHouse {
		if h.SourceURL != "" {
			r.walk(ctx, h.SourceURL, depth+1, seen, out, errs)
		}
	}
	for _, u := range cfg.URLs {
		if u.URL != "" {
			r.walk(ctx, u.URL, depth+1, seen, out, errs)
		}
	}
}
