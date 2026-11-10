package rule

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/provider"
)

// 真实站点端到端验收。默认跳过；需要联网时显式运行：
//
//	UNBOX_ACCEPTANCE=1 go test ./internal/provider/tvbox/rule/ -run TestAcceptance -v -count=1
//
// 验收断言保持宽松（不校验具体条数/片名），只要求规则能解析、
// 各阶段不报错且产出非空，以容忍站点内容漂移。
func acceptanceEnabled() bool { return os.Getenv("UNBOX_ACCEPTANCE") == "1" }

// 真实站点端到端验收的规则地址经环境变量 ACCEPTANCE_RULES_JSON 注入，
// 不在代码里写死任何实际地址。格式：JSON 对象 {"<key>": "<规则URL>"}。
//
//	ACCEPTANCE_RULES_JSON='{"wangou":"https://example.com/wangou.json",...}' \
//	  UNBOX_ACCEPTANCE=1 go test ./internal/provider/tvbox/rule/ -run TestAcceptance -v -count=1
var acceptanceRules = func() map[string]string {
	m := map[string]string{}
	raw := os.Getenv("ACCEPTANCE_RULES_JSON")
	if raw == "" {
		return m
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return m
	}
	return m
}()

// accURL 返回某个验收样例的规则地址；未配置时跳过该测试。
func accURL(t *testing.T, key string) string {
	t.Helper()
	u := acceptanceRules[key]
	if u == "" {
		t.Skipf("ACCEPTANCE_RULES_JSON 未配置样例 %q，跳过", key)
	}
	return u
}

const (
	accWangou     = "wangou"
	accYingsou    = "yingsou"
	accBidi       = "bidi"
	accHema       = "hema"
	accTuxibaobei = "tuxiaobei"
	accDDYS       = "ddys"
	accBuman      = "buman"
	accQkan       = "qkan"
)

// 样例A（csp_XYQHikerAL，完整 JSoup 站点）：分类→列表→详情→选集。
func TestAcceptanceWangou(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHikerAL", accURL(t, accWangou))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("Browse 返回空列表")
	}
	t.Logf("首分类列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)

	detailID := page.Items[0].ID
	v, err := eng.Detail(ctx, detailID)
	if err != nil {
		t.Fatalf("Detail(%s) 失败: %v", detailID, err)
	}
	t.Logf("详情: 片名=%q 线路=%q 剧集段数=%d", v.Name, v.PlayFrom, countPlays(v.PlayURL))
	if v.Name == "" {
		t.Fatal("详情片名为空")
	}
}

// 样例B（csp_XYQHiker，搜索-only + JSON 响应 + POST）。
func TestAcceptanceYingsou(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accYingsou))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	items, err := eng.Search(ctx, "庆余年")
	if err != nil {
		t.Fatalf("JSON 搜索失败: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("JSON 搜索返回空")
	}
	t.Logf("样例B结果: %d 项，首项=%q id=%q", len(items), items[0].Title, items[0].ID)
}

// 样例C（:has() / || / !前缀 / bing 搜索）。
func TestAcceptanceBidi(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accBidi))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("样例C分类数: %d", len(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	t.Logf("样例C列表: %d 项", len(page.Items))
}

// 样例D（React 应用类站点，img&&alt 取片名）。
func TestAcceptanceHema(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accHema))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("样例D分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("样例D Browse 返回空列表")
	}
	t.Logf("样例D列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 样例E2（儿歌站：44 分类、mip-img&&src 图片、mip-search-video 选集）。
func TestAcceptanceTuxiaobei(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accTuxibaobei))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("样例E分类数: %d", len(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("样例E Browse 返回空列表")
	}
	t.Logf("样例E列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 样例F（[firstPg=...] 变体分类链接、detail 为 XPath 模式 / 空线路）。
func TestAcceptanceDDYS(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accDDYS))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("样例F分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("样例F Browse 返回空列表")
	}
	t.Logf("样例F列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 某动漫站（a&&title!在线观看 去前缀、[firstPage] 分类链接）。
func TestAcceptanceDongmanBus(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accBuman))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("某动漫站分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("某动漫站 Browse 返回空列表")
	}
	t.Logf("某动漫站列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 样例G（JSON 搜索模式 + XPath 详情）。
func TestAcceptanceQkan(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accQkan))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("样例G分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("样例G Browse 返回空列表")
	}
	t.Logf("样例G列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

func secNames(secs []provider.Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Title
	}
	return out
}

// countPlays 统计 PlayURL 里的剧集数（# 分隔各集）。
func countPlays(playURL string) int {
	if playURL == "" {
		return 0
	}
	return strings.Count(playURL, "#") + 1
}

// contentSnippet 截取简介前若干字符供日志展示。
func contentSnippet(s string) string {
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// 样例G：XPath/标记截取详情链路（详情是否Jsoup写法=0）。
func TestAcceptanceQkanDetail(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accQkan))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	page, err := eng.Browse(ctx, "33", 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("Browse = %d 项, %v", len(page.Items), err)
	}
	v, err := eng.Detail(ctx, page.Items[0].ID)
	if err != nil {
		t.Fatalf("Detail(%s) 失败: %v", page.Items[0].ID, err)
	}
	t.Logf("qkan 详情: 片名=%q 线路=%q 内容=%q 剧集数=%d", v.Name, v.PlayFrom, contentSnippet(v.Content), countPlays(v.PlayURL))
	if v.PlayFrom == "" {
		t.Log("qkan 线路名为空（该站线路规则可能不匹配，属站点侧）")
	}
}

// 样例D：选集完整链路（a:not(:has(img)) 伪类 + 字段首段自匹配 +
// 选集全文提取）。
func TestAcceptanceHemaDetail(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accURL(t, accHema))
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	page, err := eng.Browse(ctx, "0", 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("Browse = %d 项, %v", len(page.Items), err)
	}
	v, err := eng.Detail(ctx, page.Items[0].ID)
	if err != nil {
		t.Fatalf("Detail(%s) 失败: %v", page.Items[0].ID, err)
	}
	t.Logf("hema 详情: 片名=%q 剧集数=%d 前几集=%v", v.Name, countPlays(v.PlayURL), epNames(v.PlayURL, 3))
	if countPlays(v.PlayURL) == 0 {
		t.Fatal("hema 选集为空，期望 a:not(:has(img)) 语法要能取出免费集")
	}
}

func epNames(playURL string, n int) []string {
	var out []string
	for _, seg := range strings.Split(playURL, "$$$") {
		if len(out) >= n {
			break
		}
		for _, ep := range strings.Split(seg, "#") {
			if len(out) >= n {
				break
			}
			name, _, _ := strings.Cut(ep, "$")
			out = append(out, name)
		}
	}
	return out
}
