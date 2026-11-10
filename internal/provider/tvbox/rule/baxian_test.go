package rule

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// 某聚合源源完整冒烟：`a?id=1` → 主仓库 d.php → 23 条线路 b.php?id=N（内容为
// `\njhSPAyzn**e` 前缀 + base64 编码的站点配置）→ 每条线路的站点逐个
// Home/Browse/Search 冒烟，统计引擎 bug / 站点侧 / 可播。
//
// 之前的 TestSmokeBaxian 只测 a?id=1 第一层（看到的是仓库列表而非站点），
// 所以永远「无站点」。真正站点藏在三层嵌套 + base64 之下。
//
// 用法：UNBOX_ACCEPTANCE=1 go test ./internal/provider/tvbox/rule/ -run TestSmokeBaxianFull -v -count=1
func TestSmokeBaxianFull(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	ctx := context.Background()
	client := NewClient(nil)

	// 第 1 层：a?id=1 → 仓库列表 → 主仓库 d.php URL
	top, err := client.Get(ctx, "https://xn--cnq85d.52011.top/a?id=1")
	if err != nil {
		t.Fatalf("拉取 a?id=1 失败: %v", err)
	}
	mainRepo := baxianField(top, `"sourceUrl"\s*:\s*"([^"]*d\.php[^"]*)"`)
	if mainRepo == "" {
		t.Fatalf("a?id=1 未找到主仓库 URL：%s", clipStr(top, 300))
	}
	t.Logf("主仓库：%s", mainRepo)

	// 第 2 层：主仓库 → 23 条线路（b.php?id=N）
	repoBody, err := client.Get(ctx, encodeQuery(mainRepo))
	if err != nil {
		t.Fatalf("拉取主仓库失败: %v", err)
	}
	lineRe := regexp.MustCompile(`"url"\s*:\s*"([^"]*b\.php\?id=\d+[^"]*)"\s*,\s*"name"\s*:\s*"([^"]*)"`)
	lines := lineRe.FindAllStringSubmatch(repoBody, -1)
	if len(lines) == 0 {
		t.Fatalf("主仓库未找到线路：%s", clipStr(repoBody, 400))
	}
	t.Logf("线路数：%d", len(lines))

	// 第 3 层：每条线路 → base64 站点配置 → 站点冒烟
	var all []smokeSite
	failed := 0
	for _, line := range lines {
		url, name := line[1], line[2]
		body, err := client.Get(ctx, url)
		if err != nil {
			t.Logf("线路 %s(%s) 拉取失败: %v", url, name, shortErr(err))
			continue
		}
		dec := baxianDecode(body)
		if dec == "" {
			t.Logf("线路 %s(%s) 解码失败（长度 %d）", url, name, len(body))
			failed++
			continue
		}
		sites := extractAllSites(dec)
		// XBPQ/XYQHiker 站点 ext 是相对路径（./XBPQ/x.json），按线路 URL 展开。
		expandSiteExt(url, sites)
		t.Logf("线路 %s 站点 %d 个", name, len(sites))
		all = append(all, sites...)
	}
	t.Logf("某聚合源源全部站点共 %d 个（线路解码失败 %d 条）", len(all), failed)

	// 去重（不同线路可能有同名同 api 站点）
	var dedup []smokeSite
	seen := map[string]bool{}
	for _, s := range all {
		key := s.name + s.class
		if seen[key] {
			continue
		}
		seen[key] = true
		dedup = append(dedup, s)
	}
	t.Logf("去重后 %d 个\n", len(dedup))
	if os.Getenv("SMOKE_COLLECT") == "1" {
		// 只验证四层拆解，不跑冒烟。
		for _, s := range dedup {
			t.Logf("  %q class=%q ext=%q searchable=%v", s.name, s.class, s.ext, s.searchable)
		}
		return
	}
	runSmoke(t, "某聚合源源", dedup)
}

// baxianField 从配置文本里提取首个命名字段。
func baxianField(raw, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// encodeQuery 把 URL 的查询参数按 UTF-8 编码（主仓库 ?b=某聚合源 的中文参数
// 不编码会收到 HTTP 400）。
func encodeQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	u.RawQuery = q.Encode()
	return u.String()
}

// baxianDecode 解开某聚合源线路的 base64 封装。线路体统一为
// `\njhSPAyzn**e`（`\n` + 8 字符 + `**` = 11 字节前缀，其后紧跟 base64，
// 该 11 字节不含 base64 首字符，base64 从第 11 字节起）后接**带 padding 的
// 标准 base64**（StdEncoding）。部分线路 base64 段内已含 padding。
// 极少数无前缀明文 JSONC 直接返回。
func baxianDecode(body string) string {
	if strings.Contains(body, `"sites"`) {
		return body
	}
	const prefix = "\njhSPAyzn**"
	if strings.HasPrefix(body, prefix) {
		dec, err := base64.StdEncoding.DecodeString(body[len(prefix):])
		if err == nil {
			return string(dec)
		}
		// padding 缺失时退回 RawStd。
		if dec2, err2 := base64.RawStdEncoding.DecodeString(body[len(prefix):]); err2 == nil {
			return string(dec2)
		}
		return ""
	}
	// 无前缀兜底（旧格式/换行排版差异）
	for start := 0; start < len(body); start++ {
		if dec, err := base64.StdEncoding.DecodeString(body[start:]); err == nil {
			return string(dec)
		}
		if dec, err := base64.RawStdEncoding.DecodeString(body[start:]); err == nil {
			return string(dec)
		}
	}
	return ""
}
