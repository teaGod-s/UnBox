package rule

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FilterValue 是筛选选项；JSON 用 n/v。
type FilterValue struct {
	Name  string `json:"n"`
	Value string `json:"v"`
}

// FilterGroup 是一组筛选条件。
type FilterGroup struct {
	Key    string        `json:"key"`
	Name   string        `json:"name"`
	Values []FilterValue `json:"value"`
}

// XBPQ 是 XBPQ 规则文件的字段子集。字段名保持规则文件的中文键。
type XBPQ struct {
	HomeURL        string                   `json:"主页url"`
	Classes        string                   `json:"分类"`
	CategoryURL    string                   `json:"分类url"`
	Array          string                   `json:"数组"`
	Title          string                   `json:"标题"`
	Link           string                   `json:"链接"`
	Pic            string                   `json:"图片"`
	Subtitle       string                   `json:"副标题"`
	LinkPrefix     string                   `json:"链接前缀"`
	LinkSuffix     string                   `json:"链接后缀"`
	SearchURL      string                   `json:"搜索url"`
	SearchMode     string                   `json:"搜索模式"`
	SearchArray    string                   `json:"搜索数组"`
	SearchTitle    string                   `json:"搜索标题"`
	SearchLink     string                   `json:"搜索链接"`
	SearchPic      string                   `json:"搜索图片"`
	SearchSubtitle string                   `json:"搜索副标题"`
	Type           string                   `json:"影片类型"`
	Year           string                   `json:"影片年代"`
	Area           string                   `json:"影片地区"`
	Remarks        string                   `json:"影片状态"`
	Director       string                   `json:"导演"`
	Actor          string                   `json:"主演"`
	Content        string                   `json:"简介"`
	PlayFromArray  string                   `json:"线路数组"`
	PlayFromTitle  string                   `json:"线路标题"`
	PlayArray      string                   `json:"播放数组"`
	PlayList       string                   `json:"播放列表"`
	PlayTitle      string                   `json:"播放标题"`
	PlayLink       string                   `json:"播放链接"`
	DirectPlay     string                   `json:"直接播放"`
	SniffWords     string                   `json:"嗅探词"`
	Headers        map[string]string        `json:"请求头"`
	Filters        map[string][]FilterGroup `json:"筛选"`
}

// ParseXBPQ 解析规则 JSON；至少要有主页url或分类url之一。
func ParseXBPQ(data []byte) (*XBPQ, error) {
	var r XBPQ
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("解析 XBPQ 规则失败: %w", err)
	}
	if strings.TrimSpace(r.HomeURL) == "" && strings.TrimSpace(r.CategoryURL) == "" {
		return nil, fmt.Errorf("XBPQ 规则缺少 主页url/分类url")
	}
	return &r, nil
}
