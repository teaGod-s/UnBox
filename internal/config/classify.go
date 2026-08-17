package config

import "strings"

// Support 表示 Unbox 对某个站点的支持程度。
type Support int

const (
	SupportNo    Support = iota // 不支持，且无计划支持
	SupportMaybe                // 可能支持，需运行时验证
	SupportYes                  // 支持
)

func (s Support) String() string {
	switch s {
	case SupportYes:
		return "可用"
	case SupportMaybe:
		return "待定"
	default:
		return "不支持"
	}
}

// Classify 判断站点的爬虫种类及 Unbox 的支持程度。
//
// 依据 spec 第 2.1 节的实测分布：JAR 与 Python 爬虫在桌面端不可行，
// XPath 在当前生态实测占比为 0 且不列入实现。
func Classify(s Site) (kind string, sup Support) {
	api := strings.ToLower(s.API)

	// 去掉 query string 与 fragment，只按文件扩展名做匹配。
	if idx := strings.IndexAny(api, "?#"); idx >= 0 {
		api = api[:idx]
	}

	switch s.Type {
	case SiteTypeCMS:
		return "cms", SupportYes
	case SiteTypeXPath:
		return "xpath", SupportNo
	case SiteTypeRemote:
		return "remote", SupportMaybe
	case SiteTypeSpider:
		switch {
		case strings.HasPrefix(api, "csp_"):
			return "jar", SupportNo
		case strings.HasSuffix(api, ".js"):
			return "js", SupportYes
		case strings.HasSuffix(api, ".py"):
			return "python", SupportNo
		case strings.HasPrefix(api, "http"):
			return "http", SupportMaybe
		}
	}
	return "unknown", SupportNo
}
