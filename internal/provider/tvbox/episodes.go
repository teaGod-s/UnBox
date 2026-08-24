package tvbox

import (
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/provider"
)

// sourceSep 是详情接口里线路名 / 线路播放段的统一分隔符（实测确认）。
const sourceSep = "$$$"

// splitSources 拆分线路名（vod_play_from）。详情接口用 $$$，列表接口用 ,，二者兼容。
func splitSources(from string) []string {
	from = strings.TrimSpace(from)
	if from == "" {
		return nil
	}
	if strings.Contains(from, sourceSep) {
		return splitNonEmpty(from, sourceSep)
	}
	if strings.Contains(from, ",") {
		return splitNonEmpty(from, ",")
	}
	return []string{from}
}

// parseEpisodes 把 vod_play_url 拆成剧集列表。
// playURL：线路间 $$$、集间 #、集名与地址间第一个 $。sources 为对应线路名。
// 每条剧集的 ID 编码为 "<vodID>/<线路下标>/<集下标>"，供 Resolve 反查。
func parseEpisodes(vodID, playURL string, sources []string) []provider.Episode {
	var out []provider.Episode
	for si, seg := range strings.Split(playURL, sourceSep) {
		src := ""
		if si < len(sources) {
			src = sources[si]
		}
		for ei, ep := range strings.Split(seg, "#") {
			name, url := splitEpisode(ep)
			if url == "" {
				continue
			}
			out = append(out, provider.Episode{
				ID:     fmt.Sprintf("%s/%d/%d", vodID, si, ei),
				Source: src,
				Name:   name,
				URL:    url,
			})
		}
	}
	return out
}

// splitEpisode 拆 "集名$地址"；无 $ 时整串视为地址。
func splitEpisode(ep string) (name, url string) {
	if name, url, ok := strings.Cut(ep, "$"); ok {
		return name, url
	}
	return "", ep
}

// splitNonEmpty 按 sep 拆分并去掉空段。
func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
