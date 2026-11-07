package rule

import (
	"context"
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

// 真实规则地址（来自用户订阅中的 gaotianliuyun/gao 仓库，走 gh-proxy）。
const (
	accWangou     = "https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQHiker/玩偶哥哥.json"
	accYingsou    = "https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQHiker/影搜.json"
	accBidi       = "https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQHiker/哔嘀影视.json"
	accHema       = "https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQHiker/河马短剧.json"
	accTuxibaobei = "https://gh-proxy.com/raw.githubusercontent.com/gaotianliuyun/gao/master/XYQHiker/兔小贝2.json"
	accDDYS       = "https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQHiker/低端影视.json"
	accBuman      = "https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQHiker/动漫巴士.json"
	accQkan       = "https://gh-proxy.org/https:/raw.githubusercontent.com/xyq254245/xyqonlinerule/main/XYQHiker/去看吧动漫.json"
)

// 玩偶哥哥（csp_XYQHikerAL，完整 JSoup 站点）：分类→列表→详情→选集。
func TestAcceptanceWangou(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHikerAL", accWangou)
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

// 影搜（csp_XYQHiker，搜索-only + JSON 响应 + POST）。
func TestAcceptanceYingsou(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accYingsou)
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
	t.Logf("影搜结果: %d 项，首项=%q id=%q", len(items), items[0].Title, items[0].ID)
}

// 哔嘀影视（:has() / || / !前缀 / bing 搜索）。
func TestAcceptanceBidi(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accBidi)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("哔嘀分类数: %d", len(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	t.Logf("哔嘀列表: %d 项", len(page.Items))
}

// 河马短剧（React 应用类站点，img&&alt 取片名）。
func TestAcceptanceHema(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accHema)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("河马分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("河马 Browse 返回空列表")
	}
	t.Logf("河马列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 兔小贝2（儿歌站：44 分类、mip-img&&src 图片、mip-search-video 选集）。
func TestAcceptanceTuxiaobei(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accTuxibaobei)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("兔小贝分类数: %d", len(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("兔小贝 Browse 返回空列表")
	}
	t.Logf("兔小贝列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 低端影视（[firstPg=...] 变体分类链接、detail 为 XPath 模式 / 空线路）。
func TestAcceptanceDDYS(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accDDYS)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("低端影视分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("低端影视 Browse 返回空列表")
	}
	t.Logf("低端影视列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 动漫巴士（a&&title!在线观看 去前缀、[firstPage] 分类链接）。
func TestAcceptanceDongmanBus(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accBuman)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("动漫巴士分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("动漫巴士 Browse 返回空列表")
	}
	t.Logf("动漫巴士列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
}

// 去看吧动漫（JSON 搜索模式 + XPath 详情）。
func TestAcceptanceQkan(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accQkan)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	secs, err := eng.Home(ctx)
	if err != nil || len(secs) == 0 {
		t.Fatalf("Home = %d 项, %v", len(secs), err)
	}
	t.Logf("去看吧动漫分类: %v", secNames(secs))

	page, err := eng.Browse(ctx, secs[0].ID, 1)
	if err != nil {
		t.Fatalf("Browse 失败: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("去看吧动漫 Browse 返回空列表")
	}
	t.Logf("去看吧动画列表: %d 项，首项=%q", len(page.Items), page.Items[0].Title)
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

// 去看吧动漫：XPath/标记截取详情链路（详情是否Jsoup写法=0）。
func TestAcceptanceQkanDetail(t *testing.T) {
	if !acceptanceEnabled() {
		t.Skip("跳过真实站点验收（设置 UNBOX_ACCEPTANCE=1 开启）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	eng, err := NewEngine(ctx, "csp_XYQHiker", accQkan)
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
	t.Logf("qkan 详情: 片名=%q 线路=%q 内容=%q", v.Name, v.PlayFrom, contentSnippet(v.Content))
	if v.PlayFrom == "" {
		t.Log("qkan 线路名为空（该站线路规则可能不匹配，属站点侧）")
	}
}
