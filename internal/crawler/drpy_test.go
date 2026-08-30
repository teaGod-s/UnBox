package crawler

import (
	"net/url"
	"testing"
)

func TestFillURL(t *testing.T) {
	if got := fillURL("/list/fyclass--------fypage---.html", "movie", 2); got != "/list/movie--------2---.html" {
		t.Fatalf("fillURL=%q", got)
	}
	if got := fillURL("/s?wd=**&page=fypage", "斗罗", 1); got != "/s?wd="+url.QueryEscape("斗罗")+"&page=1" {
		t.Fatalf("fillURL search=%q", got)
	}
}
