package rule

import (
	"encoding/json"
	"fmt"
	"strings"
)

// uaMobile / uaPC / uaUC / uaIOS 与 drpy2 引擎里的同名全局常量保持一致。
// XBPQ 规则经常用 "User-Agent$MOBILE_UA" 这种写法引用它们。
const (
	uaMobile = "Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/90.0.4430.91 Mobile Safari/537.36"
	uaPC = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/95.0.4638.54 Safari/537.36"
	uaUC = "Mozilla/5.0 (Linux; U; Android 9; zh-CN; MI 9 Build/PKQ1.181121.001) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/57.0.2987.108 " +
		"UCBrowser/12.5.5.1035 Mobile Safari/537.36"
	uaIOS = "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) " +
		"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1"
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

// FilterMap 兼容 筛选 的对象写法与「JSON 字符串」写法；无法识别时按无筛选
// 处理，避免一条可选字段让整站规则失效。
type FilterMap map[string][]FilterGroup

func (f *FilterMap) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*f = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var m map[string][]FilterGroup
		if err := json.Unmarshal(data, &m); err != nil {
			return nil
		}
		*f = m
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		var m map[string][]FilterGroup
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			*f = m
		}
	}
	return nil
}

// HeaderMap 兼容 请求头 的两种真实写法：
//   - 对象：{"User-Agent":"..."}
//   - 字符串："User-Agent$MOBILE_UA#Referer$https://..."
type HeaderMap map[string]string

func (h *HeaderMap) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*h = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		*h = m
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*h = parseHeaderString(raw)
	return nil
}

// parseHeaderString 解析 "K$V#K$V" 形态的请求头。
func parseHeaderString(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '#' || r == '\n' || r == '\r'
	}) {
		k, v, ok := strings.Cut(part, "$")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = resolveUAConstant(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveUAConstant 把规则里的 UA 常量展开成真实 User-Agent。
func resolveUAConstant(v string) string {
	switch v {
	case "MOBILE_UA":
		return uaMobile
	case "PC_UA":
		return uaPC
	case "UC_UA":
		return uaUC
	case "IOS_UA":
		return uaIOS
	case "UA":
		return "Mozilla/5.0"
	default:
		return v
	}
}

// XBPQ 是 XBPQ 规则文件的字段子集。字段名保持规则文件的中文键。
type XBPQ struct {
	HomeURL        string `json:"主页url"`
	Classes        string `json:"分类"`
	CategoryURL    string `json:"分类url"`
	Array          string `json:"数组"`
	Title          string `json:"标题"`
	Link           string `json:"链接"`
	Pic            string `json:"图片"`
	Subtitle       string `json:"副标题"`
	LinkPrefix     string `json:"链接前缀"`
	LinkSuffix     string `json:"链接后缀"`
	SearchURL      string `json:"搜索url"`
	SearchMode     string `json:"搜索模式"`
	SearchArray    string `json:"搜索数组"`
	SearchTitle    string `json:"搜索标题"`
	SearchLink     string `json:"搜索链接"`
	SearchPic      string `json:"搜索图片"`
	SearchSubtitle string `json:"搜索副标题"`
	Type           string `json:"影片类型"`
	Year           string `json:"影片年代"`
	Area           string `json:"影片地区"`
	Remarks        string `json:"影片状态"`
	// 兼容另一套更短的详情字段名（真实规则里两套并存）。
	LegacyType    string    `json:"类型"`
	LegacyYear    string    `json:"年代"`
	LegacyArea    string    `json:"地区"`
	LegacyStatus  string    `json:"状态"`
	Director      string    `json:"导演"`
	Actor         string    `json:"主演"`
	Content       string    `json:"简介"`
	PlayFromArray string    `json:"线路数组"`
	PlayFromTitle string    `json:"线路标题"`
	PlayArray     string    `json:"播放数组"`
	PlayList      string    `json:"播放列表"`
	PlayTitle     string    `json:"播放标题"`
	PlayLink      string    `json:"播放链接"`
	DirectPlay    string    `json:"直接播放"`
	SniffWords    string    `json:"嗅探词"`
	Headers       HeaderMap `json:"请求头"`
	Filters       FilterMap `json:"筛选"`
}

// ParseXBPQ 解析规则 JSON；至少要有主页url或分类url之一。
func ParseXBPQ(data []byte) (*XBPQ, error) {
	var r XBPQ
	if err := json.Unmarshal(sanitizeJSONEscapes(data), &r); err != nil {
		return nil, fmt.Errorf("解析 XBPQ 规则失败: %w", err)
	}
	if strings.TrimSpace(r.HomeURL) == "" && strings.TrimSpace(r.CategoryURL) == "" {
		return nil, fmt.Errorf("XBPQ 规则缺少 主页url/分类url")
	}
	return &r, nil
}

// sanitizeJSONEscapes 把非标准 JSON 转义（真实规则里出现过 \&）补成
// 字面量反斜杠，避免一条规则因为一个字符整份作废。
func sanitizeJSONEscapes(data []byte) []byte {
	needsFix := false
	for i := 0; i+1 < len(data); i++ {
		if data[i] == '\\' && !validJSONEscape(data[i+1]) {
			needsFix = true
			break
		}
	}
	if !needsFix {
		return data
	}
	out := make([]byte, 0, len(data)+16)
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' && i+1 < len(data) && !validJSONEscape(data[i+1]) {
			out = append(out, '\\', '\\')
			continue
		}
		out = append(out, data[i])
	}
	return out
}

func validJSONEscape(c byte) bool {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}
