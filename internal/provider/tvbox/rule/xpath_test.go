package rule

import (
	"reflect"
	"testing"
)

// xpathFixtureHTML 模拟 XPath 规则站点的典型结构：分类列表 + 详情信息，
// 属性选择器、位置谓词、text() 取值都覆盖。
const xpathFixtureHTML = `<!DOCTYPE html><html><body>
<div id="main">
<div class="module-items">
  <a class="module-item" href="/v1"><div class="pic"><img data-src="/pic1.jpg"></div><h3>片名X1</h3></a>
  <a class="module-item" href="/v2"><div class="pic"><img data-src="/pic2.jpg"></div><h3>片名X2</h3></a>
</div>
<div class="video-info-main">
  <span class="actor">主演：王五</span>
  <p>简介甲</p><p>简介乙</p><p>简介丙</p>
</div>
</div>
</body></html>`

// 分类列表数组：//div[@class=...]//a[@class=...]——容器下所有列表项。
func TestXPathItemArray(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	got := xpathExtractFirst(doc, "//div[@class='module-items']//a[@class='module-item']")
	if len(got) != 2 {
		t.Fatalf("列表数组 = %d 项，期望 2：%v", len(got), got)
	}
}

// text() 取值：最后一段取节点文本。
func TestXPathTextExtraction(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	got := xpathExtractFirst(doc, "//div[@class='module-items']//h3/text()")
	want := []string{"片名X1", "片名X2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("h3 文本 = %v，期望 %v", got, want)
	}
}

// @属性 取值：取 href。
func TestXPathAttrExtraction(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	got := xpathExtractFirst(doc, "//a[@class='module-item']/@href")
	want := []string{"/v1", "/v2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("@href = %v，期望 %v", got, want)
	}
}

// 位置谓词 [1]：取第 2 个 p。
func TestXPathPosition(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	got := xpathExtractFirst(doc, "//div[@class='video-info-main']//p[2]/text()")
	if len(got) != 1 || got[0] != "简介乙" {
		t.Fatalf("p[2] = %v，期望 [简介乙]", got)
	}
}

// 属性定位 #id 与 class 点号简写。
func TestXPathIDShortcut(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	got := xpathExtractFirst(doc, "//*[@id='main']//p[last()]/text()")
	if len(got) != 1 || got[0] != "简介丙" {
		t.Fatalf("last() = %v，期望 [简介丙]", got)
	}
}

// 空规则返回空。
func TestXPathEmptyRule(t *testing.T) {
	doc := jsoupDoc(t, xpathFixtureHTML)
	if got := xpathExtractFirst(doc, ""); len(got) != 0 {
		t.Fatalf("空 xpath 应返回空：%v", got)
	}
}
