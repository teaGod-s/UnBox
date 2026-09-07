// Package library 实现本地媒体库：目录扫描、片名/海报识别、本地文件服务与播放路由。
package library

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// tagRe 匹配字幕组/下载站水印标签（[xxx]、【xxx】），标签内不是片名。
var tagRe = regexp.MustCompile(`[\[【][^\]】]*[\]】]`)

// seasonEpRe 匹配季集标记（S01E01、EP01、E01、第03集、第1话 等）。
// 拉丁形式必须被空白/点/下划线/连字符包围：既避免把单词里的字母误判成集号，
// 也避免 1080p 里的 e 被当成 E1080。中文「第N集/话」本身无歧义，不要求分隔符。
var seasonEpRe = regexp.MustCompile(
	`(?i)(?:^|[\s._-])((?:s\d{1,2}\s*e\d{1,3}|ep\s*\d{1,3}|e\s*\d{1,3}))(?:[\s._-]|$)|第\s*\d{1,3}\s*[集话]`)

// namePunct 是剥离后留在片名两端、没有表意价值的分隔字符。
const namePunct = "._-（）()【】[]"

// displayName 从文件路径提取展示片名：
//  1. 去扩展名，剥掉 [...] / 【...】 水印标签；
//  2. 季集标记保留为「 · S01E01」后缀，同季多集在列表里才分得开；
//  3. 中文片名在首个点号或连续空格处截断（「片名.元数据」「片名  英语版」两种约定）；
//     英文片名不截断，避免「UNIX - Making Computers...」被砍成「UNIX」；
//  4. 折叠空白；清洗后为空则回落到原始 stem，再回落到文件名。
func displayName(path string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = tagRe.ReplaceAllString(stem, "")

	marker := ""
	if loc := seasonEpRe.FindStringSubmatchIndex(stem); loc != nil {
		start, end := loc[2], loc[3]
		if start < 0 { // 中文分支无捕获组，标记即整个匹配
			start, end = loc[0], loc[1]
		}
		marker = strings.Join(strings.Fields(stem[start:end]), " ")
		if asciiOnly(marker) {
			marker = strings.ToUpper(marker)
		}
		stem = stem[:loc[0]]
	}

	if hasCJK(stem) {
		cut := strings.Index(stem, ".")
		if i := indexDoubleSpace(stem); i >= 0 && (cut < 0 || i < cut) {
			cut = i
		}
		if cut > 0 {
			stem = stem[:cut]
		}
	}

	stem = strings.Trim(strings.Join(strings.Fields(stem), " "), namePunct)
	if stem == "" {
		stem = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if stem == "" {
		return base
	}
	if marker != "" {
		return stem + " · " + marker
	}
	return stem
}

// asciiOnly 判断 s 是否只含 ASCII 字符。
func asciiOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// hasCJK 判断 s 是否含中日韩表意文字。
func hasCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0xF900 && r <= 0xFAFF) {
			return true
		}
	}
	return false
}

// indexDoubleSpace 返回第一个连续空格的位置，无则返回 -1。
func indexDoubleSpace(s string) int {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ' ' && s[i+1] == ' ' {
			return i
		}
	}
	return -1
}

// findPoster 在同目录找海报：<stem>-poster.jpg / poster.jpg / folder.jpg / 目录名.jpg。
func findPoster(dir, stem string) string {
	if absoluteDir, err := filepath.Abs(dir); err == nil {
		dir = absoluteDir
	}
	candidates := []string{
		filepath.Join(dir, stem+"-poster.jpg"),
		filepath.Join(dir, stem+"-poster.png"),
		filepath.Join(dir, "poster.jpg"),
		filepath.Join(dir, "folder.jpg"),
		filepath.Join(dir, "cover.jpg"),
		filepath.Join(dir, filepath.Base(dir)+".jpg"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
