package crawler

import (
	"net/url"
	"strconv"
	"strings"
)

func fillURL(tmpl, tid string, pg int) string {
	tmpl = strings.ReplaceAll(tmpl, "fyclass", tid)
	tmpl = strings.ReplaceAll(tmpl, "fypage", strconv.Itoa(pg))
	return strings.ReplaceAll(tmpl, "**", url.QueryEscape(tid))
}
