// Package library 实现本地媒体库：目录扫描、片名/海报识别、本地文件服务与播放路由。
package library

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// seasonEpRe 匹配常见季集后缀（S01E01、第03集、EP01、第1话 等）。
var seasonEpRe = regexp.MustCompile(`(?i)[. _-]*(s\d{1,2}e\d{1,2}|第\s*\d{1,3}\s*[集话]|ep?\s*\d{1,3}).*$`)

// displayName 从文件路径提取展示片名：去扩展名，去掉季集/分辨率等尾缀。
func displayName(path string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if match := seasonEpRe.FindStringIndex(stem); match != nil {
		stem = strings.TrimSpace(stem[:match[0]])
	}
	if i := strings.IndexAny(stem, ". "); i > 0 {
		stem = strings.TrimSpace(stem[:i])
	}
	if stem == "" {
		return base
	}
	return stem
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
