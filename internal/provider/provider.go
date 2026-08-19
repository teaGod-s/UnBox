// Package provider 定义所有内容来源的统一接口。所有来源实现 Provider，
// UI 层只依赖接口，不依赖具体实现（spec §3.1）。
package provider

import (
	"context"

	"github.com/unbox/unbox/internal/player"
)

// Section 是首页的一个分组。
type Section struct {
	ID    string
	Title string
}

// Item 是浏览列表中的一项（直播=频道）。
type Item struct {
	ID    string
	Title string
	Logo  string
	Group string
}

// Page 是一页浏览结果。
type Page struct {
	Items []Item
}

// Media 是详情（M1 直播仅含频道元信息；M2 点播再扩展剧集字段）。
type Media struct {
	ID    string
	Title string
	Logo  string
	Group string
}

// Provider 是所有来源的统一接口。
type Provider interface {
	ID() string
	Home(ctx context.Context) ([]Section, error)
	Browse(ctx context.Context, cat string, page int) (Page, error)
	Search(ctx context.Context, q string) ([]Item, error)
	Detail(ctx context.Context, id string) (Media, error)
	Resolve(ctx context.Context, id string) (player.Stream, error)
}
