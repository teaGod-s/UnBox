package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"
)

// js/drpy2 站点完整播放链路冒烟：对主订阅（tvboxmuti）子源里 api 指向 .js
// 脚本的站点，逐个走 Load → Init → 分类 → 列表 → 详情 → play 首集。
// 与 rule 包的规则型冒烟互补，覆盖 drpy2/drpyS JS 引擎侧的播放层 bug。
//
// 用法：UNBOX_ACCEPTANCE=1 go test ./internal/crawler/ -run TestSmokeJS -v -count=1 -timeout 3600s

var smokeJSONCRe = regexp.MustCompile(`(?m)^\s*//.*$`)

func TestSmokeJS(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	subs := []string{
		"https://gh-proxy.com/raw.githubusercontent.com/ytadiao/ytadiao/main/1.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/js.json",
		"https://clun.top/box.json",
		"https://clun.top/js/jddyy.js",
	}
	_ = subs

	// 收集 tvboxmuti 14 子源里的 js 站（api 以 .js 结尾）。
	// 只拉常规可达的子源，避免整棵聚合递归。
	direct := []string{
		"https://gh-proxy.com/raw.githubusercontent.com/yw88075/tvbox/main/yw.json",
		"https://clun.top/box.json",
		"https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQTVBox.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/js.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQ.json",
		"https://gh-proxy.com/https://raw.githubusercontent.com/yoursmile66/TVBox/main/XC.json",
		"https://www.wya6.cn/tv/yc.json",
		"https://gitlab.com/noimank/tvbox/-/raw/main/tvbox1.json",
	}
	var jsJobs []jsSite
	for _, rawURL := range direct {
		raw, err := fetchStr(rawURL)
		if err != nil {
			t.Logf("子源 %s 拉取失败: %v", rawURL, shortErrStr(err))
			continue
		}
		n := extractJSSites(raw, rawURL, &jsJobs)
		t.Logf("子源 %s → js 站点 %d 个", rawURL, n)
	}

	t.Logf("js/drpy2 站点共 %d 个，启动完整播放链路冒烟…", len(jsJobs))

	// SMOKE_LIMIT=<n> 时只跑前 n 站（排查崩溃定位用）。
	if lim := os.Getenv("SMOKE_LIMIT"); lim != "" {
		n := 0
		for _, c := range lim {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 && n < len(jsJobs) {
			jsJobs = jsJobs[:n]
			t.Logf("SMOKE_LIMIT=%d，只跑前 %d 站", n, len(jsJobs))
		}
	}

	var ok, engineBug, siteSide []string
	for i, job := range jsJobs {
		if i%15 == 0 {
			t.Logf("进度 %d/%d…", i, len(jsJobs))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		eng := New()
		eng.SetContext(ctx)
		// 与 tvbox.Spider 的选源一致：ext 指向 js 且 api 是 drpy 引擎库时
		// 加载 ext；否则加载 api 本身。
		loadURL := job.api
		if isJSFile(job.ext) && isDrpyLib(job.api) {
			loadURL = job.ext
		}
		// api/ext 可能是相对路径（./dr/js/xxx.js），按来源子源展开成绝对地址。
		loadURL = resolveSub(job.base, loadURL)
		if err := eng.LoadFromURL(ctx, loadURL); err != nil {
			engineBug = append(engineBug, fmt.Sprintf("%s: 加载 %v", job.name, shortErrStr(err)))
			cancel()
			continue
		}
		if err := eng.Init(); err != nil {
			engineBug = append(engineBug, fmt.Sprintf("%s: init %v", job.name, shortErrStr(err)))
			cancel()
			continue
		}
		classes, cerr := eng.VodClasses()
		if cerr != nil || len(classes) == 0 {
			if cerr != nil && looksNetErr(cerr) {
				siteSide = append(siteSide, fmt.Sprintf("%s: 分类网络 %v", job.name, shortErrStr(cerr)))
			} else if cerr != nil {
				engineBug = append(engineBug, fmt.Sprintf("%s: 分类 %v", job.name, shortErrStr(cerr)))
			} else {
				siteSide = append(siteSide, fmt.Sprintf("%s: 无分类", job.name))
			}
			cancel()
			continue
		}
		items, berr := eng.VodCategory(classes[0].TypeID, 1)
		if berr != nil || len(items) == 0 {
			if berr != nil && looksNetErr(berr) {
				siteSide = append(siteSide, fmt.Sprintf("%s: 列表网络 %v", job.name, shortErrStr(berr)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s: 列表=%d项 %v", job.name, len(items), shortErrStr(berr)))
			}
			cancel()
			continue
		}
		detail, derr := eng.VodDetail(items[0].VodID.String())
		if derr != nil || strings.TrimSpace(detail.VodPlayURL) == "" {
			if derr != nil && looksNetErr(derr) {
				siteSide = append(siteSide, fmt.Sprintf("%s: 详情网络 %v", job.name, shortErrStr(derr)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s: 详情 %v playURL=%q", job.name, shortErrStr(derr), detail.VodPlayURL))
			}
			cancel()
			continue
		}
		// 首集 play
		firstSeg := strings.SplitN(detail.VodPlayURL, "$$$", 2)[0]
		firstEp := strings.SplitN(firstSeg, "#", 2)[0]
		_, epURL, _ := strings.Cut(firstEp, "$")
		src := strings.SplitN(detail.VodPlayFrom, "$$$", 2)[0]
		purl, perr := eng.VodPlay(src, epURL)
		if perr != nil {
			if looksNetErr(perr) {
				siteSide = append(siteSide, fmt.Sprintf("%s: play网络 %v", job.name, shortErrStr(perr)))
			} else {
				engineBug = append(engineBug, fmt.Sprintf("%s: play %v", job.name, shortErrStr(perr)))
			}
			cancel()
			continue
		}
		if strings.HasPrefix(purl, "http://") || strings.HasPrefix(purl, "https://") {
			ok = append(ok, fmt.Sprintf("%s: 首集播放 %s", job.name, clipS(purl, 70)))
		} else {
			siteSide = append(siteSide, fmt.Sprintf("%s: play结果非媒体 %q", job.name, clipS(purl, 50)))
		}
		cancel()
	}

	t.Logf("\n=== js/drpy2 完整链路汇总: 可播=%d 站点侧=%d 引擎bug=%d ===", len(ok), len(siteSide), len(engineBug))
	if len(engineBug) > 0 {
		t.Logf("\n=== 疑似引擎 bug ===")
		for _, b := range engineBug {
			t.Logf("  %s", b)
		}
	}
	if len(ok) > 0 {
		t.Logf("\n=== 首集可播（%d）前15 ===", len(ok))
		for i, b := range ok {
			if i >= 15 {
				break
			}
			t.Logf("  %s", b)
		}
	}
	if len(siteSide) > 0 {
		t.Logf("\n=== 站点侧（前20） ===")
		for i, b := range siteSide {
			if i >= 20 {
				break
			}
			t.Logf("  %s", b)
		}
	}
}

type jsSite struct {
	name, api, ext, base string
}

func extractJSSites(raw, subURL string, out *[]jsSite) int {
	clean := smokeJSONCRe.ReplaceAllString(raw, "")
	var root any
	if err := json.Unmarshal([]byte(clean), &root); err != nil {
		return 0
	}
	n := 0
	var walk func(v any, depth int)
	walk = func(v any, depth int) {
		if depth > 6 {
			return
		}
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e, depth+1)
			}
		case map[string]any:
			if list, ok := x["sites"].([]any); ok {
				for _, s := range list {
					if m, ok := s.(map[string]any); ok {
						api, _ := m["api"].(string)
						name, _ := m["name"].(string)
						if isJSFile(api) {
							ext := ""
							switch e := m["ext"].(type) {
							case string:
								ext = e
							case map[string]any:
								b, _ := json.Marshal(e)
								ext = string(b)
							case []any:
								b, _ := json.Marshal(e)
								ext = string(b)
							}
							// api 是直播引擎（lf_live_min.js 等）且 ext 是订阅 txt 时，
							// 这是直播源（走 provider/live），不是点播 js 爬虫，跳过。
							if isJSFile(api) && ext != "" && !isJSFile(ext) {
								continue
							}
							*out = append(*out, jsSite{name: name, api: api, ext: ext, base: subURL})
							n++
						}
					}
				}
			}
			for _, c := range x {
				walk(c, depth+1)
			}
		}
	}
	walk(root, 0)
	return n
}

// isJSFile 判断 api/ext 是否指向 .js 脚本（忽略 query）。
func isJSFile(raw string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return strings.HasSuffix(strings.ToLower(raw), ".js")
}

// isDrpyLib 判断 api 是否指向 drpy/drpy2 引擎库。
func isDrpyLib(raw string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	base := strings.ToLower(path.Base(raw))
	return strings.HasPrefix(base, "drpy") && strings.HasSuffix(base, ".js")
}

func fetchStr(u string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "okhttp/3.12.11")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func looksNetErr(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") || strings.Contains(s, "超时") ||
		strings.Contains(s, "http ") || strings.Contains(s, "连接失败") ||
		strings.Contains(s, "tls") || strings.Contains(s, "404") ||
		strings.Contains(s, "403") || strings.Contains(s, "deadline") ||
		strings.Contains(s, "无法解析")
}

func shortErrStr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 90 {
		return s[:90] + "…"
	}
	return s
}

func clipS(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// resolveSub 按 base 展开相对路径（./x.js → https://host/path/x.js）。
func resolveSub(base, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return b.ResolveReference(ref).String()
}
