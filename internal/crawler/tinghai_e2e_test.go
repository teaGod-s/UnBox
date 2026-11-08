package crawler

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// 真实 tinghai.js 端到端：detail -> play 解真实媒体地址。需联网：
//
//	UNBOX_ACCEPTANCE=1 go test ./internal/crawler/ -run TestTinghaiE2E -v
func TestTinghaiE2E(t *testing.T) {
	if os.Getenv("UNBOX_ACCEPTANCE") != "1" {
		t.Skip("UNBOX_ACCEPTANCE=1 开启")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	e := New()
	e.SetContext(ctx)
	if err := e.LoadFromURL(ctx, "https://g.blfrp.cn/https://raw.githubusercontent.com/tushen6/Tomorrow/master/JS/tinghai.js"); err != nil {
		t.Fatalf("加载脚本失败: %v", err)
	}
	if err := e.Init(); err != nil {
		t.Fatalf("init 失败: %v", err)
	}
	classes, err := e.VodClasses()
	if err != nil || len(classes) == 0 {
		t.Fatalf("分类: %v %v", classes, err)
	}
	t.Logf("听海分类[0]=%q (共%d)", classes[0].TypeName, len(classes))

	// 在分类里找一个歌单并进详情
	items, err := e.VodCategory(classes[0].TypeID, 1)
	if err != nil || len(items) == 0 {
		t.Fatalf("分类列表: %v %v", items, err)
	}
	siteID := items[0].VodID.String()
	t.Logf("首个歌单 id=%q name=%q", siteID, items[0].VodName)

	detail, err := e.VodDetail(siteID)
	if err != nil || strings.TrimSpace(detail.VodPlayURL) == "" {
		t.Fatalf("详情: %v playURL=%q", err, detail.VodPlayURL)
	}
	t.Logf("play_from=%q", detail.VodPlayFrom)
	// 每个线路段内第一集：$ 前为集名，$ 后为剧集 URL
	firstSeg := strings.Split(detail.VodPlayURL, "$$$")[0]
	firstEp := strings.Split(firstSeg, "#")[0]
	epName := firstEp
	epURL := firstEp
	if name, url, ok := strings.Cut(firstEp, "$"); ok {
		epName, epURL = name, url
	}
	src := strings.Split(detail.VodPlayFrom, "$$$")[0]
	t.Logf("首集 name=%q url=%q", epName, epURL)
	if !strings.Contains(epURL, "&&") {
		t.Fatalf("听海剧集 URL 应为 rid&&pic&&pic 形态，得 %q", epURL)
	}

	playURL, err := e.VodPlay(src, epURL)
	if err != nil {
		t.Fatalf("VodPlay: %v", err)
	}
	t.Logf("解析真实地址: %q", playURL)
	if !strings.HasPrefix(playURL, "http://") && !strings.HasPrefix(playURL, "https://") {
		t.Fatalf("期望 http(s) 媒体地址，得 %q", playURL)
	}
}
