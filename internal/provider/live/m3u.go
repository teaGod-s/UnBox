// Package live 实现 M1 的 IPTV 直播来源：M3U/TXT 解析 + Provider 适配。
package live

import (
	"bufio"
	"bytes"
	"strings"
)

// Entry 是 M3U/TXT 播放列表中的一条媒体条目。
type Entry struct {
	Name  string
	URL   string
	Logo  string
	Group string
	ID    string // tvg-id
}

// ParseM3U 解析 #EXTM3U 播放列表。#EXTINF 行中的 tvg-id / tvg-logo /
// group-title 属性被提取，逗号后的标题作为 Name。容错：剥 BOM、容忍
// CRLF、跳过空行与 # 注释行、无 #EXTINF 前导的 URL 跳过、#EXTINF 后
// 没有 URL 的条目丢弃。
func ParseM3U(raw []byte) []Entry {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	var out []Entry
	var cur *Entry // 当前待配对 URL 的 #EXTINF
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#EXTM3U") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXTINF") {
				e := parseExtinf(line)
				cur = &e
			}
			continue
		}
		// URL 行：必须已有 #EXTINF 前导才配对
		if cur != nil {
			cur.URL = line
			out = append(out, *cur)
			cur = nil
		}
	}
	return out
}

// parseExtinf 解析形如 `#EXTINF:-1 attr="v" attr2="v2",Name` 的行。
func parseExtinf(line string) Entry {
	var e Entry
	rest := strings.TrimPrefix(line, "#EXTINF")
	// 去掉时长字段（到第一个逗号为止），剩余是属性段
	if before, after, ok := strings.Cut(rest, ","); ok {
		e.Name = strings.TrimSpace(after)
		rest = before
	}
	e.ID = attr(rest, "tvg-id")
	e.Logo = attr(rest, "tvg-logo")
	e.Group = attr(rest, "group-title")
	return e
}

// attr 从 `key="value"` 形式的属性串中取值；不存在返回 ""。
func attr(s, key string) string {
	needle := key + "="
	_, s, found := strings.Cut(s, needle)
	if !found {
		return ""
	}
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, `"`) {
		s = strings.TrimPrefix(s, `"`)
		if before, _, ok := strings.Cut(s, `"`); ok {
			return before
		}
	}
	if j := strings.IndexAny(s, " \t"); j >= 0 {
		return s[:j]
	}
	return s
}

// ParseTXT 解析「名称,URL」每行一条的简单 TXT 播放列表。
func ParseTXT(raw []byte) []Entry {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	var out []Entry
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, url, ok := strings.Cut(line, ",")
		name = strings.TrimSpace(name)
		url = strings.TrimSpace(url)
		if !ok || name == "" || url == "" {
			continue
		}
		// 只接受带协议头的地址。HTML 网页里含逗号的行（meta/script/文本）会被
		// 切成「名字,伪地址」，不校验就会把网页当成一堆垃圾频道导进来。
		if !hasURLScheme(url) {
			continue
		}
		out = append(out, Entry{Name: name, URL: url})
	}
	return out
}

// hasURLScheme 判断地址是否带协议头（http://、rtmp://、udp:// 等）。
func hasURLScheme(raw string) bool {
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok || scheme == "" {
		return false
	}
	for i, r := range scheme {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') && i > 0:
		default:
			return false
		}
	}
	return true
}
