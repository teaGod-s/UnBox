package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unbox/unbox/internal/config"
)

// Report 是订阅源的兼容性体检结果。
//
// 没有 LiveChannels 字段：FongMi多线路 配置 JSON 本身不携带直播频道，频道只存在
// 于每个直播源各自的 m3u 里，需要额外为每个直播源发一次网络请求才能拿到。
// unbox-scan 只做订阅结构的展开与站点分类，不做这一步 m3u 抓取，因此这里
// 如果放一个必然是 0 的频道计数字段，就是在假装做过一件实际没做的事。
type Report struct {
	ConfigCount int            `json:"configCount"`
	TotalSites  int            `json:"totalSites"`
	Usable      int            `json:"usable"`
	Maybe       int            `json:"maybe"`
	Unsupported int            `json:"unsupported"`
	ByKind      map[string]int `json:"byKind"`
	LiveGroups  int            `json:"liveGroups"`
}

// BuildReport 汇总一组已展开的终端配置，得到站点分类统计与直播组数。
func BuildReport(cfgs []*config.Config) Report {
	r := Report{ByKind: map[string]int{}, ConfigCount: len(cfgs)}
	for _, c := range cfgs {
		for _, s := range c.Sites {
			kind, sup := config.Classify(s)
			r.TotalSites++
			r.ByKind[kind]++
			switch sup {
			case config.SupportYes:
				r.Usable++
			case config.SupportMaybe:
				r.Maybe++
			default:
				r.Unsupported++
			}
		}
		r.LiveGroups += len(c.Lives)
	}
	return r
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

// Text 渲染人类可读的报告。
//
// TotalSites 为 0 时不走百分比分支：一份 "0  (0.0%)" 的统计表读起来像是
// "体检跑完了、结果恰好是 0"，容易和"没扫到任何站点"混淆，这里必须用
// 一句明确的话把两者区分开。
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "配置解析\n  成功 %d 个配置\n\n", r.ConfigCount)

	if r.TotalSites == 0 {
		b.WriteString("点播站点  0 个（未发现任何点播站点，无法评估兼容性）\n\n")
	} else {
		fmt.Fprintf(&b, "点播站点  %d 个\n", r.TotalSites)
		fmt.Fprintf(&b, "  可用    %3d  (%.1f%%)\n", r.Usable, pct(r.Usable, r.TotalSites))
		fmt.Fprintf(&b, "  待定    %3d  (%.1f%%)\n", r.Maybe, pct(r.Maybe, r.TotalSites))
		fmt.Fprintf(&b, "  不支持  %3d  (%.1f%%)\n\n", r.Unsupported, pct(r.Unsupported, r.TotalSites))

		kinds := make([]string, 0, len(r.ByKind))
		for k := range r.ByKind {
			kinds = append(kinds, k)
		}
		sort.Slice(kinds, func(i, j int) bool {
			if r.ByKind[kinds[i]] != r.ByKind[kinds[j]] {
				return r.ByKind[kinds[i]] > r.ByKind[kinds[j]]
			}
			return kinds[i] < kinds[j]
		})
		b.WriteString("按类型\n")
		for _, k := range kinds {
			fmt.Fprintf(&b, "  %-8s %d\n", k, r.ByKind[k])
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "直播源    %d 组\n", r.LiveGroups)
	b.WriteString("（频道数需实际拉取每个直播源的 m3u 才能统计，unbox-scan 不做此步，故不列出）\n")
	return b.String()
}

// JSON 序列化报告。
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
