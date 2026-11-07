package rule

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// fixtureHTML 模拟一个典型 JSoup 链式规则站点的布局：
// 顶部有混入的「饭太硬/公众号」推荐模块（真实 wangou 规则用
// :not(:matches(...)) 过滤掉），列表项把片名放在 title 属性、图片在 data-src。
// 另有详情页结构（多演员 / 多段简介），用于 ,N / ,-1 索引测试。
const fixtureHTML = `<html><head><title>标题</title></head><body>
<div class="module-items">
  <div class="module-item" sub="1"><a title="片名A" href="/a"><div class="module-item-pic"><img data-src="/pic/a.jpg"></div><div class="module-item-text">更新A</div></a></div>
  <div class="module-item" sub="2"><a title="片名B" href="/b"><div class="module-item-pic"><img data-src="/pic/b.jpg"></div><div class="module-item-text">更新B</div></a></div>
</div>
<div class="module-item">饭太硬广告</div>
<div class="module-item">公众号推广</div>
<div class="video-info-main">
  <div class="video-info-actor">主演：张三</div>
  <div class="video-info-actor">主演：李四</div>
  <div class="video-info-content"><p>简介第一段</p><p>简介第二段</p></div>
</div>
</body></html>`

func jsoupDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("构造测试 HTML 失败: %v", err)
	}
	return doc
}

// 列表数组规则：先定位容器 .module-items，再在容器内取 .module-item，
// 且用 :not(:matches(...)) 过滤掉广告项。
func TestJsoupArrayChain(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".module-items&&.module-item:not(:matches(饭太硬|公众号))")
	// 应该命中 A、B 两个列表项：广告的 .module-item 全部被滤掉
	if len(got) != 2 {
		t.Fatalf("切片数组规则 = %d 项，期望 2：%v", len(got), got)
	}
}

// 分类片单标题：a&&title —— 在列表项内找 a，取 title 属性。
func TestJsoupChainTitleAttr(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".module-item&&a&&title")
	want := []string{"片名A", "片名B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("标题 = %v，期望 %v", got, want)
	}
}

// 分类片单图片：.module-item-pic&&img&&data-src —— 两级 Find 再取属性。
func TestJsoupChainPicDataSrc(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".module-item-pic&&img&&data-src")
	want := []string{"/pic/a.jpg", "/pic/b.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("图片 = %v，期望 %v", got, want)
	}
}

// 分类片单副标题：Text —— 取节点纯文本。
func TestJsoupChainText(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".module-item&&.module-item-text&&Text")
	want := []string{"更新A", "更新B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("副标题 = %v，期望 %v", got, want)
	}
}

// ,1 索引：取第 2 个命中的演员节点再继续（真实 wangou 详情规则）。
func TestJsoupChainIndexSuffix(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".video-info-main&&.video-info-actor,1&&Text")
	if len(got) != 1 || got[0] != "主演：李四" {
		t.Fatalf(",1 应命中第 2 个演员：%v", got)
	}
}

// ,-1 索引：负数从末尾数，-1 = 最后一段简介（真实 wangou 详情规则）。
func TestJsoupChainLastIndex(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".video-info-content&&p,-1&&Text")
	if len(got) != 1 || got[0] != "简介第二段" {
		t.Fatalf("p,-1 应命中最后一个 p：%v", got)
	}
}

// 无提取步时返回元素文本（默认行为）。
func TestJsoupMissingElement(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".video-info-main&&span&&href")
	if len(got) != 0 {
		t.Fatalf("不存在的元素应返回空: %v", got)
	}
}

// 空规则返回空。
func TestJsoupChainEmptyRule(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	if got := jsoupExtractFirst(doc, ""); len(got) != 0 {
		t.Fatalf("空规则应返回空: %v", got)
	}
}

// :matches 正过滤：只保留文本匹配的节点。
func TestJsoupChainMatchesFilter(t *testing.T) {
	doc := jsoupDoc(t, fixtureHTML)
	got := jsoupExtractFirst(doc, ".module-item:matches(饭太硬|公众号)")
	if len(got) != 2 {
		t.Fatalf(":matches 应命中 2 个广告项，得 %d：%v", len(got), got)
	}
}

// || 回退：img&&data-src||src，第一个属性缺失时取第二个。
func TestJsoupAttrFallback(t *testing.T) {
	const html = `<div class="card"><img src="/fallback.jpg"></div>
<div class="card"><img data-src="/lazy.jpg"></div>`
	doc := jsoupDoc(t, html)
	got := jsoupExtractFirst(doc, ".card&&img&&data-src||src")
	want := []string{"/fallback.jpg", "/lazy.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("|| 回退 = %v，期望 %v", got, want)
	}
}

// !前缀 去前缀：Text!主演： 去掉结果里的固定前缀。
func TestJsoupTrimPrefix(t *testing.T) {
	const html = `<p class="actor">主演：张三</p>`
	doc := jsoupDoc(t, html)
	got := jsoupExtractFirst(doc, ".actor&&Text!主演：")
	if len(got) != 1 || got[0] != "张三" {
		t.Fatalf("!去前缀 = %v，期望 [张三]", got)
	}
}

// :has() 后置过滤：只保留内部含指定后代的节点。
func TestJsoupHasFilter(t *testing.T) {
	const html = `<div class="card"><div id="download-list"></div><a href="/d1">下载卡</a></div>
<div class="card"><a href="/d2">普通卡</a></div>`
	doc := jsoupDoc(t, html)
	got := jsoupExtractFirst(doc, ".card:has(#download-list)&&a&&href")
	if len(got) != 1 || got[0] != "/d1" {
		t.Fatalf(":has 过滤 = %v，期望 [/d1]", got)
	}
}

// 多个 || 选择器备选：第一个选择器无命中时退到第二个。
func TestJsoupSelectorFallback(t *testing.T) {
	const html = `<div class="badge">角标</div><div class="ribbon">缎带</div>`
	doc := jsoupDoc(t, html)
	got := jsoupExtractFirst(doc, ".missing||.ribbon&&Text")
	if len(got) != 1 || got[0] != "缎带" {
		t.Fatalf("选择器 || 回退 = %v，期望 [缎带]", got)
	}
}
