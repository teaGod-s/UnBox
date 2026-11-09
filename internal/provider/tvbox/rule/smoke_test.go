package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// 主订阅（tvboxmuti）里已支持引擎站点（csp_XBPQ / csp_XYQHiker*）的批量
// 冒烟：逐站 Home → Browse，把失败分成「引擎 bug」与「站点侧问题」两类，
// 供排查「本应可播但因 UnBox 缺陷不支持」的站点。
//
// 用法：UNBOX_ACCEPTANCE=1 go test ./internal/provider/tvbox/rule/ -run TestSmoke -v -count=1
// 说明：子源逐一下载（复用 fetch → spin 的函数），再按 api 前缀过滤已支持类。
func TestSmoke(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	// 来源：tvboxmuti 的 14 条子线路中，仅拉取下载成功的常规子源。
	subs := []string{
		"https://gh-proxy.com/raw.githubusercontent.com/yw88075/tvbox/main/yw.json",
		"https://gitlab.com/noimank/tvbox/-/raw/main/tvbox1.json",
		"https://clun.top/box.json",
		"https://gh-proxy.com/raw.githubusercontent.com//gaotianliuyun/gao/master/0827.json",
		"https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQTVBox.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/js.json",
		"https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQ.json",
		"https://gh-proxy.com/https://raw.githubusercontent.com/yoursmile66/TVBox/main/XC.json",
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
	runSmoke(t, "主订阅", sites)
}

// TestSmokeBaxian 对八仙源（xn--cnq85d.52011.top）全部站点冒烟。
// 该源 api 全为未实现类（csp_Douban/Duopan/PanWebShare/Config/Push/Market/py)，
// 冒烟确认它们因「引擎未实现」而非其他 bug 而不可用。
// 用法：UNBOX_ACCEPTANCE=1 go test ./... -run 'TestSmokeBaxian' -v -count=1
func TestSmokeBaxian(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	ctx := context.Background()
	raw, err := NewClient(nil).Get(ctx, "https://xn--cnq85d.52011.top/a?id=1")
	if err != nil {
		t.Fatalf("八仙源拉取失败: %v", err)
	}
	sites := extractAllSites(raw)
	runSmoke(t, "八仙源", sites)
}

// runSmoke 对一批站点逐一 Home/Browse/Search，统计三类结果。
func runSmoke(t *testing.T, label string, sites []smokeSite) {
	if len(sites) == 0 {
		t.Fatalf("%s 无站点", label)
	}
	t.Logf("%s: %d 个站点，开始冒烟…", label, len(sites))

	var engineBugs, siteIssues, transOK []string
	okCount := 0
	unsupported := 0
	for i, s := range sites {
		if i%15 == 0 {
			t.Logf("进度 %d/%d…", i, len(sites))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		// 内联规则 ext（对象/文本）是引擎未实现的形态，和「未支持的 JAR
		// 爬虫类」一样归未实现桶，不当作 URL 请求（否则报空 URL / parse
		// 失败误导为引擎 bug）。对象用标记串占位；樱花动漫这类文本规则
		//（`分类url:...,分类:...`）本身不是 URL，一并按内联规则处理。
		if !extIsRuleURL(s.ext) {
			unsupported++
			t.Logf("%s(%s): 内联规则ext（引擎未实现形态）", s.name, s.class)
			cancel()
			continue
		}
		eng, err := NewEngine(ctx, s.class, s.ext)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "暂不支持的 JAR 爬虫类") {
				unsupported++
				t.Logf("%s(%s): 引擎未实现（预期）", s.name, s.class)
				cancel()
				continue
			}
			engineBugs = append(engineBugs, fmt.Sprintf("%s(%s): 加载规则 %v", s.name, s.class, shortErr(err)))
			cancel()
			continue
		}
		secs, err := eng.Home(ctx)
		if err != nil {
			if looksNetwork(err) {
				siteIssues = append(siteIssues, fmt.Sprintf("%s(%s): Home网络 %v", s.name, s.class, shortErr(err)))
			} else {
				engineBugs = append(engineBugs, fmt.Sprintf("%s(%s): Home %v", s.name, s.class, shortErr(err)))
			}
			cancel()
			continue
		}
		if len(secs) == 0 {
			siteIssues = append(siteIssues, fmt.Sprintf("%s(%s): 无分类", s.name, s.class))
			cancel()
			continue
		}
		// Browse 一个分类，进一步分层
		if s.searchable {
			items, serr := eng.Search(ctx, "火影")
			if serr != nil {
				if looksNetwork(serr) {
					siteIssues = append(siteIssues, fmt.Sprintf("%s(%s): Search网络 %v", s.name, s.class, shortErr(serr)))
				} else {
					engineBugs = append(engineBugs, fmt.Sprintf("%s(%s): Search %v", s.name, s.class, shortErr(serr)))
				}
				cancel()
				continue
			}
			transOK = append(transOK, fmt.Sprintf("%s(%s): 搜索%d", s.name, s.class, len(items)))
		} else {
			pg, berr := eng.Browse(ctx, secs[0].ID, 1)
			if berr != nil {
				if looksNetwork(berr) {
					siteIssues = append(siteIssues, fmt.Sprintf("%s(%s): Browse网络 %v", s.name, s.class, shortErr(berr)))
				} else {
					engineBugs = append(engineBugs, fmt.Sprintf("%s(%s): Browse %v", s.name, s.class, shortErr(berr)))
				}
				cancel()
				continue
			}
			transOK = append(transOK, fmt.Sprintf("%s(%s): 列表%d", s.name, s.class, len(pg.Items)))
		}
		okCount++
		cancel()
	}
	t.Logf("\n=== %s 汇总: 通过=%d 站点侧=%d 疑似引擎bug=%d 引擎未实现=%d ===", label, okCount, len(siteIssues), len(engineBugs), unsupported)
	if len(transOK) > 0 {
		t.Logf("\n=== 可用 ===")
		for _, x := range transOK {
			t.Logf("  %s", x)
		}
	}
	if len(engineBugs) > 0 {
		t.Logf("\n=== 疑似引擎 bug ===")
		for _, b := range engineBugs {
			t.Logf("  %s", b)
		}
	}
	if len(siteIssues) > 0 {
		t.Logf("\n=== 站点侧问题（非bug）前15 ===")
		for i, b := range siteIssues {
			if i >= 15 {
				break
			}
			t.Logf("  %s", b)
		}
	}
}

type smokeSite struct {
	name, class, ext string
	searchable       bool
}

// looksNetwork 粗略区分站点侧网络问题（这类通常与引擎无关）。
func looksNetwork(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "超时") || strings.Contains(s, "timeout") ||
		strings.Contains(s, "http ") || strings.Contains(s, "连接") ||
		strings.Contains(s, "tls") || strings.Contains(s, "无法解析") ||
		strings.Contains(s, "404") || strings.Contains(s, "403") ||
		strings.Contains(s, "请求") || strings.Contains(s, "deadline")
}

// extractAllSites 收集一个源里所有 api 站点（不限已支持类），
// 供「任意源全量冒烟」使用（如八仙源全部 12 站）。
// 八仙源是混杂注释/尾逗号的 JSONC，整份 Unmarshal 会失败，这里用正则
// 逐对象提取（站点对象可能含一层嵌套 ext，正则容忍一层 {}），
// 保证不因解析失败漏站。
func extractAllSites(raw string) []smokeSite {
	objRe := regexp.MustCompile(`(?s)\{(?:[^{}]|\{[^{}]*\})*"api"(?:[^{}]|\{[^{}]*\})*\}`)
	seen := map[string]bool{}
	var out []smokeSite
	for _, m := range objRe.FindAllString(raw, -1) {
		api := reField(m, `"api"\s*:\s*"([^"]*)"`)
		name := reField(m, `"name"\s*:\s*"([^"]*)"`)
		key := reField(m, `"key"\s*:\s*"([^"]*)"`)
		if api == "" || seen[key+api] {
			continue
		}
		seen[key+api] = true
		searchable := reField(m, `"searchable"\s*:\s*(\d+)`) == "1" || reField(m, `"searchable"\s*:\s*(true)`) == "true"
		ext := reField(m, `"ext"\s*:\s*"([^"]*)"`)
		// ext 为对象（`"ext": {...}`）：八仙内联规则形态，规则直接写在
		// ext 里（含 分类url/主页url 等字段），无需外链。extractAllSites
		// 用标记串占位，冒烟把它归「未实现形态」而不是当 URL 请求。
		if isInlineExt(m) {
			ext = "<内联规则ext对象>"
		}
		out = append(out, smokeSite{name: name, class: api, ext: ext, searchable: searchable})
	}
	return out
}

// isInlineExt 判断站点对象文本里 ext 是否是对象（`"ext": {`）——
// 八仙源给 csp_XBPQ 站内联完整规则（副标题/请求头/编码/分类url 等），
// 这类站无需外链规则文件，引擎暂不支持这种形态。
func isInlineExt(m string) bool {
	return regexp.MustCompile(`"ext"\s*:\s*\{`).MatchString(m)
}

// extIsRuleURL 判断 ext 是否是规则文件 URL（而非内联规则文本/对象）。
// 引擎支持的是「ext 指向要下载的规则文件」：`./x.json`、`/x.json` 相对
// 路径或 `http(s)://` 绝对地址。八仙源里 `分类url:https://...,分类:...`
// 这类逗号分隔的文本规则（以字段名开头，scheme 是字段值的一部分）与
// `<内联规则ext对象>` 标记都不是 URL —— 归未实现。
func extIsRuleURL(ext string) bool {
	ext = strings.TrimSpace(ext)
	if ext == "" || strings.HasPrefix(ext, "<") {
		return false
	}
	if strings.HasPrefix(ext, "./") || strings.HasPrefix(ext, "/") {
		return true
	}
	return strings.HasPrefix(ext, "http://") || strings.HasPrefix(ext, "https://")
}

// expandSiteExt 把站点 ext 的相对路径（./x.json）按 base 展开成绝对 URL，
// 与 extractSupportedSites 的 resolve 语义一致（否则引擎拿 ./x.json 当 URL
// 请求，误报加载失败）。已带协议的原样保留。
func expandSiteExt(base string, sites []smokeSite) {
	for i := range sites {
		ext := strings.TrimSpace(sites[i].ext)
		if ext == "" || strings.Contains(ext, "://") || strings.HasPrefix(ext, "<") {
			continue
		}
		b, err := url.Parse(base)
		if err != nil || b.Host == "" {
			continue
		}
		r, err := url.Parse(ext)
		if err != nil {
			continue
		}
		sites[i].ext = b.ResolveReference(r).String()
	}
}

// reField 从站点对象文本里提取某个字段值。
func reField(m string, pattern string) string {
	re := regexp.MustCompile(pattern)
	sm := re.FindStringSubmatch(m)
	if len(sm) < 2 {
		return ""
	}
	return sm[1]
}

// extractSupportedSites 从子源 JSON 里收集 XBPQ / XYQHiker 站点。
// 站点的 ext 常是相对路径（如 "./XBPQ/6080.json"），按子源 base 展开成
// 绝对地址（与 config.ResolveRelativeURLs 相同语义），否则会把相对路径
// 当 URL 请求，误报成加载失败。
func extractSupportedSites(raw, subURL string) []smokeSite {
	clean := jsoncCommentRe.ReplaceAllString(raw, "")
	var root any
	if err := json.Unmarshal([]byte(clean), &root); err != nil {
		return nil
	}
	var out []smokeSite
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
						lower := strings.ToLower(strings.TrimSpace(api))
						switch {
						case strings.HasPrefix(lower, "csp_xbpq"),
							strings.HasPrefix(lower, "csp_xyqhiker"):
							out = append(out, mkSite(m, subURL))
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
	return out
}

func mkSite(m map[string]any, subURL string) smokeSite {
	name, _ := m["name"].(string)
	api, _ := m["api"].(string)
	ext := ""
	switch e := m["ext"].(type) {
	case string:
		ext = e
	case map[string]any:
		b, _ := json.Marshal(e)
		ext = string(b)
	}
	// 相对路径 ext（./XBPQ/xxx.json）按子源地址展开成绝对地址。
	if ext != "" && !strings.HasPrefix(ext, "http://") && !strings.HasPrefix(ext, "https://") {
		if abs := resolveRelativeURLTest(subURL, ext); abs != "" {
			ext = abs
		}
	}
	// searchable 双形态：0/1 数字与 bool。
	searchable := false
	switch v := m["searchable"].(type) {
	case float64:
		searchable = v != 0
	case bool:
		searchable = v
	}
	return smokeSite{name: name, class: api, ext: ext, searchable: searchable}
}

// resolveRelativeURLTest 按 base 解析相对地址（net/url 语义）。
func resolveRelativeURLTest(base, raw string) string {
	bu, err := url.Parse(base)
	if err != nil || bu.Host == "" {
		return ""
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if ref.Scheme != "" || ref.Host != "" {
		return raw
	}
	return bu.ResolveReference(ref).String()
}

var jsoncCommentRe = regexp.MustCompile(`(?m)^\s*//.*$`)

func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 90 {
		return s[:90] + "…"
	}
	return s
}
