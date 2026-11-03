package rule

import (
	"context"
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Vod 是引擎返回的详情原始数据；由 tvbox.JarProvider 转成 provider.Media。
type Vod struct {
	ID       string
	Name     string
	Pic      string
	TypeName string
	Remarks  string
	Year     string
	Area     string
	Content  string
	PlayFrom string // $$$ 分隔的线路名
	PlayURL  string // $$$ 线路、# 集、$ 集名/地址
}

// Engine 是一个规则型 JAR 爬虫的 Go 实现。
type Engine interface {
	Home(ctx context.Context) ([]provider.Section, error)
	Browse(ctx context.Context, cat string, page int) (provider.Page, error)
	Search(ctx context.Context, q string) ([]provider.Item, error)
	Detail(ctx context.Context, id string) (Vod, error)
	Resolve(ctx context.Context, epURL string) (player.Stream, error)
}

// Supported 判断类名是否已有 Go 规则引擎。
func Supported(class string) bool {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "csp_xbpq":
		return true
	default:
		return false
	}
}

// NewEngine 读取 ext 规则并构造引擎。
func NewEngine(ctx context.Context, class, extURL string) (Engine, error) {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "csp_xbpq":
		raw, err := NewClient(nil).Get(ctx, extURL)
		if err != nil {
			return nil, fmt.Errorf("获取 XBPQ 规则失败: %w", err)
		}
		r, err := ParseXBPQ([]byte(raw))
		if err != nil {
			return nil, err
		}
		return NewXBPQEngine(r, NewClient(r.Headers)), nil
	default:
		return nil, fmt.Errorf("暂不支持的 JAR 爬虫类: %s", class)
	}
}
