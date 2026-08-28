// Package config 解析 TVBox 及其衍生分支的订阅配置。
package config

import "encoding/json"

// SiteType 是 TVBox 配置中 site.type 字段的取值。
type SiteType int

const (
	SiteTypeXPath  SiteType = 0
	SiteTypeCMS    SiteType = 1
	SiteTypeSpider SiteType = 3
	SiteTypeRemote SiteType = 4
)

func (t SiteType) String() string {
	switch t {
	case SiteTypeXPath:
		return "xpath"
	case SiteTypeCMS:
		return "cms"
	case SiteTypeSpider:
		return "spider"
	case SiteTypeRemote:
		return "remote"
	default:
		return "unknown"
	}
}

// Site 是一个点播站点。
type Site struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Type       SiteType `json:"type"`
	API        string   `json:"api"`
	Searchable int      `json:"searchable"`
	Ext        any      `json:"ext"`
}

// Channel 是直播源中的一个频道，可含多条备用流。
type Channel struct {
	Name  string   `json:"name"`
	URLs  []string `json:"urls"`
	Logo  string   `json:"logo"`
	Group string   `json:"group"`
}

// Live 是一组直播源。
type Live struct {
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Channels []Channel `json:"channels"`
}

// LiveList 是 Config.Lives 的容错切片类型。
//
// TVBox 的 lives 字段在真实源里可能出现 [[]]、[{}] 这类畸形形态；若直接用
// []Live 反序列化，单个畸形项会让整份配置解析失败（encoding/json 对结构体
// 字段要么全成要么全败），导致全部 sites 一并丢失。这里逐元素解析并跳过
// 无法解析成 Live 对象的项，把「单个坏直播组」的代价限制在丢那一组。
type LiveList []Live

func (l *LiveList) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make([]Live, 0, len(raw))
	for _, item := range raw {
		var live Live
		if err := json.Unmarshal(item, &live); err != nil {
			continue
		}
		out = append(out, live)
	}
	*l = out
	return nil
}

// StoreHouse 是多仓订阅中的一个仓库入口。
type StoreHouse struct {
	SourceName string `json:"sourceName"`
	SourceURL  string `json:"sourceUrl"`
}

// Config 是解析后的统一配置模型。
type Config struct {
	Spider    string   `json:"spider"`
	Wallpaper string   `json:"wallpaper"`
	Logo      string   `json:"logo"`
	Sites     []Site   `json:"sites"`
	Lives     LiveList `json:"lives"`
	Hosts     []string `json:"hosts"`
	// StoreHouse 与 URLs 用于多仓/聚合结构，二者非空时表示这是索引而非终端配置。
	StoreHouse []StoreHouse `json:"storeHouse"`
	URLs       []struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	} `json:"urls"`
	// SourceName 是解析时填充的「线路/仓库名」（urls[].name 或 storeHouse[].sourceName）。
	// 单线路源为空字符串。
	SourceName string `json:"sourceName,omitempty"`
}
