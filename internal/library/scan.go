package library

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unbox/unbox/internal/store"
)

// videoExts 是被识别的视频容器扩展名（D5 白名单）。
var videoExts = map[string]bool{
	"mp4": true, "mkv": true, "m4v": true, "mov": true, "avi": true,
	"flv": true, "ts": true, "m2ts": true, "wmv": true, "rmvb": true,
	"rm": true, "webm": true, "mpg": true, "mpeg": true, "3gp": true, "vob": true,
}

// scanDir 递归扫描 root 下的视频文件，返回规范化条目。
func scanDir(root string) ([]store.LibraryItem, error) {
	var items []store.LibraryItem
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 仅根目录错误需要报告；后代不可读时继续扫描其他路径。
			if filepath.Clean(path) == filepath.Clean(root) {
				return walkErr
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry == nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if !videoExts[ext] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		dir := filepath.Dir(path)
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		items = append(items, store.LibraryItem{
			Path:   path,
			Name:   displayName(path),
			Dir:    dir,
			Ext:    ext,
			Size:   info.Size(),
			MTime:  info.ModTime().Unix(),
			Poster: findPoster(dir, stem),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
