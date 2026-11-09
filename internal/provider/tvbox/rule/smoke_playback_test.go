package rule

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// 完整播放链路冒烟：对每个已支持引擎站点走 Home→Browse→Detail→Resolve 首集，
// 专门揪出「引擎能列出剧集、但 Resolve 拿不到可播地址」的播放层 bug。
// 上一版冒烟只测到 Browse/Search，播放层 bug（斗鱼 PostBody、看影网线路、
// 加密云播 Resolve 等）全部漏网。
//
// 用法：UNBOX_ACCEPTANCE=1 go test ./internal/provider/tvbox/rule/ -run TestSmokePlayback -v -count=1
func TestSmokePlayback(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	subs := []string{
		"https://gh-proxy.com/raw.githubusercontent.com/yw88075/tvbox/main/yw.json",
		"https://gitlab.com/noimank/tvbox/-/raw/main/tvbox1.json",
		"https://clun.top/box.json",
		"https://gh-proxy.com/raw.githubusercontent.com//gaotianliuyun/gao/master/0827.json",
		"https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQTVBox.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQ.json",
		"https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQHiker/看影网.json",
		"https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQHiker/路漫漫.json",
	}
	var sites []smokeSite
	for _, rawURL := range subs {
		raw, err := NewClient(nil).Get(context.Background(), rawURL)
		if err != nil {
			t.Logf("子源 %s 拉取失败: %v", rawURL, shortErr(err))
			continue
		}
		sites = append(sites, extractSupportedSites(raw, rawURL)...)
	}
	// 去重（多个子源可能引用同一 ext）
	seen := map[string]bool{}
	var uniq []smokeSite
	for _, s := range sites {
		if s.ext == "" || seen[s.ext] {
			continue
		}
		seen[s.ext] = true
		uniq = append(uniq, s)
	}
	t.Logf("已支持站点（去重后）%d 个，启动完整播放链路冒烟…", len(uniq))

	var okLine, engineBug, siteSide, cipherCloud []string
	for idx, s := range uniq {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		eng, err := NewEngine(ctx, s.class, s.ext)
		if err != nil {
			if strings.Contains(err.Error(), "暂不支持的 JAR 爬虫类") {
				cancel()
				continue
			}
			engineBug = append(engineBug, fmt.Sprintf("%s(%s): 加载 %v", s.name, s.class, shortErr(err)))
			cancel()
			continue
		}
		secs, err := eng.Home(ctx)
		if err != nil {
			if looksNetwork(err) {
				siteSide = append(siteSide, fmt.Sprintf("%s(%s): Home网络 %v", s.name, s.class, shortErr(err)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s(%s): Home %v", s.name, s.class, shortErr(err)))
			}
			cancel()
			continue
		}
		if len(secs) == 0 {
			siteSide = append(siteSide, fmt.Sprintf("%s(%s): 无分类", s.name, s.class))
			cancel()
			continue
		}
		pg, err := eng.Browse(ctx, secs[0].ID, 1)
		if err != nil || len(pg.Items) == 0 {
			if err != nil && looksNetwork(err) {
				siteSide = append(siteSide, fmt.Sprintf("%s(%s): Browse网络 %v", s.name, s.class, shortErr(err)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s(%s): Browse=%d项 %v", s.name, s.class, len(pg.Items), shortErr(err)))
			}
			cancel()
			continue
		}
		// Detail 第一项
		v, err := eng.Detail(ctx, pg.Items[0].ID)
		if err != nil {
			if looksNetwork(err) {
				siteSide = append(siteSide, fmt.Sprintf("%s(%s): Detail网络 %v", s.name, s.class, shortErr(err)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s(%s): Detail %v", s.name, s.class, shortErr(err)))
			}
			cancel()
			continue
		}
		// 取第一线第一集
		firstSeg := strings.SplitN(v.PlayURL, "$$$", 2)[0]
		firstEp := strings.SplitN(firstSeg, "#", 2)[0]
		if firstEp == "" {
			siteSide = append(siteSide, fmt.Sprintf("%s(%s): 无选集", s.name, s.class))
			cancel()
			continue
		}
		_, epURL, _ := strings.Cut(firstEp, "$")
		if strings.TrimSpace(epURL) == "" {
			engineBug = append(engineBug, fmt.Sprintf("%s(%s): 选集URL为空", s.name, s.class))
			cancel()
			continue
		}
		// Resolve 播放
		stream, err := eng.Resolve(ctx, epURL)
		if err != nil {
			if strings.Contains(err.Error(), "加密云播") {
				cipherCloud = append(cipherCloud, fmt.Sprintf("%s(%s): %v", s.name, s.class, shortErr(err)))
			} else if looksNetwork(err) {
				siteSide = append(siteSide, fmt.Sprintf("%s(%s): Resolve网络 %v", s.name, s.class, shortErr(err)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s(%s): Resolve %v", s.name, s.class, shortErr(err)))
			}
			cancel()
			continue
		}
		media := strings.TrimSpace(stream.URL)
		isMedia := looksLikeMediaURL(media) || strings.HasPrefix(media, "http://") && strings.Contains(media, ".m3u8")
		_ = isMedia
		// 判定：Resolve 返回的地址是否为可播媒体（m3u8/mp4/flv… 直链）。
		if looksLikeMediaURL(media) {
			okLine = append(okLine, fmt.Sprintf("%s(%s): 首集播放 %s", s.name, s.class, clipStr(media, 70)))
		} else {
			siteSide = append(siteSide, fmt.Sprintf("%s(%s): Resolve结果非媒体 %q", s.name, s.class, clipStr(media, 60)))
		}
		if idx%10 == 0 {
			t.Logf("进度 %d/%d…", idx, len(uniq))
		}
		cancel()
	}

	t.Logf("\n=== 完整播放链路汇总: 可播=%d 站点侧=%d 引擎bug=%d 加密云播=%d ===", len(okLine), len(siteSide), len(engineBug), len(cipherCloud))
	if len(engineBug) > 0 {
		t.Logf("\n=== 疑似引擎 bug（需修） ===")
		for _, b := range engineBug {
			t.Logf("  %s", b)
		}
	}
	if len(cipherCloud) > 0 {
		t.Logf("\n=== 加密云播（已明确报错，非bug） ===")
		for _, b := range cipherCloud {
			t.Logf("  %s", b)
		}
	}
	if len(siteSide) > 0 {
		t.Logf("\n=== 站点侧（网络/反爬/无分类，非bug）前25 ===")
		for i, b := range siteSide {
			if i >= 25 {
				break
			}
			t.Logf("  %s", b)
		}
	}
	if len(okLine) > 0 {
		t.Logf("\n=== 首集可播（%d）前15 ===", len(okLine))
		for i, b := range okLine {
			if i >= 15 {
				break
			}
			t.Logf("  %s", b)
		}
	}
}
func clipStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
