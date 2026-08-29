package crawler

// 真实爬虫验收（本地手动跑，CI 不触发）。
//
// 用法见 docs/2026-08-29-m5.1-real-acceptance.md。核心目的：用真实 FongMi js0
//（或 dr_py）爬虫跑通 load → 分类 → 列表 → 搜索 → 详情 → 播放，回答「文本级
// async/await 剥离是否够用，还是要升级 goja 原生 Promise」。
//
// 合规：真实地址/脚本只经环境变量传入，绝不写进代码或提交进 git。
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealCrawlerAcceptance(t *testing.T) {
	url := os.Getenv("UNBOX_REAL_JS_URL")
	path := os.Getenv("UNBOX_REAL_JS_PATH")
	if url == "" && path == "" {
		t.Skip("设置 UNBOX_REAL_JS_URL（远程 .js 地址）或 UNBOX_REAL_JS_PATH（本地文件）后运行真实爬虫验收")
	}
	keyword := os.Getenv("UNBOX_REAL_SEARCH")

	e := New()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取本地爬虫失败: %v", err)
		}
		if err := e.Load(string(b)); err != nil {
			dumpNormalized(t, string(b))
			t.Fatalf("加载爬虫失败（疑似 async/await 剥离问题，见 normalized 文件）: %v", err)
		}
	} else {
		if err := e.LoadFromURL(context.Background(), url); err != nil {
			t.Fatalf("下载/加载爬虫失败: %v", err)
		}
	}

	_, ruleErr := e.Rule()
	t.Logf("脚本形态: rule=%v init=%v homeContent=%v categoryContent=%v searchContent=%v detailContent=%v playerContent=%v",
		ruleErr == nil, hasFunction(e, "init"), hasFunction(e, "homeContent"),
		hasFunction(e, "categoryContent"), hasFunction(e, "searchContent"),
		hasFunction(e, "detailContent"), hasFunction(e, "playerContent"))

	// 分类
	classes, err := e.VodClasses()
	if err != nil {
		t.Logf("VodClasses 失败: %v", err)
	} else {
		t.Logf("分类(%d):", len(classes))
		for _, c := range classes {
			t.Logf("  - id=%q name=%q", c.TypeID, c.TypeName)
		}
	}

	// 列表（取第一个分类 id）
	tid := "1"
	if len(classes) > 0 && classes[0].TypeID != "" {
		tid = classes[0].TypeID
	}
	vods, err := e.VodCategory(tid, 1)
	if err != nil {
		t.Fatalf("VodCategory(%q) 失败: %v", tid, err)
	}
	t.Logf("分类列表(%d)（取前 3）:", len(vods))
	for i, v := range vods {
		if i >= 3 {
			break
		}
		t.Logf("  - id=%q name=%q remarks=%q", v.VodID, v.VodName, v.VodRemarks)
	}

	// 搜索
	if keyword != "" {
		sr, err := e.VodSearch(keyword)
		if err != nil {
			t.Logf("VodSearch(%q) 失败: %v", keyword, err)
		} else {
			t.Logf("搜索结果(%d)（取前 3）:", len(sr))
			for i, v := range sr {
				if i >= 3 {
					break
				}
				t.Logf("  - id=%q name=%q", v.VodID, v.VodName)
			}
		}
	}

	// 详情（取第一个影片 id）
	if len(vods) == 0 {
		t.Fatalf("分类列表为空，无法验证详情/播放")
	}
	vid := vods[0].VodID
	detail, err := e.VodDetail(vid)
	if err != nil {
		t.Fatalf("VodDetail(%q) 失败: %v", vid, err)
	}
	t.Logf("详情: name=%q year=%q area=%q", detail.VodName, detail.VodYear, detail.VodArea)
	t.Logf("  播放来源: %q", detail.VodPlayFrom)
	t.Logf("  播放地址: %q", detail.VodPlayURL)

	// 播放（best-effort，懒播放语义待校准，失败不 fail）
	flag, epURL := firstPlayPair(detail.VodPlayFrom, detail.VodPlayURL)
	playURL, err := e.VodPlay(flag, epURL)
	if err != nil {
		t.Logf("VodPlay(flag=%q, id=%q) 失败（懒播放语义待校准）: %v", flag, epURL, err)
	} else {
		t.Logf("播放地址: %q", playURL)
	}
}

// dumpNormalized 在 Load 失败时把剥离后的源码写到临时目录，供排查
// async/await 剥离是否误伤（含真实地址，属本地临时文件，勿提交）。
func dumpNormalized(t *testing.T, src string) {
	t.Helper()
	out := filepath.Join(os.TempDir(), "unbox-normalized.js")
	if err := os.WriteFile(out, []byte(normalizeModuleSource(src)), 0o600); err != nil {
		t.Logf("写出 normalized 失败: %v", err)
		return
	}
	t.Logf("已把剥离后的源码写到 %s（供排查 async/await 剥离是否误伤）", out)
}

// firstPlayPair 从 vod_play_from / vod_play_url 里抠出第一个 (线路名, 播放地址)。
// 仅用于冒烟，分隔符按 FongMi多线路 常见约定（来源 &&&/$$$，剧集 #，名/址 $）。
func firstPlayPair(from, urls string) (string, string) {
	flag := firstToken(from)
	block, _, _ := strings.Cut(urls, "#")
	parts := strings.SplitN(block, "$", 2)
	if len(parts) == 2 {
		return flag, parts[1]
	}
	return flag, block
}

func firstToken(s string) string {
	for _, r := range strings.FieldsFunc(s, func(r rune) bool {
		return r == '$' || r == '#' || r == '&' || r == '|'
	}) {
		if strings.TrimSpace(r) != "" {
			return strings.TrimSpace(r)
		}
	}
	return ""
}
